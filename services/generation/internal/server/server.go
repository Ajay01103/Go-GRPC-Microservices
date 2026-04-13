package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/generation/gen/pb"
	"github.com/go-grpc-sqlc/generation/gen/pb/pbconnect"
	db "github.com/go-grpc-sqlc/generation/internal/db"
	generations3 "github.com/go-grpc-sqlc/generation/internal/s3"
	"github.com/go-grpc-sqlc/generation/internal/worker"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const textMaxLength = 5000

type GenerationServer struct {
	pbconnect.UnimplementedGenerationServiceHandler
	queries      db.Querier
	redis        *redis.Client
	s3Client     *generations3.Client
	queueChannel string
	logger       *zap.Logger
}

func NewGenerationServer(queries db.Querier, redisClient *redis.Client, s3Client *generations3.Client, queueChannel string, logger *zap.Logger) *GenerationServer {
	return &GenerationServer{
		queries:      queries,
		redis:        redisClient,
		s3Client:     s3Client,
		queueChannel: strings.TrimSpace(queueChannel),
		logger:       logger,
	}
}

func (s *GenerationServer) GetGeneration(ctx context.Context, req *connect.Request[pb.GetGenerationRequest]) (*connect.Response[pb.GetGenerationResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	id := strings.TrimSpace(req.Msg.Id)
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	generation, err := s.queries.GetGenerationByIDAndUser(ctx, db.GetGenerationByIDAndUserParams{
		ID:     id,
		UserID: payload.UserID.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("generation not found"))
		}
		s.logger.Error("GetGeneration failed", zap.Error(err), zap.String("generation_id", id))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get generation"))
	}

	var s3ObjectKey string
	if generation.S3ObjectKey.Valid && generation.S3ObjectKey.String != "" {
		s3ObjectKey = generation.S3ObjectKey.String
	}

	return connect.NewResponse(&pb.GetGenerationResponse{
		Id:                generation.ID,
		JobId:             generation.JobID,
		VoiceId:           generation.VoiceID.String,
		VoiceName:         generation.VoiceName,
		Text:              generation.Text,
		Temperature:       generation.Temperature,
		LanguageId:        generation.LanguageID,
		Exaggeration:      generation.Exaggeration,
		CfgWeight:         generation.CfgWeight,
		S3ObjectKey:       s3ObjectKey,
		Status:            statusToProto(generation.Status),
		ErrorMessage:      generation.ErrorMessage.String,
		CreatedAtUnix:     generation.CreatedAt.Time.Unix(),
		UpdatedAtUnix:     generation.UpdatedAt.Time.Unix(),
	}), nil
}

