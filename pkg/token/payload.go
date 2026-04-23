package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// AccessTokenClaims is a typed JWT claim set for access tokens.
type AccessTokenClaims struct {
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	TokenType         TokenType `json:"token_type"`
	FamilyID          string    `json:"family_id"`
	RefreshJTI        string    `json:"refresh_jti"`
	DPoPKeyThumbprint string    `json:"dpop_key_thumbprint,omitempty"`
	jwt.RegisteredClaims
}

// Reset clears all fields before this claim object is reused from a pool.
func (c *AccessTokenClaims) Reset() {
	*c = AccessTokenClaims{}
}

// NewAccessPayload creates a new AccessPayload for the given user.
// refreshJTI must be the JTI of the refresh token this access token is paired with.
func NewAccessPayload(userID uuid.UUID, email, name string, familyID, refreshJTI uuid.UUID, dpopKeyThumbprint string, duration time.Duration) (*AccessPayload, error) {
	return NewAccessPayloadAt(userID, email, name, familyID, refreshJTI, dpopKeyThumbprint, time.Now().UTC(), duration)
}

// NewAccessPayloadAt creates a new AccessPayload using a provided timestamp.
func NewAccessPayloadAt(userID uuid.UUID, email, name string, familyID, refreshJTI uuid.UUID, dpopKeyThumbprint string, now time.Time, duration time.Duration) (*AccessPayload, error) {
	now = now.UTC()
	return &AccessPayload{
		JTI:               uuid.New(),
		UserID:            userID,
		Email:             email,
		Name:              name,
		TokenType:         TokenTypeAccess,
		FamilyID:          familyID,
		RefreshJTI:        refreshJTI,
		IssuedAt:          now,
		ExpiredAt:         now.Add(duration),
		DPoPKeyThumbprint: dpopKeyThumbprint,
	}, nil
}

func (p *AccessPayload) FillClaims(claims *AccessTokenClaims) {
	claims.Email = p.Email
	claims.Name = p.Name
	claims.TokenType = p.TokenType
	claims.FamilyID = p.FamilyID.String()
	claims.RefreshJTI = p.RefreshJTI.String()
	claims.DPoPKeyThumbprint = p.DPoPKeyThumbprint
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ID:        p.JTI.String(),
		Subject:   p.UserID.String(),
		IssuedAt:  jwt.NewNumericDate(p.IssuedAt),
		ExpiresAt: jwt.NewNumericDate(p.ExpiredAt),
	}
}

func accessPayloadFromTokenClaims(claims *AccessTokenClaims) (*AccessPayload, error) {
	if claims == nil || claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrInvalidToken
	}

	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrInvalidToken
	}
	familyID, err := uuid.Parse(claims.FamilyID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	refreshJTI, err := uuid.Parse(claims.RefreshJTI)
	if err != nil {
		return nil, ErrInvalidToken
	}

	payload := &AccessPayload{
		JTI:               jti,
		UserID:            uid,
		Email:             claims.Email,
		Name:              claims.Name,
		TokenType:         claims.TokenType,
		FamilyID:          familyID,
		RefreshJTI:        refreshJTI,
		IssuedAt:          claims.IssuedAt.Time,
		ExpiredAt:         claims.ExpiresAt.Time,
		DPoPKeyThumbprint: claims.DPoPKeyThumbprint,
	}

	if time.Now().After(payload.ExpiredAt) {
		return nil, ErrExpiredToken
	}

	return payload, nil
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

// RefreshTokenClaims is a typed JWT claim set for refresh tokens.
type RefreshTokenClaims struct {
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	TokenType         TokenType `json:"token_type"`
	FamilyID          string    `json:"family_id"`
	DPoPKeyThumbprint string    `json:"dpop_key_thumbprint,omitempty"`
	jwt.RegisteredClaims
}

// Reset clears all fields before this claim object is reused from a pool.
func (c *RefreshTokenClaims) Reset() {
	*c = RefreshTokenClaims{}
}

// NewRefreshPayload creates a new RefreshPayload for the given user.
func NewRefreshPayload(userID uuid.UUID, email, name string, familyID uuid.UUID, dpopKeyThumbprint string, duration time.Duration) (*RefreshPayload, error) {
	return NewRefreshPayloadAt(userID, email, name, familyID, dpopKeyThumbprint, time.Now().UTC(), duration)
}

// NewRefreshPayloadAt creates a new RefreshPayload using a provided timestamp.
func NewRefreshPayloadAt(userID uuid.UUID, email, name string, familyID uuid.UUID, dpopKeyThumbprint string, now time.Time, duration time.Duration) (*RefreshPayload, error) {
	now = now.UTC()
	return &RefreshPayload{
		JTI:               uuid.New(),
		UserID:            userID,
		Email:             email,
		Name:              name,
		TokenType:         TokenTypeRefresh,
		FamilyID:          familyID,
		IssuedAt:          now,
		ExpiredAt:         now.Add(duration),
		DPoPKeyThumbprint: dpopKeyThumbprint,
	}, nil
}

func (p *RefreshPayload) FillClaims(claims *RefreshTokenClaims) {
	claims.Email = p.Email
	claims.Name = p.Name
	claims.TokenType = p.TokenType
	claims.FamilyID = p.FamilyID.String()
	claims.DPoPKeyThumbprint = p.DPoPKeyThumbprint
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ID:        p.JTI.String(),
		Subject:   p.UserID.String(),
		IssuedAt:  jwt.NewNumericDate(p.IssuedAt),
		ExpiresAt: jwt.NewNumericDate(p.ExpiredAt),
	}
}

func refreshPayloadFromTokenClaims(claims *RefreshTokenClaims) (*RefreshPayload, error) {
	if claims == nil || claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrInvalidToken
	}

	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrInvalidToken
	}
	familyID, err := uuid.Parse(claims.FamilyID)
	if err != nil {
		return nil, ErrInvalidToken
	}

	payload := &RefreshPayload{
		JTI:               jti,
		UserID:            uid,
		Email:             claims.Email,
		Name:              claims.Name,
		TokenType:         claims.TokenType,
		FamilyID:          familyID,
		IssuedAt:          claims.IssuedAt.Time,
		ExpiredAt:         claims.ExpiresAt.Time,
		DPoPKeyThumbprint: claims.DPoPKeyThumbprint,
	}

	if time.Now().After(payload.ExpiredAt) {
		return nil, ErrExpiredToken
	}

	return payload, nil
}

// Valid implements the jwt.Claims interface.
func (p *RefreshPayload) Valid() error {
	if time.Now().After(p.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}
