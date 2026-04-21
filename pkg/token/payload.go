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
//   - KeyID:      the kid of the RSA key used to sign this token
//   - DPoPKeyThumbprint: S256 thumbprint of client's DPoP public key (if DPoP-bound)
type AccessPayload struct {
	JTI               uuid.UUID `json:"jti"`
	UserID            uuid.UUID `json:"sub"`
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	TokenType         TokenType `json:"token_type"`
	FamilyID          uuid.UUID `json:"family_id"`
	RefreshJTI        uuid.UUID `json:"refresh_jti"`
	IssuedAt          time.Time `json:"iat"`
	ExpiredAt         time.Time `json:"exp"`
	KeyID             string    `json:"kid,omitempty"`
	DPoPKeyThumbprint string    `json:"dpop_key_thumbprint,omitempty"`
}

// NewAccessPayload creates a new AccessPayload for the given user.
// refreshJTI must be the JTI of the refresh token this access token is paired with.
func NewAccessPayload(userID uuid.UUID, email, name string, familyID, refreshJTI uuid.UUID, dpopKeyThumbprint string, duration time.Duration) (*AccessPayload, error) {
	return &AccessPayload{
		JTI:               uuid.New(),
		UserID:            userID,
		Email:             email,
		Name:              name,
		TokenType:         TokenTypeAccess,
		FamilyID:          familyID,
		RefreshJTI:        refreshJTI,
		IssuedAt:          time.Now(),
		ExpiredAt:         time.Now().Add(duration),
		DPoPKeyThumbprint: dpopKeyThumbprint,
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
//   - KeyID: the kid of the RSA key used to sign this token
//   - DPoPKeyThumbprint: S256 thumbprint of client's DPoP public key (if DPoP-bound)
type RefreshPayload struct {
	JTI               uuid.UUID `json:"jti"`
	UserID            uuid.UUID `json:"sub"`
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	TokenType         TokenType `json:"token_type"`
	FamilyID          uuid.UUID `json:"family_id"`
	IssuedAt          time.Time `json:"iat"`
	ExpiredAt         time.Time `json:"exp"`
	KeyID             string    `json:"kid,omitempty"`
	DPoPKeyThumbprint string    `json:"dpop_key_thumbprint,omitempty"`
}

// NewRefreshPayload creates a new RefreshPayload for the given user.
func NewRefreshPayload(userID uuid.UUID, email, name string, familyID uuid.UUID, dpopKeyThumbprint string, duration time.Duration) (*RefreshPayload, error) {
	return &RefreshPayload{
		JTI:               uuid.New(),
		UserID:            userID,
		Email:             email,
		Name:              name,
		TokenType:         TokenTypeRefresh,
		FamilyID:          familyID,
		IssuedAt:          time.Now(),
		ExpiredAt:         time.Now().Add(duration),
		DPoPKeyThumbprint: dpopKeyThumbprint,
	}, nil
}

// Valid implements the jwt.Claims interface.
func (p *RefreshPayload) Valid() error {
	if time.Now().After(p.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}