func (s *GenerationServer) ListGenerations(ctx context.Context, _ *connect.Request[pb.ListGenerationsRequest]) (*connect.Response[pb.ListGenerationsResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	rows, err := s.queries.ListGenerationsByUser(ctx, payload.UserID.String())
	if err != nil {
		s.logger.Error("ListGenerations failed", zap.Error(err), zap.String("user_id", payload.UserID.String()))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list generations"))
	}

	items := make([]*pb.GenerationItem, 0, len(rows))
	for _, row := range rows {
		var s3ObjectKey string
		if row.S3ObjectKey.Valid && row.S3ObjectKey.String != "" {
			s3ObjectKey = row.S3ObjectKey.String
		}

		items = append(items, &pb.GenerationItem{
			Id:                row.ID,
			JobId:             row.JobID,
			VoiceId:           row.VoiceID.String,
			VoiceName:         row.VoiceName,
			Text:              row.Text,
			Temperature:       row.Temperature,
			LanguageId:        row.LanguageID,
			Exaggeration:      row.Exaggeration,
			CfgWeight:         row.CfgWeight,
			S3ObjectKey:       s3ObjectKey,
			Status:            statusToProto(row.Status),
			CreatedAtUnix:     row.CreatedAt.Time.Unix(),
			UpdatedAtUnix:     row.UpdatedAt.Time.Unix(),
		})
	}

	return connect.NewResponse(&pb.ListGenerationsResponse{Generations: items}), nil
}

func (s *GenerationServer) GenerateSpeech(ctx context.Context, req *connect.Request[pb.GenerateSpeechRequest]) (*connect.Response[pb.GenerateSpeechResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}
	if s.redis == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("redis queue not initialized"))
	}
	if s.queueChannel == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("queue channel is not configured"))
	}

	text := strings.TrimSpace(req.Msg.Text)
	if text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text is required"))
	}
	if len(text) > textMaxLength {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text exceeds maximum length"))
	}

	voiceID := strings.TrimSpace(req.Msg.VoiceId)
	if voiceID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("voice_id is required"))
	}

	voiceName := strings.TrimSpace(req.Msg.VoiceName)
	if voiceName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("voice_name is required"))
	}

	voiceKey := strings.TrimSpace(req.Msg.VoiceKey)
	if voiceKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("voice_key is required"))
	}

	languageID := strings.TrimSpace(req.Msg.LanguageId)
	if languageID == "" {
		languageID = "en"
	}

	if req.Msg.Exaggeration < 0.25 || req.Msg.Exaggeration > 2.0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("exaggeration must be between 0.25 and 2.0"))
	}

	if req.Msg.CfgWeight < 0.2 || req.Msg.CfgWeight > 1.0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cfg_weight must be between 0.2 and 1.0"))
	}

	id := uuid.NewString()
	jobID := id
	_, err := s.queries.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		ID:                id,
		JobID:             jobID,
		UserID:            payload.UserID.String(),
		VoiceID:           pgtype.Text{String: voiceID, Valid: true},
		VoiceName:         voiceName,
		VoiceKey:          voiceKey,
		Text:              text,
		Temperature:       req.Msg.Temperature,
		LanguageID:        languageID,
		Exaggeration:      req.Msg.Exaggeration,
		CfgWeight:         req.Msg.CfgWeight,
		Status:            "queued",
	})
	if err != nil {
		s.logger.Error("GenerateSpeech insert failed", zap.Error(err), zap.String("user_id", payload.UserID.String()))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enqueue generation"))
	}

	jobPayload := worker.JobMessage{
		GenerationID:      id,
		JobID:             jobID,
		UserID:            payload.UserID.String(),
		Text:              text,
		VoiceKey:          voiceKey,
		LanguageID:        languageID,
		Temperature:       req.Msg.Temperature,
		Exaggeration:      req.Msg.Exaggeration,
		CfgWeight:         req.Msg.CfgWeight,
	}
	payloadBytes, err := json.Marshal(jobPayload)
	if err != nil {
		_ = s.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{ID: id, ErrorMessage: pgtype.Text{String: "failed to enqueue job", Valid: true}})
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enqueue generation"))
	}

	if err := s.redis.Publish(ctx, s.queueChannel, payloadBytes).Err(); err != nil {
		_ = s.queries.MarkGenerationFailed(ctx, db.MarkGenerationFailedParams{ID: id, ErrorMessage: pgtype.Text{String: "failed to publish job", Valid: true}})
		s.logger.Error("failed to publish job", zap.Error(err), zap.String("generation_id", id), zap.String("job_id", jobID))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enqueue generation"))
	}

	return connect.NewResponse(&pb.GenerateSpeechResponse{
		GenerationId: id,
		JobId:        jobID,
		Status:       pb.GenerationJobStatus_GENERATION_JOB_STATUS_QUEUED,
	}), nil
}

func (s *GenerationServer) GetJobStatus(ctx context.Context, req *connect.Request[pb.GetJobStatusRequest]) (*connect.Response[pb.GetJobStatusResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	jobID := strings.TrimSpace(req.Msg.JobId)
	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	generation, err := s.queries.GetGenerationByJobIDAndUser(ctx, db.GetGenerationByJobIDAndUserParams{
		JobID:  jobID,
		UserID: payload.UserID.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("job not found"))
		}
		s.logger.Error("GetJobStatus failed", zap.Error(err), zap.String("job_id", jobID))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get job status"))
	}

	var s3ObjectKey string
	if generation.S3ObjectKey.Valid && generation.S3ObjectKey.String != "" {
		s3ObjectKey = generation.S3ObjectKey.String
	}

	return connect.NewResponse(&pb.GetJobStatusResponse{
		GenerationId: generation.ID,
		JobId:        generation.JobID,
		Status:       statusToProto(generation.Status),
		S3ObjectKey:  s3ObjectKey,
		ErrorMessage: generation.ErrorMessage.String,
		UpdatedAtUnix: generation.UpdatedAt.Time.Unix(),
	}), nil
}

func statusToProto(status string) pb.GenerationJobStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return pb.GenerationJobStatus_GENERATION_JOB_STATUS_QUEUED
	case "processing":
		return pb.GenerationJobStatus_GENERATION_JOB_STATUS_PROCESSING
	case "completed":
		return pb.GenerationJobStatus_GENERATION_JOB_STATUS_COMPLETED
	case "failed":
		return pb.GenerationJobStatus_GENERATION_JOB_STATUS_FAILED
	default:
		return pb.GenerationJobStatus_GENERATION_JOB_STATUS_UNSPECIFIED
	}
}
