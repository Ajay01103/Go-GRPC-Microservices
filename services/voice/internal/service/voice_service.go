package service

import (
	"context"
	"errors"
	"fmt"

	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/s3"
	"go.uber.org/zap"
)

// ListVoicesParams bundles inputs for the GetAll operation.
type ListVoicesParams struct {
	UserID string
	Scope  repository.ListScope
	Query  string
}

// VoiceItem is the service-layer representation (no S3 key exposed).
type VoiceItem struct {
	ID          string
	Name        string
	Description string
	Category    string
	Language    string
	Variant     string
}

// VoiceService handles all business logic for voices.
type VoiceService struct {
	repo   repository.Repository
	s3     *s3.Client
	logger *zap.Logger
}

// New constructs a VoiceService.
func New(repo repository.Repository, s3Client *s3.Client, logger *zap.Logger) *VoiceService {
	return &VoiceService{
		repo:   repo,
		s3:     s3Client,
		logger: logger,
	}
}

// GetAll returns all voices for the given user, optionally filtered by query.
func (s *VoiceService) GetAll(ctx context.Context, params ListVoicesParams) ([]VoiceItem, error) {
	rows, err := s.repo.ListVoices(ctx, repository.ListVoicesParams{
		UserID: params.UserID,
		Scope:  params.Scope,
		Query:  params.Query,
	})
	if err != nil {
		return nil, fmt.Errorf("service: get all voices: %w", err)
	}

	items := make([]VoiceItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, rowToItem(r))
	}
	return items, nil
}

// Delete removes a voice owned by userID and cleans up its S3 object if present.
func (s *VoiceService) Delete(ctx context.Context, id, userID string) error {
	voice, err := s.repo.GetVoiceByIDAndUser(ctx, id, userID)
	if err != nil {
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return repository.ErrVoiceNotFound
		}
		return fmt.Errorf("service: fetch voice for delete: %w", err)
	}

	if err := s.repo.DeleteVoice(ctx, voice.ID, userID); err != nil {
		return fmt.Errorf("service: delete voice record: %w", err)
	}

	// Best-effort S3 cleanup — log failures but don't surface them.
	if voice.S3ObjectKey.Valid && voice.S3ObjectKey.String != "" {
		if err := s.s3.Delete(ctx, voice.S3ObjectKey.String); err != nil {
			s.logger.Warn("s3 cleanup failed after voice delete",
				zap.String("voice_id", id),
				zap.String("s3_key", voice.S3ObjectKey.String),
				zap.Error(err),
			)
		}
	}

	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func rowToItem(r db.ListCustomVoicesRow) VoiceItem {
	desc := ""
	if r.Description.Valid {
		desc = r.Description.String
	}
	return VoiceItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: desc,
		Category:    string(r.Category),
		Language:    r.Language,
		Variant:     string(r.Variant),
	}
}
