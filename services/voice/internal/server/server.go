package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
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
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	voices, err := s.svc.GetAll(ctx, service.ListVoicesParams{
		UserID: req.Msg.UserId,
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
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	err := s.svc.Delete(ctx, req.Msg.Id, req.Msg.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("voice not found"))
		}
		s.logger.Error("DeleteVoice failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.DeleteVoiceResponse{Success: true}), nil
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
