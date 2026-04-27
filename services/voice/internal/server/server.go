package server

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	"go.uber.org/zap"

	"github.com/go-grpc-sqlc/voice/gen/pb"
	"github.com/go-grpc-sqlc/voice/gen/pb/pbconnect"
	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/service"
)

// VoiceServer implements pbconnect.VoiceServiceHandler.
type VoiceServer struct {
	pbconnect.UnimplementedVoiceServiceHandler
	svc    voiceService
	logger *zap.Logger
}

type voiceService interface {
	GetAll(ctx context.Context, params service.ListVoicesParams) ([]service.VoiceItem, error)
	Delete(ctx context.Context, id, userID string) error
	GetPlaybackURL(ctx context.Context, voiceID, requesterUserID string) (string, int64, string, error)
	CreateVoice(ctx context.Context, params service.CreateVoiceParams) (service.VoiceItem, error)
	UpdateVoice(ctx context.Context, params service.UpdateVoiceParams) (service.VoiceItem, error)
}

var userPayloadFromContext = interceptor.UserPayloadFromContext

// NewVoiceServer constructs a VoiceServer with the given service and logger.
func NewVoiceServer(svc voiceService, logger *zap.Logger) *VoiceServer {
	return &VoiceServer{svc: svc, logger: logger}
}

// GetAllVoices fetches all voices for a user, with optional search.
func (s *VoiceServer) GetAllVoices(
	ctx context.Context,
	req *connect.Request[pb.GetAllVoicesRequest],
) (*connect.Response[pb.GetAllVoicesResponse], error) {
	payload, ok := userPayloadFromContext(ctx)
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
	payload, ok := userPayloadFromContext(ctx)
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
	payload, ok := userPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	if req.Msg.VoiceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("voice id is required"))
	}

	url, expiresAt, providerKey, err := s.svc.GetPlaybackURL(ctx, req.Msg.VoiceId, payload.UserID.String())
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
		ProviderKey:   providerKey,
	}), nil
}

// CreateVoice creates a new custom voice from uploaded audio data.
func (s *VoiceServer) CreateVoice(
	ctx context.Context,
	req *connect.Request[pb.CreateVoiceRequest],
) (*connect.Response[pb.CreateVoiceResponse], error) {
	payload, ok := userPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	if strings.TrimSpace(req.Msg.Name) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(req.Msg.Language) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("language is required"))
	}
	if len(req.Msg.AudioData) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("audio_data is required"))
	}

	category, ok := categoryFromProto(req.Msg.Category)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid category"))
	}

	variant, ok := variantFromProto(req.Msg.Variant)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid variant"))
	}

	created, err := s.svc.CreateVoice(ctx, service.CreateVoiceParams{
		UserID:      payload.UserID.String(),
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		Category:    category,
		Language:    req.Msg.Language,
		Variant:     variant,
		AudioData:   req.Msg.AudioData,
		ContentType: req.Msg.ContentType,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCreateVoiceInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}

		s.logger.Error("CreateVoice failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.CreateVoiceResponse{
		Voice: &pb.VoiceItem{
			Id:          created.ID,
			Name:        created.Name,
			Description: created.Description,
			Category:    categoryToProto(created.Category),
			Language:    created.Language,
			Variant:     variantToProto(created.Variant),
		},
	}), nil
}

// UpdateVoice updates metadata on a custom voice owned by the requester.
func (s *VoiceServer) UpdateVoice(
	ctx context.Context,
	req *connect.Request[pb.UpdateVoiceRequest],
) (*connect.Response[pb.UpdateVoiceResponse], error) {
	payload, ok := userPayloadFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth payload"))
	}

	if strings.TrimSpace(req.Msg.Id) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.UserId != "" && req.Msg.UserId != payload.UserID.String() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("cannot update another user's voice"))
	}

	category, ok := categoryFromProto(req.Msg.Category)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid category"))
	}

	updated, err := s.svc.UpdateVoice(ctx, service.UpdateVoiceParams{
		ID:          req.Msg.Id,
		UserID:      payload.UserID.String(),
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		Category:    category,
		Language:    req.Msg.Language,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidUpdateVoiceInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if errors.Is(err, repository.ErrVoiceNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("voice not found"))
		}

		s.logger.Error("UpdateVoice failed", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&pb.UpdateVoiceResponse{
		Voice: &pb.VoiceItem{
			Id:          updated.ID,
			Name:        updated.Name,
			Description: updated.Description,
			Category:    categoryToProto(updated.Category),
			Language:    updated.Language,
			Variant:     variantToProto(updated.Variant),
		},
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

func categoryFromProto(c pb.VoiceCategory) (db.VoiceCategory, bool) {
	switch c {
	case pb.VoiceCategory_VOICE_CATEGORY_UNSPECIFIED:
		return db.VoiceCategoryGENERAL, true
	case pb.VoiceCategory_VOICE_CATEGORY_GENERAL:
		return db.VoiceCategoryGENERAL, true
	case pb.VoiceCategory_VOICE_CATEGORY_NARRATION:
		return db.VoiceCategoryNARRATION, true
	case pb.VoiceCategory_VOICE_CATEGORY_CHARACTER:
		return db.VoiceCategoryCHARACTER, true
	default:
		return "", false
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

func variantFromProto(v pb.VoiceVariant) (db.VoiceVariant, bool) {
	switch v {
	case pb.VoiceVariant_VOICE_VARIANT_UNSPECIFIED:
		return db.VoiceVariantNEUTRAL, true
	case pb.VoiceVariant_VOICE_VARIANT_MALE:
		return db.VoiceVariantMALE, true
	case pb.VoiceVariant_VOICE_VARIANT_FEMALE:
		return db.VoiceVariantFEMALE, true
	case pb.VoiceVariant_VOICE_VARIANT_NEUTRAL:
		return db.VoiceVariantNEUTRAL, true
	default:
		return "", false
	}
}
