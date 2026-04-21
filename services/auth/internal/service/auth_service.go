package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/go-grpc-sqlc/auth/config"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
	"github.com/go-grpc-sqlc/pkg/dpop"
	"github.com/go-grpc-sqlc/pkg/token"
)

// AuthService holds all dependencies needed by the auth business logic.
type AuthService struct {
	userRepo   *repository.UserRepo
	tokenMaker token.TokenMaker
	tokenStore *redisstore.TokenStore
	dpopStore  *dpop.DPoPStore
	cfg        config.Config
	logger     *zap.Logger
}

// New creates an AuthService with its dependencies wired.
func New(
	userRepo *repository.UserRepo,
	tokenMaker token.TokenMaker,
	tokenStore *redisstore.TokenStore,
	dpopStore *dpop.DPoPStore,
	cfg config.Config,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenMaker: tokenMaker,
		tokenStore: tokenStore,
		dpopStore:  dpopStore,
		cfg:        cfg,
		logger:     logger,
	}
}

// ─── Register ─────────────────────────────────────────────────────────────────

type RegisterResult struct {
	User         db.User
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) Register(ctx context.Context, email, name, password string) (*RegisterResult, error) {
	hashed, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.userRepo.CreateUser(ctx, email, name, hashed)
	if err != nil {
		if errors.Is(err, repository.ErrEmailTaken) {
			return nil, ErrEmailAlreadyExists
		}
		if errors.Is(err, repository.ErrNameTaken) {
			return nil, ErrNameAlreadyExists
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	accessToken, refreshToken, err := s.mintTokenPair(ctx, user, uuid.NewString(), "")
	if err != nil {
		return nil, err
	}

	return &RegisterResult{User: user, AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

// ─── Login ────────────────────────────────────────────────────────────────────

type LoginResult struct {
	User         db.User
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := VerifyPassword(user.Password, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, refreshToken, err := s.mintTokenPair(ctx, user, uuid.NewString(), "")
	if err != nil {
		return nil, err
	}

	return &LoginResult{User: user, AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

// ─── RefreshToken ─────────────────────────────────────────────────────────────

type RefreshResult struct {
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr, dpopProof, dpopKeyThumbprint string) (*RefreshResult, error) {
	payload, err := s.tokenMaker.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	oldTokenHash := redisstore.HashTokenSHA256(refreshTokenStr)
	blacklisted, err := s.tokenStore.IsTokenHashBlacklisted(ctx, oldTokenHash)
	if err != nil {
		return nil, fmt.Errorf("check blacklist: %w", err)
	}
	if blacklisted {
		graceFamilyID, graceErr := s.tokenStore.GetRotatedTokenGraceFamilyID(ctx, oldTokenHash)
		if graceErr == nil && graceFamilyID == payload.FamilyID.String() {
			s.logger.Info("concurrent refresh stale token hit grace window",
				zap.String("userID", payload.UserID.String()),
				zap.String("familyID", payload.FamilyID.String()),
			)
			return nil, ErrTokenExpired
		}
		if graceErr != nil && !errors.Is(graceErr, redisstore.ErrGraceNotFound) {
			s.logger.Warn("failed to check rotated grace key",
				zap.String("userID", payload.UserID.String()),
				zap.String("familyID", payload.FamilyID.String()),
				zap.Error(graceErr),
			)
		}

		s.logger.Warn("blacklisted refresh token reuse detected",
			zap.String("userID", payload.UserID.String()),
			zap.String("familyID", payload.FamilyID.String()),
		)
		s.nukeFamily(ctx, payload.UserID.String(), payload.FamilyID.String())
		return nil, ErrTokenReuseDetected
	}

	kid, err := s.tokenStore.GetFamilyKID(ctx, payload.FamilyID.String())
	if err != nil {
		if errors.Is(err, redisstore.ErrFamilyNotFound) {
			return nil, ErrRefreshFamilyMissing
		}
		return nil, fmt.Errorf("get family kid: %w", err)
	}

	if dpopProof != "" && s.dpopStore != nil {
		proofKey := hashDPoPProof(dpopProof)
		proofUsed, proofErr := s.dpopStore.IsProofUsed(ctx, proofKey)
		if proofErr != nil {
			return nil, fmt.Errorf("check dpop proof replay: %w", proofErr)
		}
		if proofUsed {
			s.logger.Warn("dpop proof replay detected",
				zap.String("userID", payload.UserID.String()),
				zap.String("familyID", payload.FamilyID.String()),
			)
			s.nukeFamily(ctx, payload.UserID.String(), payload.FamilyID.String())
			return nil, ErrDPoPProofReplayed
		}
		if proofErr = s.dpopStore.RecordProof(ctx, proofKey, 60*time.Second); proofErr != nil {
			return nil, fmt.Errorf("record dpop proof replay key: %w", proofErr)
		}
	}

	presentedJKT := dpopKeyThumbprint
	if presentedJKT == "" {
		presentedJKT = payload.DPoPKeyThumbprint
	}

	if payload.DPoPKeyThumbprint != "" && presentedJKT != payload.DPoPKeyThumbprint {
		s.logger.Warn("refresh key substitution detected",
			zap.String("userID", payload.UserID.String()),
			zap.String("familyID", payload.FamilyID.String()),
		)
		s.nukeFamily(ctx, payload.UserID.String(), payload.FamilyID.String())
		return nil, ErrKeyBindingMismatch
	}

	user, err := s.userRepo.GetByID(ctx, payload.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	newRefreshStr, newRefreshPayload, err := s.tokenMaker.CreateRefreshToken(
		user.ID,
		user.Email,
		user.Name,
		payload.FamilyID.String(),
		presentedJKT,
		s.cfg.RefreshTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	newRecord := redisstore.ActiveRefreshTokenRecord{
		UserID:     user.ID,
		TokenHash:  redisstore.HashTokenSHA256(newRefreshStr),
		JKT:        presentedJKT,
		ExpiresAt:  newRefreshPayload.ExpiredAt.UTC().Format(time.RFC3339Nano),
		RefreshJTI: newRefreshPayload.JTI.String(),
		SigningKID: s.tokenMaker.GetCurrentKeyID(),
		IssuedAt:   time.Now().Unix(),
	}

	outcome, err := s.tokenStore.RotateFamilyActiveToken(
		ctx,
		payload.FamilyID.String(),
		user.ID,
		oldTokenHash,
		payload.DPoPKeyThumbprint,
		kid,
		newRecord,
		s.cfg.RefreshTokenDuration,
		s.cfg.RefreshTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token family: %w", err)
	}

	switch outcome {
	case redisstore.RotateSuccess:
		// happy path
	case redisstore.RotateFamilyNotFound:
		s.logger.Warn("refresh family missing during rotation",
			zap.String("userID", user.ID),
			zap.String("familyID", payload.FamilyID.String()),
		)
		return nil, ErrRefreshFamilyMissing
	case redisstore.RotateJKTMismatch:
		s.logger.Warn("refresh key binding mismatch during rotation",
			zap.String("userID", user.ID),
			zap.String("familyID", payload.FamilyID.String()),
		)
		s.nukeFamily(ctx, user.ID, payload.FamilyID.String())
		return nil, ErrKeyBindingMismatch
	case redisstore.RotateKIDMismatch:
		s.logger.Warn("key was rotated and this token is from the old key",
			zap.String("userID", user.ID),
			zap.String("familyID", payload.FamilyID.String()),
		)
		return nil, ErrInvalidToken
	default:
		s.logger.Warn("refresh token reuse detected",
			zap.String("userID", user.ID),
			zap.String("familyID", payload.FamilyID.String()),
			zap.String("outcome", string(outcome)),
		)
		s.nukeFamily(ctx, user.ID, payload.FamilyID.String())
		return nil, ErrTokenReuseDetected
	}

	newAccessStr, _, err := s.tokenMaker.CreateAccessToken(
		user.ID,
		user.Email,
		user.Name,
		payload.FamilyID.String(),
		newRefreshPayload.JTI.String(),
		presentedJKT,
		s.cfg.AccessTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	return &RefreshResult{AccessToken: newAccessStr, RefreshToken: newRefreshStr}, nil
}

// ─── Logout ───────────────────────────────────────────────────────────────────

func (s *AuthService) Logout(ctx context.Context, refreshTokenStr string) error {
	payload, err := s.tokenMaker.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return ErrTokenExpired
		}
		return ErrInvalidToken
	}
	tokenHash := redisstore.HashTokenSHA256(refreshTokenStr)
	if err := s.tokenStore.LogoutFamily(ctx, payload.UserID.String(), payload.FamilyID.String(), tokenHash, s.cfg.RefreshTokenDuration); err != nil {
		return err
	}
	return nil
}

func (s *AuthService) LogoutAllDevices(ctx context.Context, userID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return ErrInvalidToken
	}
	if err := s.tokenStore.RevokeAllUserFamilies(ctx, userID, s.cfg.RefreshTokenDuration); err != nil {
		return fmt.Errorf("revoke all families: %w", err)
	}
	return nil
}

// ─── ValidateToken ────────────────────────────────────────────────────────────

type ValidateResult struct {
	UserID uuid.UUID
	Email  string
	Name   string
}

func (s *AuthService) ValidateToken(ctx context.Context, accessTokenStr string) (*ValidateResult, error) {
	payload, err := s.tokenMaker.VerifyAccessToken(accessTokenStr)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	return &ValidateResult{
		UserID: payload.UserID,
		Email:  payload.Email,
		Name:   payload.Name,
	}, nil
}

// ─── Private helpers ──────────────────────────────────────────────────────────

func (s *AuthService) mintTokenPair(ctx context.Context, user db.User, familyID, dpopKeyThumbprint string) (accessToken, refreshToken string, err error) {
	refreshStr, refreshPayload, mintErr := s.tokenMaker.CreateRefreshToken(
		user.ID,
		user.Email,
		user.Name,
		familyID,
		dpopKeyThumbprint,
		s.cfg.RefreshTokenDuration,
	)
	if mintErr != nil {
		return "", "", fmt.Errorf("mint refresh token: %w", mintErr)
	}

	if storeErr := s.tokenStore.StoreFamilyActiveToken(
		ctx,
		familyID,
		redisstore.ActiveRefreshTokenRecord{
			UserID:     user.ID,
			TokenHash:  redisstore.HashTokenSHA256(refreshStr),
			JKT:        dpopKeyThumbprint,
			ExpiresAt:  refreshPayload.ExpiredAt.UTC().Format(time.RFC3339Nano),
			RefreshJTI: refreshPayload.JTI.String(),
			SigningKID: s.tokenMaker.GetCurrentKeyID(),
			IssuedAt:   time.Now().Unix(),
		},
		s.cfg.RefreshTokenDuration,
	); storeErr != nil {
		return "", "", fmt.Errorf("store refresh token: %w", storeErr)
	}

	accessStr, _, mintErr := s.tokenMaker.CreateAccessToken(
		user.ID,
		user.Email,
		user.Name,
		familyID,
		refreshPayload.JTI.String(),
		dpopKeyThumbprint,
		s.cfg.AccessTokenDuration,
	)
	if mintErr != nil {
		return "", "", fmt.Errorf("mint access token: %w", mintErr)
	}

	return accessStr, refreshStr, nil
}

func (s *AuthService) nukeFamily(ctx context.Context, userID, familyID string) {
	if err := s.tokenStore.RevokeFamily(ctx, familyID, s.cfg.RefreshTokenDuration); err != nil {
		s.logger.Warn("failed to revoke family", zap.String("familyID", familyID), zap.Error(err))
	}
	if err := s.tokenStore.RemoveFamilyFromUser(ctx, userID, familyID); err != nil {
		s.logger.Warn("failed to remove family from user set", zap.String("familyID", familyID), zap.String("userID", userID), zap.Error(err))
	}
}

func hashDPoPProof(proof string) string {
	sum := sha256.Sum256([]byte(proof))
	return hex.EncodeToString(sum[:])
}
