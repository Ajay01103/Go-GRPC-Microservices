package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const minSecretKeySize = 32

// JWTMaker implements TokenMaker using HS256 JWT tokens.
type JWTMaker struct {
	secretKey string
}

// NewJWTMaker creates a JWTMaker. secretKey must be at least 32 characters.
func NewJWTMaker(secretKey string) (*JWTMaker, error) {
	if len(secretKey) < minSecretKeySize {
		return nil, fmt.Errorf("invalid key size: must be at least %d characters", minSecretKeySize)
	}
	return &JWTMaker{secretKey}, nil
}

// ─── Refresh Token ────────────────────────────────────────────────────────────

// CreateRefreshToken mints a new refresh token.
// The payload's JTI must be stored in Redis by the caller before responding.
func (m *JWTMaker) CreateRefreshToken(
	userID, email, name string,
	duration time.Duration,
) (string, *RefreshPayload, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid user id: %w", err)
	}

	payload, err := NewRefreshPayload(uid, email, name, duration)
	if err != nil {
		return "", nil, err
	}

	claims := jwt.MapClaims{
		"jti":        payload.JTI.String(),
		"sub":        payload.UserID.String(),
		"email":      payload.Email,
		"name":       payload.Name,
		"token_type": string(payload.TokenType),
		"iat":        payload.IssuedAt.Unix(),
		"exp":        payload.ExpiredAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(m.secretKey))
	if err != nil {
		return "", nil, err
	}
	return signed, payload, nil
}

// ─── Access Token ─────────────────────────────────────────────────────────────

// CreateAccessToken mints a new access token paired with the given refresh token JTI.
func (m *JWTMaker) CreateAccessToken(
	userID, email, name, refreshJTI string,
	duration time.Duration,
) (string, *AccessPayload, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid user id: %w", err)
	}
	rjti, err := uuid.Parse(refreshJTI)
	if err != nil {
		return "", nil, fmt.Errorf("invalid refresh jti: %w", err)
	}

	payload, err := NewAccessPayload(uid, email, name, rjti, duration)
	if err != nil {
		return "", nil, err
	}

	claims := jwt.MapClaims{
		"jti":         payload.JTI.String(),
		"sub":         payload.UserID.String(),
		"email":       payload.Email,
		"name":        payload.Name,
		"token_type":  string(payload.TokenType),
		"refresh_jti": payload.RefreshJTI.String(),
		"iat":         payload.IssuedAt.Unix(),
		"exp":         payload.ExpiredAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(m.secretKey))
	if err != nil {
		return "", nil, err
	}
	return signed, payload, nil
}

// ─── Verification ─────────────────────────────────────────────────────────────

// VerifyAccessToken parses and validates an access token.
func (m *JWTMaker) VerifyAccessToken(tokenStr string) (*AccessPayload, error) {
	claims, err := m.parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}

	tokenType, _ := claims["token_type"].(string)
	if tokenType != string(TokenTypeAccess) {
		return nil, ErrInvalidToken
	}

	payload, err := accessPayloadFromClaims(claims)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// VerifyRefreshToken parses and validates a refresh token.
func (m *JWTMaker) VerifyRefreshToken(tokenStr string) (*RefreshPayload, error) {
	claims, err := m.parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}

	tokenType, _ := claims["token_type"].(string)
	if tokenType != string(TokenTypeRefresh) {
		return nil, ErrInvalidToken
	}

	payload, err := refreshPayloadFromClaims(claims)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (m *JWTMaker) parseClaims(tokenStr string) (jwt.MapClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(m.secretKey), nil
	}

	jwtToken, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, keyFunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok || !jwtToken.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func uuidFromClaims(claims jwt.MapClaims, key string) (uuid.UUID, error) {
	raw, ok := claims[key].(string)
	if !ok {
		return uuid.Nil, ErrInvalidToken
	}
	v, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return v, nil
}

func timeFromClaims(claims jwt.MapClaims, key string) (time.Time, error) {
	raw, ok := claims[key].(float64)
	if !ok {
		return time.Time{}, ErrInvalidToken
	}
	return time.Unix(int64(raw), 0), nil
}

func accessPayloadFromClaims(claims jwt.MapClaims) (*AccessPayload, error) {
	jti, err := uuidFromClaims(claims, "jti")
	if err != nil {
		return nil, err
	}
	sub, err := uuidFromClaims(claims, "sub")
	if err != nil {
		return nil, err
	}
	rjti, err := uuidFromClaims(claims, "refresh_jti")
	if err != nil {
		return nil, err
	}
	exp, err := timeFromClaims(claims, "exp")
	if err != nil {
		return nil, err
	}
	iat, err := timeFromClaims(claims, "iat")
	if err != nil {
		return nil, err
	}

	return &AccessPayload{
		JTI:        jti,
		UserID:     sub,
		Email:      fmt.Sprintf("%v", claims["email"]),
		Name:       fmt.Sprintf("%v", claims["name"]),
		TokenType:  TokenTypeAccess,
		RefreshJTI: rjti,
		IssuedAt:   iat,
		ExpiredAt:  exp,
	}, nil
}

func refreshPayloadFromClaims(claims jwt.MapClaims) (*RefreshPayload, error) {
	jti, err := uuidFromClaims(claims, "jti")
	if err != nil {
		return nil, err
	}
	sub, err := uuidFromClaims(claims, "sub")
	if err != nil {
		return nil, err
	}
	exp, err := timeFromClaims(claims, "exp")
	if err != nil {
		return nil, err
	}
	iat, err := timeFromClaims(claims, "iat")
	if err != nil {
		return nil, err
	}

	return &RefreshPayload{
		JTI:       jti,
		UserID:    sub,
		Email:     fmt.Sprintf("%v", claims["email"]),
		Name:       fmt.Sprintf("%v", claims["name"]),
		TokenType: TokenTypeRefresh,
		IssuedAt:  iat,
		ExpiredAt: exp,
	}, nil
}
