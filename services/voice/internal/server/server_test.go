package server

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/pkg/token"
	"github.com/go-grpc-sqlc/voice/gen/pb"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeVoiceService struct {
	getPlaybackURLFn func(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error)
}

func (f fakeVoiceService) GetAll(ctx context.Context, params service.ListVoicesParams) ([]service.VoiceItem, error) {
	return nil, nil
}

func (f fakeVoiceService) Delete(ctx context.Context, id, userID string) error {
	return nil
}

func (f fakeVoiceService) GetPlaybackURL(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error) {
	if f.getPlaybackURLFn != nil {
		return f.getPlaybackURLFn(ctx, voiceID, requesterUserID)
	}
	return "", 0, "", errors.New("not implemented")
}

func (f fakeVoiceService) CreateVoice(ctx context.Context, params service.CreateVoiceParams) (service.VoiceItem, error) {
	return service.VoiceItem{}, nil
}

func (f fakeVoiceService) UpdateVoice(ctx context.Context, params service.UpdateVoiceParams) (service.VoiceItem, error) {
	return service.VoiceItem{}, nil
}

func TestGetVoicePlaybackUrlIncludesProviderKey(t *testing.T) {
	original := userPayloadFromContext
	defer func() { userPayloadFromContext = original }()

	userID := uuid.New()
	userPayloadFromContext = func(ctx context.Context) (*token.AccessPayload, bool) {
		return &token.AccessPayload{UserID: userID}, true
	}

	srv := NewVoiceServer(fakeVoiceService{getPlaybackURLFn: func(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error) {
		if voiceID != "voice-1" {
			t.Fatalf("unexpected voice ID %s", voiceID)
		}
		if requesterUserID != userID.String() {
			t.Fatalf("unexpected requester user id %s", requesterUserID)
		}
		return "https://signed.example/voice.wav", 1700000000, "system/voice-1.wav", nil
	}}, zap.NewNop())

	resp, err := srv.GetVoicePlaybackUrl(context.Background(), connect.NewRequest(&pb.GetVoicePlaybackUrlRequest{VoiceId: "voice-1"}))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Msg.Url != "https://signed.example/voice.wav" {
		t.Fatalf("unexpected URL: %s", resp.Msg.Url)
	}
	if resp.Msg.ProviderKey != "system/voice-1.wav" {
		t.Fatalf("unexpected provider key: %s", resp.Msg.ProviderKey)
	}
	if resp.Msg.ExpiresAtUnix != 1700000000 {
		t.Fatalf("unexpected expires_at_unix: %d", resp.Msg.ExpiresAtUnix)
	}
}

func TestGetVoicePlaybackUrlMapsAccessDenied(t *testing.T) {
	original := userPayloadFromContext
	defer func() { userPayloadFromContext = original }()

	userPayloadFromContext = func(ctx context.Context) (*token.AccessPayload, bool) {
		return &token.AccessPayload{UserID: uuid.New()}, true
	}

	srv := NewVoiceServer(fakeVoiceService{getPlaybackURLFn: func(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error) {
		return "", 0, "", service.ErrVoiceAccessDenied
	}}, zap.NewNop())

	_, err := srv.GetVoicePlaybackUrl(context.Background(), connect.NewRequest(&pb.GetVoicePlaybackUrlRequest{VoiceId: "voice-2"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %s", connect.CodeOf(err))
	}
}

func TestGetVoicePlaybackUrlMapsNotFound(t *testing.T) {
	original := userPayloadFromContext
	defer func() { userPayloadFromContext = original }()

	userPayloadFromContext = func(ctx context.Context) (*token.AccessPayload, bool) {
		return &token.AccessPayload{UserID: uuid.New()}, true
	}

	srv := NewVoiceServer(fakeVoiceService{getPlaybackURLFn: func(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error) {
		return "", 0, "", repository.ErrVoiceNotFound
	}}, zap.NewNop())

	_, err := srv.GetVoicePlaybackUrl(context.Background(), connect.NewRequest(&pb.GetVoicePlaybackUrlRequest{VoiceId: "voice-404"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected not found, got %s", connect.CodeOf(err))
	}
}

var _ voiceService = fakeVoiceService{}
