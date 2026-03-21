package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/go-grpc-sqlc/auth/config"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
	"github.com/go-grpc-sqlc/auth/internal/token"
)

// AuthService holds all dependencies needed by the auth business logic.
type AuthService struct {
	userRepo   *repository.UserRepo
	tokenMaker token.TokenMaker
	tokenStore *redisstore.TokenStore
	cfg        config.Config
}

// New creates an AuthService with its dependencies wired.
func New(
	userRepo *repository.UserRepo,
	tokenMaker token.TokenMaker,
	tokenStore *redisstore.TokenStore,
	cfg config.Config,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenMaker: tokenMaker,
		tokenStore: tokenStore,
		cfg:        cfg,
	}
}

// ─── Register ─────────────────────────────────────────────────────────────────

type RegisterResult struct {
	User         db.User
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) Register(ctx context.Context, email, name, password string) (*RegisterResult, error) {
	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Persist user
	user, err := s.userRepo.CreateUser(ctx, email, name, string(hashed))
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

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
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

// RefreshToken validates the incoming refresh token, checks Redis for revocation,
// then atomically rotates the token pair (old JTI deleted, new JTI stored).
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string) (*RefreshResult, error) {
	// 1. Verify JWT signature + expiry
	payload, err := s.tokenMaker.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	// 2. Check Redis — if the key is gone, the token was revoked (logout occurred)
	oldJTI := payload.JTI.String()
	_, err = s.tokenStore.ValidateRefreshToken(ctx, oldJTI)
	if err != nil {
		if errors.Is(err, redisstore.ErrTokenRevoked) {
			return nil, ErrTokenRevoked
		}
		return nil, fmt.Errorf("redis validate: %w", err)
	}

	// 3. Fetch user to ensure they still exist
	user, err := s.userRepo.GetByID(ctx, payload.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 4. Mint new refresh token
	newRefreshStr, newRefreshPayload, err := s.tokenMaker.CreateRefreshToken(
		user.ID, user.Email, user.Name,
		s.cfg.RefreshTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	// 5. Mint new access token paired with new refresh JTI
	newAccessStr, _, err := s.tokenMaker.CreateAccessToken(
		user.ID, user.Email, user.Name,
		newRefreshPayload.JTI.String(),
		s.cfg.AccessTokenDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	// 6. Atomically revoke old JTI, store new JTI in Redis
	if err := s.tokenStore.RotateRefreshToken(
		ctx, oldJTI, newRefreshPayload.JTI.String(),
		user.ID, s.cfg.RefreshTokenDuration,
	); err != nil {
		return nil, fmt.Errorf("rotate token in redis: %w", err)
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
	UserID   uuid.UUID
	Email    string
	Name     string
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
		UserID:   payload.UserID,
		Email:    payload.Email,
		Name:     payload.Name,
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
