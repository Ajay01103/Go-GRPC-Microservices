package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	db "github.com/go-grpc-sqlc/generation/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type JobMessage struct {
	GenerationID      string  `json:"generation_id"`
	JobID             string  `json:"job_id"`
	UserID            string  `json:"user_id"`
	Text              string  `json:"text"`
	VoiceKey          string  `json:"voice_key"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int32   `json:"top_k"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	NormalizeLoudness bool    `json:"norm_loudness"`
}

type TTSWorker struct {
	redis                *redis.Client
	queries              db.Querier
	logger               *zap.Logger
	httpClient           *http.Client
	ttsEndpoint          string
	ttsAPIKey            string
	queueChannel         string
	resultsChannelPrefix string
	audioBaseURL         string
}

func New(
	redisClient *redis.Client,
	queries db.Querier,
	logger *zap.Logger,
	ttsEndpoint string,
	ttsAPIKey string,
	queueChannel string,
	resultsChannelPrefix string,
	audioBaseURL string,
) *TTSWorker {
	return &TTSWorker{
		redis:                redisClient,
		queries:              queries,
		logger:               logger,
		httpClient:           &http.Client{Timeout: 90 * time.Second},
		ttsEndpoint:          strings.TrimSpace(ttsEndpoint),
		ttsAPIKey:            strings.TrimSpace(ttsAPIKey),
		queueChannel:         strings.TrimSpace(queueChannel),
		resultsChannelPrefix: strings.TrimSpace(resultsChannelPrefix),
		audioBaseURL:         strings.TrimRight(strings.TrimSpace(audioBaseURL), "/"),
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

	if err := w.callModalTTS(ctx, job); err != nil {
		_ = w.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{
			ID:           job.GenerationID,
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
		})
		w.publishResult(ctx, job, "failed", "", err.Error())
		w.logger.Error("tts generation failed", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
		return
	}

	audioURL := w.audioBaseURL + "/" + job.GenerationID
	if err := w.queries.MarkGenerationCompleted(ctx, db.MarkGenerationCompletedParams{
		ID:       job.GenerationID,
		AudioUrl: pgtype.Text{String: audioURL, Valid: true},
	}); err != nil {
		w.logger.Error("failed to mark completed", zap.Error(err), zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
		return
	}

	w.publishResult(ctx, job, "completed", audioURL, "")
	w.logger.Info("tts generation completed", zap.String("generation_id", job.GenerationID), zap.String("job_id", job.JobID))
}

func (w *TTSWorker) callModalTTS(ctx context.Context, job JobMessage) error {
	body, err := json.Marshal(map[string]any{
		"prompt":             job.Text,
		"voice_key":          job.VoiceKey,
		"temperature":        job.Temperature,
		"top_p":              job.TopP,
		"top_k":              job.TopK,
		"repetition_penalty": job.RepetitionPenalty,
		"norm_loudness":      job.NormalizeLoudness,
	})
	if err != nil {
		return fmt.Errorf("marshal tts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.ttsEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", w.ttsAPIKey)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call modal tts endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("modal tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read modal tts response: %w", err)
	}

	return nil
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
