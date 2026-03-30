package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/go-grpc-sqlc/auth/config"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/pkg/token"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
)

// AuthService holds all dependencies needed by the auth business logic.
type AuthService struct {
	userRepo   *repository.UserRepo
	tokenMaker token.TokenMaker
	tokenStore *redisstore.TokenStore
	cfg        config.Config
	logger     *zap.Logger
}

// New creates an AuthService with its dependencies wired.
func New(
	userRepo *repository.UserRepo,
	tokenMaker token.TokenMaker,
	tokenStore *redisstore.TokenStore,
	cfg config.Config,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenMaker: tokenMaker,
		tokenStore: tokenStore,
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
	// Hash password with argon2id
	hashed, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Persist user
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

	// Mint tokens
	accessToken, refreshToken, err := s.mintTokenPair(ctx, user)
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

	accessToken, refreshToken, err := s.mintTokenPair(ctx, user)
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

// RefreshToken implements the suggested secure flow:
//
//  1. Verify JWT signature + expiry              — cryptographic gate
//  2. Atomically claim (delete) the old JTI      — race-safe, reuse detection here
//  3. Fetch user from DB                         — existence check
//  4. Mint new refresh token (new JTI)
//  5. Mint new access token paired with new JTI
//  6. Store new JTI in Redis
//  7. Return token pair
//
// Steps 2–6 are ordered so the most reliable failure (Redis atomic claim) happens
// before any expensive work. If step 6 fails, the user loses their session and
// must re-login — this is an acceptable trade-off for race safety.
//
// Reuse detection: if step 2 returns ErrTokenRevoked for a token that already
// passed JWT verification, a consumed token was replayed. This signals possible
// theft, so all sessions for that user are immediately revoked.
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string) (*RefreshResult, error) {
	// 1. Verify JWT signature + expiry — fast cryptographic gate.
	payload, err := s.tokenMaker.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	oldJTI := payload.JTI.String()

	// 2. Atomically claim (delete) the old JTI from Redis.
	//    This is the race-safe gate: only one concurrent request can win this.
	//    If the key is missing, either:
	//      a) the user already logged out (normal revocation), or
	//      b) a concurrent request already claimed it (race), or
	//      c) a consumed token is being replayed (theft attempt).
	//    All three cases → refuse, and escalate (c) to full session wipe.
	if err := s.tokenStore.ClaimRefreshToken(ctx, oldJTI); err != nil {
		if errors.Is(err, redisstore.ErrTokenRevoked) {
			// A valid-signature token whose JTI is already gone was replayed.
			// This is a strong signal of theft — nuke every session for this user.
			s.logger.Warn("refresh token reuse detected — possible theft, revoking all sessions",
				zap.String("userID", payload.UserID.String()),
			)
			_ = s.tokenStore.RevokeAllUserTokens(ctx, payload.UserID.String())
			return nil, ErrTokenRevoked
		}
		return nil, fmt.Errorf("redis claim: %w", err)
	}

	// 3. Fetch the user — verify they still exist in the DB.
	user, err := s.userRepo.GetByID(ctx, payload.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 4. Mint new refresh token (generates its own JTI).
	newRefreshStr, newRefreshPayload, err := s.tokenMaker.CreateRefreshToken(
		user.ID, user.Email, user.Name,
		s.cfg.RefreshTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	// 5. Mint new access token paired with the new refresh JTI.
	newAccessStr, _, err := s.tokenMaker.CreateAccessToken(
		user.ID, user.Email, user.Name,
		newRefreshPayload.JTI.String(),
		s.cfg.AccessTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	// 6. Store the new JTI in Redis.
	//    If this fails, the old JTI is already gone (step 2) and the new one is
	//    not stored — the user's session is lost and they must re-login.
	//    This is acceptable: it's a transient Redis error, not a security issue.
	if err := s.tokenStore.StoreRefreshToken(
		ctx,
		newRefreshPayload.JTI.String(),
		user.ID,
		s.cfg.RefreshTokenDuration,
	); err != nil {
		return nil, fmt.Errorf("store new refresh token: %w", err)
	}

	return &RefreshResult{AccessToken: newAccessStr, RefreshToken: newRefreshStr}, nil
}

// ─── Logout ───────────────────────────────────────────────────────────────────

// Logout revokes the refresh token from Redis. After this, any RefreshToken call
// with the same token will be rejected.
func (s *AuthService) Logout(ctx context.Context, refreshTokenStr string) error {
	payload, err := s.tokenMaker.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		// If the token is expired or invalid, we still attempt to revoke by
		// returning success (idempotent logout).
		return nil
	}
	return s.tokenStore.RevokeRefreshToken(ctx, payload.JTI.String())
}

// ─── ValidateToken ────────────────────────────────────────────────────────────

type ValidateResult struct {
	UserID uuid.UUID
	Email  string
	Name   string
}

// ValidateToken verifies an access token (used by other services / Next.js middleware).
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

// mintTokenPair creates a refresh token, stores it in Redis, then pairs it with
// a new access token. Both tokens are returned as signed JWT strings.
func (s *AuthService) mintTokenPair(ctx context.Context, user db.User) (accessToken, refreshToken string, err error) {
	// 1. Create refresh token (generates its own JTI)
	refreshStr, refreshPayload, mintErr := s.tokenMaker.CreateRefreshToken(
		user.ID, user.Email, user.Name,
		s.cfg.RefreshTokenDuration,
	)
	if mintErr != nil {
		return "", "", fmt.Errorf("mint refresh token: %w", mintErr)
	}

	// 2. Store refresh JTI in Redis
	if storeErr := s.tokenStore.StoreRefreshToken(
		ctx,
		refreshPayload.JTI.String(),
		user.ID,
		s.cfg.RefreshTokenDuration,
	); storeErr != nil {
		return "", "", fmt.Errorf("store refresh token: %w", storeErr)
	}

	// 3. Create access token — embeds the refresh JTI as refresh_jti claim
	accessStr, _, mintErr := s.tokenMaker.CreateAccessToken(
		user.ID, user.Email, user.Name,
		refreshPayload.JTI.String(),
		s.cfg.AccessTokenDuration,
	)
	if mintErr != nil {
		return "", "", fmt.Errorf("mint access token: %w", mintErr)
	}

	return accessStr, refreshStr, nil
}
