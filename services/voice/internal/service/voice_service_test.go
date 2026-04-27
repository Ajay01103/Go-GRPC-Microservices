package service

import (
	"context"
	"testing"
	"time"

	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/s3"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type fakeRepo struct {
	getVoiceByIDFn func(ctx context.Context, id string) (db.Voice, error)
}

func (f fakeRepo) ListVoices(ctx context.Context, params repository.ListVoicesParams) ([]db.ListCustomVoicesRow, error) {
	return nil, nil
}

func (f fakeRepo) GetVoiceByID(ctx context.Context, id string) (db.Voice, error) {
	if f.getVoiceByIDFn != nil {
		return f.getVoiceByIDFn(ctx, id)
	}
	return db.Voice{}, repository.ErrVoiceNotFound
}

func (f fakeRepo) GetVoiceByIDAndUser(ctx context.Context, id, userID string) (db.Voice, error) {
	return db.Voice{}, repository.ErrVoiceNotFound
}

func (f fakeRepo) CreateVoice(ctx context.Context, params repository.CreateVoiceParams) (db.Voice, error) {
	return db.Voice{}, nil
}

func (f fakeRepo) UpdateVoice(ctx context.Context, params repository.UpdateVoiceParams) (db.Voice, error) {
	return db.Voice{}, nil
}

func (f fakeRepo) DeleteVoice(ctx context.Context, id, userID string) error {
	return nil
}

type fakeS3 struct {
	urlByKey map[string]string
	lastKey  string
}

func (f *fakeS3) Upload(ctx context.Context, opts s3.UploadOptions) error {
	return nil
}

func (f *fakeS3) Delete(ctx context.Context, key string) error {
	return nil
}

func (f *fakeS3) GetSignedURL(ctx context.Context, key string) (string, error) {
	f.lastKey = key
	if f.urlByKey == nil {
		return "", nil
	}
	return f.urlByKey[key], nil
}

func TestGetPlaybackURLReturnsProviderKey(t *testing.T) {
	t.Parallel()

	key := "system/voice-123.wav"
	svc := New(
		fakeRepo{getVoiceByIDFn: func(ctx context.Context, id string) (db.Voice, error) {
			return db.Voice{
				ID:     id,
				UserID: "SYSTEM",
				S3ObjectKey: pgtype.Text{
					String: key,
					Valid:  true,
				},
			}, nil
		}},
		&fakeS3{urlByKey: map[string]string{key: "https://signed.example/audio.wav"}},
		zap.NewNop(),
	)

	before := time.Now().Unix()
	url, expiresAt, providerKey, err := svc.GetPlaybackURL(context.Background(), "voice-123", "user-a")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if url != "https://signed.example/audio.wav" {
		t.Fatalf("unexpected signed URL: %s", url)
	}
	if providerKey != key {
		t.Fatalf("unexpected provider key: %s", providerKey)
	}
	if expiresAt <= before {
		t.Fatalf("expected future expiration, got %d", expiresAt)
	}
}

func TestGetPlaybackURLAccessDeniedForDifferentOwner(t *testing.T) {
	t.Parallel()

	svc := New(
		fakeRepo{getVoiceByIDFn: func(ctx context.Context, id string) (db.Voice, error) {
			return db.Voice{
				ID:     id,
				UserID: "owner-a",
				S3ObjectKey: pgtype.Text{
					String: "custom/owner-a/voice.wav",
					Valid:  true,
				},
			}, nil
		}},
		&fakeS3{urlByKey: map[string]string{"custom/owner-a/voice.wav": "https://signed.example/owner.wav"}},
		zap.NewNop(),
	)

	_, _, _, err := svc.GetPlaybackURL(context.Background(), "voice-321", "owner-b")
	if err != ErrVoiceAccessDenied {
		t.Fatalf("expected ErrVoiceAccessDenied, got %v", err)
	}
}
