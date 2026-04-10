package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	"go.uber.org/zap"

	"github.com/go-grpc-sqlc/voice/gen/pb"
	"github.com/go-grpc-sqlc/voice/gen/pb/pbconnect"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/service"
)

// VoiceServer implements pbconnect.VoiceServiceHandler.
type VoiceServer struct {
	pbconnect.UnimplementedVoiceServiceHandler
	svc    *service.VoiceService
	logger *zap.Logger
}

// NewVoiceServer constructs a VoiceServer with the given service and logger.
func NewVoiceServer(svc *service.VoiceService, logger *zap.Logger) *VoiceServer {
	return &VoiceServer{svc: svc, logger: logger}
}

// GetAllVoices fetches all voices for a user, with optional search.
func (s *VoiceServer) GetAllVoices(
	ctx context.Context,
	req *connect.Request[pb.GetAllVoicesRequest],
) (*connect.Response[pb.GetAllVoicesResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	requestedUserID := req.Msg.UserId
	if requestedUserID == "" {
		requestedUserID = payload.UserID.String()
	}
	scope := repository.ListScopeCustom

	if requestedUserID != "SYSTEM" && requestedUserID != payload.UserID.String() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("cannot access another user's voices"))
	}
	if requestedUserID == "SYSTEM" {
		scope = repository.ListScopeSystem
	}

	voices, err := s.svc.GetAll(ctx, service.ListVoicesParams{
		UserID: requestedUserID,
		Scope:  scope,
		Query:  req.Msg.Query,
	})
	if err != nil {
		s.logger.Error("GetAllVoices failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	items := make([]*pb.VoiceItem, 0, len(voices))
	for _, v := range voices {
		items = append(items, &pb.VoiceItem{
			Id:          v.ID,
			Name:        v.Name,
			Description: v.Description,
			Category:    categoryToProto(v.Category),
			Language:    v.Language,
			Variant:     variantToProto(v.Variant),
		})
	}

	return connect.NewResponse(&pb.GetAllVoicesResponse{Voices: items}), nil
}

// DeleteVoice removes a voice owned by the requesting user and cleans up S3.
func (s *VoiceServer) DeleteVoice(
	ctx context.Context,
	req *connect.Request[pb.DeleteVoiceRequest],
) (*connect.Response[pb.DeleteVoiceResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.UserId != "" && req.Msg.UserId != payload.UserID.String() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("cannot delete another user's voice"))
	}

	err := s.svc.Delete(ctx, req.Msg.Id, payload.UserID.String())
	if err != nil {
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("voice not found"))
		}
		s.logger.Error("DeleteVoice failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.DeleteVoiceResponse{Success: true}), nil
}

// GetVoicePlaybackUrl returns a short-lived signed playback URL for a voice.
func (s *VoiceServer) GetVoicePlaybackUrl(
	ctx context.Context,
	req *connect.Request[pb.GetVoicePlaybackUrlRequest],
) (*connect.Response[pb.GetVoicePlaybackUrlResponse], error) {
	payload, ok := interceptor.UserPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	if req.Msg.VoiceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("voice id is required"))
	}

	url, expiresAt, err := s.svc.GetPlaybackURL(ctx, req.Msg.VoiceId, payload.UserID.String())
	if err != nil {
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("voice not found"))
		}
		if errors.Is(err, service.ErrVoiceAccessDenied) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("cannot access this voice"))
		}
		s.logger.Error("GetVoicePlaybackUrl failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.GetVoicePlaybackUrlResponse{
		Url:           url,
		ExpiresAtUnix: expiresAt,
	}), nil
}

// ─── enum converters ──────────────────────────────────────────────────────────

func categoryToProto(s string) pb.VoiceCategory {
	switch s {
	case "NARRATION":
		return pb.VoiceCategory_VOICE_CATEGORY_NARRATION
	case "CHARACTER":
		return pb.VoiceCategory_VOICE_CATEGORY_CHARACTER
	default:
		return pb.VoiceCategory_VOICE_CATEGORY_GENERAL
	}
}

func variantToProto(s string) pb.VoiceVariant {
	switch s {
	case "FEMALE":
		return pb.VoiceVariant_VOICE_VARIANT_FEMALE
	case "NEUTRAL":
		return pb.VoiceVariant_VOICE_VARIANT_NEUTRAL
	default:
		return pb.VoiceVariant_VOICE_VARIANT_MALE
	}
}
