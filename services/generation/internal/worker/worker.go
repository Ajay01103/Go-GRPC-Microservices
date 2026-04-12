package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	db "github.com/go-grpc-sqlc/generation/internal/db"
	generations3 "github.com/go-grpc-sqlc/generation/internal/s3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type JobMessage struct {
	GenerationID string  `json:"generation_id"`
	JobID        string  `json:"job_id"`
	UserID       string  `json:"user_id"`
	Text         string  `json:"text"`
	VoiceKey     string  `json:"voice_key"`
	LanguageID   string  `json:"language_id"`
	Temperature  float64 `json:"temperature"`
	Exaggeration float64 `json:"exaggeration"`
	CfgWeight    float64 `json:"cfg_weight"`
}

type TTSWorker struct {
	redis                *redis.Client
	queries              db.Querier
	logger               *zap.Logger
	s3Client             *generations3.Client
	httpClient           *http.Client
	ttsEndpoint          string
	ttsAPIKey            string
	queueChannel         string
	resultsChannelPrefix string
}

func New(
	redisClient *redis.Client,
	queries db.Querier,
	logger *zap.Logger,
	s3Client *generations3.Client,
	ttsEndpoint string,
	ttsAPIKey string,
	queueChannel string,
	resultsChannelPrefix string,
) *TTSWorker {
	return &TTSWorker{
		redis:                redisClient,
		queries:              queries,
		logger:               logger,
		s3Client:             s3Client,
		httpClient:           &http.Client{Timeout: 90 * time.Second},
		ttsEndpoint:          strings.TrimSpace(ttsEndpoint),
		ttsAPIKey:            strings.TrimSpace(ttsAPIKey),
		queueChannel:         strings.TrimSpace(queueChannel),
		resultsChannelPrefix: strings.TrimSpace(resultsChannelPrefix),
	}
}

func (w *TTSWorker) Start(ctx context.Context) error {
	if w.queueChannel == "" {
		return errors.New("queue channel is required")
	}
	if w.ttsEndpoint == "" {
		return errors.New("tts endpoint is required")
	}
	if w.ttsAPIKey == "" {
		return errors.New("tts api key is required")
	}
	if w.s3Client == nil {
		return errors.New("generation s3 client is required")
	}

	sub := w.redis.Subscribe(ctx, w.queueChannel)
	defer sub.Close()

	if _, err := sub.Receive(ctx); err != nil {
		return fmt.Errorf("subscribe to queue: %w", err)
	}

	w.logger.Info("tts worker subscribed", zap.String("channel", w.queueChannel))
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return errors.New("redis subscription channel closed")
			}
			w.handleMessage(ctx, msg.Payload)
		}
	}
}

func (w *TTSWorker) handleMessage(ctx context.Context, payload string) {
	var job JobMessage
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		w.logger.Error("invalid job payload", zap.Error(err), zap.String("payload", payload))
		return
	}

	if err := w.queries.MarkGenerationProcessing(ctx, job.GenerationID); err != nil {
		w.logger.Error("failed to mark processing", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
		return
	}

	audioBytes, err := w.callModalTTS(ctx, job)
	if err != nil {
		_ = w.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{
			ID:           job.GenerationID,
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
		})
		w.publishResult(ctx, job, "failed", "", err.Error())
		w.logger.Error("tts generation failed", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
		return
	}

	s3ObjectKey := buildGeneratedAudioS3Key(job.GenerationID)
	if err := w.s3Client.Upload(ctx, generations3.UploadOptions{
		Key:         s3ObjectKey,
		Body:        audioBytes,
		ContentType: "audio/wav",
	}); err != nil {
		msg := fmt.Sprintf("failed to upload generated audio to s3: %v", err)
		_ = w.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{
			ID:           job.GenerationID,
			ErrorMessage: pgtype.Text{String: msg, Valid: true},
		})
		w.publishResult(ctx, job, "failed", "", msg)
		w.logger.Error("failed to upload generated audio", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID), zap.String("s3_key", s3ObjectKey))
		return
	}

	audioURL, err := w.s3Client.GetSignedURL(ctx, s3ObjectKey)
	if err != nil {
		msg := fmt.Sprintf("failed to sign generated audio url: %v", err)
		_ = w.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{
			ID:           job.GenerationID,
			ErrorMessage: pgtype.Text{String: msg, Valid: true},
		})
		w.publishResult(ctx, job, "failed", "", msg)
		w.logger.Error("failed to sign generated audio", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID), zap.String("s3_key", s3ObjectKey))
		return
	}

	if err := w.queries.MarkGenerationCompleted(ctx, db.MarkGenerationCompletedParams{
		ID:          job.GenerationID,
		AudioUrl:    pgtype.Text{String: audioURL, Valid: true},
		S3ObjectKey: pgtype.Text{String: s3ObjectKey, Valid: true},
	}); err != nil {
		w.logger.Error("failed to mark completed", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
		return
	}

	w.publishResult(ctx, job, "completed", audioURL, "")
	w.logger.Info("tts generation completed", zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
}

func (w *TTSWorker) callModalTTS(ctx context.Context, job JobMessage) ([]byte, error) {
	voiceKey := normalizeVoiceKey(job.VoiceKey)
	if voiceKey == "" {
		return nil, errors.New("voice_key is required")
	}
	languageID := strings.TrimSpace(job.LanguageID)
	if languageID == "" {
		languageID = "en"
	}

	body, err := json.Marshal(map[string]any{
		"prompt":       job.Text,
		"voice_key":    voiceKey,
		"language_id":  languageID,
		"temperature":  job.Temperature,
		"exaggeration": job.Exaggeration,
		"cfg_weight":   job.CfgWeight,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.ttsEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", w.ttsAPIKey)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call modal tts endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("modal tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read modal tts response: %w", err)
	}
	if len(audioBytes) == 0 {
		return nil, errors.New("modal tts returned empty audio")
	}

	return audioBytes, nil
}

func buildGeneratedAudioS3Key(generationID string) string {
	return path.Join("generated", generationID, "audio.wav")
}

func normalizeVoiceKey(raw string) string {
	key := strings.TrimSpace(raw)
	if key == "" {
		return ""
	}

	if parsed, err := url.Parse(key); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		key = parsed.Path
	}

	key = strings.TrimPrefix(strings.TrimSpace(key), "/")

	if bucket := strings.TrimSpace(getEnv("S3_BUCKET_NAME", "")); bucket != "" && strings.HasPrefix(key, bucket+"/") {
		key = strings.TrimPrefix(key, bucket+"/")
	}

	if idx := strings.Index(key, "voices/"); idx > 0 {
		key = key[idx:]
	}

	return key
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (w *TTSWorker) publishResult(ctx context.Context, job JobMessage, status string, audioURL string, errMsg string) {
	if w.resultsChannelPrefix == "" {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"generation_id": job.GenerationID,
		"job_id":        job.JobID,
		"status":        status,
		"audio_url":     audioURL,
		"error_message": errMsg,
	})
	if err != nil {
		w.logger.Warn("failed to marshal result payload", zap.Error(err), zap.String("job_id", job.JobID))
		return
	}

	channel := w.resultsChannelPrefix + job.JobID
	if err := w.redis.Publish(ctx, channel, payload).Err(); err != nil {
		w.logger.Warn("failed to publish result event", zap.Error(err), zap.String("channel", channel), zap.String("job_id", job.JobID))
	}
}
