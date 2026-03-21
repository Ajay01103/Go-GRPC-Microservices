package token

import (
	"time"

	"github.com/google/uuid"
)

// TokenType distinguishes between access and refresh tokens in claims.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// AccessPayload holds the JWT claims for an access token.
//
// Fields:
//   - JTI:        unique ID for this specific access token
//   - RefreshJTI: the JTI of the paired refresh token stored in Redis.
//     When the client calls RefreshToken, the server looks up
//     Redis key "refresh_token:{RefreshJTI}" to check revocation.
type AccessPayload struct {
	JTI        uuid.UUID `json:"jti"`
	UserID     uuid.UUID `json:"sub"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	TokenType  TokenType `json:"token_type"`
	RefreshJTI uuid.UUID `json:"refresh_jti"`
	IssuedAt   time.Time `json:"iat"`
	ExpiredAt  time.Time `json:"exp"`
}

// NewAccessPayload creates a new AccessPayload for the given user.
// refreshJTI must be the JTI of the refresh token this access token is paired with.
func NewAccessPayload(userID uuid.UUID, email, name string, refreshJTI uuid.UUID, duration time.Duration) (*AccessPayload, error) {
	return &AccessPayload{
		JTI:        uuid.New(),
		UserID:     userID,
		Email:      email,
		Name:       name,
		TokenType:  TokenTypeAccess,
		RefreshJTI: refreshJTI,
		IssuedAt:   time.Now(),
		ExpiredAt:  time.Now().Add(duration),
	}, nil
}

// Valid implements the jwt.Claims interface.
func (p *AccessPayload) Valid() error {
	if time.Now().After(p.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}

// RefreshPayload holds the JWT claims for a refresh token.
//
// Fields:
//   - JTI: unique ID stored as the Redis key "refresh_token:{JTI}".
//     Revoking a refresh token means deleting this key from Redis.
type RefreshPayload struct {
	JTI       uuid.UUID `json:"jti"`
	UserID    uuid.UUID `json:"sub"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	TokenType TokenType `json:"token_type"`
	IssuedAt  time.Time `json:"iat"`
	ExpiredAt time.Time `json:"exp"`
}

// NewRefreshPayload creates a new RefreshPayload for the given user.
func NewRefreshPayload(userID uuid.UUID, email, name string, duration time.Duration) (*RefreshPayload, error) {
	return &RefreshPayload{
		JTI:       uuid.New(),
		UserID:    userID,
		Email:     email,
		Name:      name,
		TokenType: TokenTypeRefresh,
		IssuedAt:  time.Now(),
		ExpiredAt: time.Now().Add(duration),
	}, nil
}

// Valid implements the jwt.Claims interface.
func (p *RefreshPayload) Valid() error {
	if time.Now().After(p.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}
