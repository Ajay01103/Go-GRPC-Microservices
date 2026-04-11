package service

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

// CreateVoiceParams bundles inputs for custom voice creation.
type CreateVoiceParams struct {
	UserID      string
	Name        string
	Description string
	Category    db.VoiceCategory
	Language    string
	AudioData   []byte
	ContentType string
}

// VoiceService handles all business logic for voices.
type VoiceService struct {
	repo   repository.Repository
	s3     *s3.Client
	logger *zap.Logger
}

var ErrVoiceAccessDenied = errors.New("voice access denied")
var ErrInvalidCreateVoiceInput = errors.New("invalid create voice input")

const maxCreateVoiceAudioBytes = 20 * 1024 * 1024

// New constructs a VoiceService.
func New(repo repository.Repository, s3Client *s3.Client, logger *zap.Logger) *VoiceService {
	return &VoiceService{
		repo:   repo,
		s3:     s3Client,
		logger: logger,
	}
}

// GetPlaybackURL returns a short-lived signed URL for a voice audio object.
func (s *VoiceService) GetPlaybackURL(ctx context.Context, voiceID, requesterUserID string) (string, int64, error) {
	voice, err := s.repo.GetVoiceByID(ctx, voiceID)
	if err != nil {
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return "", 0, repository.ErrVoiceNotFound
		}
		return "", 0, fmt.Errorf("service: fetch voice for playback: %w", err)
	}

	if voice.UserID != requesterUserID && voice.UserID != "SYSTEM" {
		return "", 0, ErrVoiceAccessDenied
	}

	if !voice.S3ObjectKey.Valid || voice.S3ObjectKey.String == "" {
		return "", 0, repository.ErrVoiceNotFound
	}

	url, err := s.s3.GetSignedURL(ctx, voice.S3ObjectKey.String)
	if err != nil {
		return "", 0, fmt.Errorf("service: sign playback url: %w", err)
	}

	return url, time.Now().Add(time.Hour).Unix(), nil
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

// CreateVoice uploads a user audio sample and stores its metadata.
func (s *VoiceService) CreateVoice(ctx context.Context, params CreateVoiceParams) (VoiceItem, error) {
	name := strings.TrimSpace(params.Name)
	language := strings.TrimSpace(params.Language)

	if strings.TrimSpace(params.UserID) == "" || name == "" || language == "" {
		return VoiceItem{}, ErrInvalidCreateVoiceInput
	}
	if len(params.AudioData) == 0 || len(params.AudioData) > maxCreateVoiceAudioBytes {
		return VoiceItem{}, ErrInvalidCreateVoiceInput
	}
	if params.Category == "" {
		params.Category = db.VoiceCategoryGENERAL
	}

	voiceID := uuid.NewString()
	s3Key := buildS3ObjectKey(params.UserID, voiceID, params.ContentType)

	if err := s.s3.Upload(ctx, s3.UploadOptions{
		Key:         s3Key,
		Body:        params.AudioData,
		ContentType: normalizeContentType(params.ContentType),
	}); err != nil {
		return VoiceItem{}, fmt.Errorf("service: upload voice audio: %w", err)
	}

	created, err := s.repo.CreateVoice(ctx, repository.CreateVoiceParams{
		ID:          voiceID,
		UserID:      params.UserID,
		Name:        name,
		Description: strings.TrimSpace(params.Description),
		Category:    params.Category,
		Language:    language,
		Variant:     db.VoiceVariantNEUTRAL,
		S3ObjectKey: s3Key,
	})
	if err != nil {
		if cleanupErr := s.s3.Delete(ctx, s3Key); cleanupErr != nil {
			s.logger.Warn("failed to cleanup uploaded audio after create error",
				zap.String("voice_id", voiceID),
				zap.String("s3_key", s3Key),
				zap.Error(cleanupErr),
			)
		}
		return VoiceItem{}, fmt.Errorf("service: create voice record: %w", err)
	}

	return VoiceItem{
		ID:          created.ID,
		Name:        created.Name,
		Description: textOrEmpty(created.Description),
		Category:    string(created.Category),
		Language:    created.Language,
		Variant:     string(created.Variant),
	}, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func rowToItem(r db.ListCustomVoicesRow) VoiceItem {
	desc := textOrEmpty(r.Description)
	return VoiceItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: desc,
		Category:    string(r.Category),
		Language:    r.Language,
		Variant:     string(r.Variant),
	}
}

func textOrEmpty(v pgtype.Text) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func buildS3ObjectKey(userID, voiceID, contentType string) string {
	ext := extensionFromContentType(contentType)
	return path.Join("voices", userID, fmt.Sprintf("%s%s", voiceID, ext))
}

func extensionFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "audio/webm":
		return ".webm"
	case "audio/x-wav", "audio/wav":
		return ".wav"
	case "audio/mp4", "audio/aac":
		return ".m4a"
	default:
		return ".wav"
	}
}

func normalizeContentType(contentType string) string {
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		return "audio/wav"
	}
	return ct
}
