package token

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RSAMaker implements TokenMaker using RS256 (asymmetric) JWT tokens.
// Each token includes a kid (key ID) header for key rotation support.
type RSAMaker struct {
	mu               sync.RWMutex
	privateKey       *rsa.PrivateKey
	currentKeyID     string
	publicKeys       map[string]*rsa.PublicKey
	privateKeysByKid map[string]*rsa.PrivateKey
	expiresByKid     map[string]time.Time
	keyStore         *rsaRedisKeyStore
	keyTTL           time.Duration
}

// NewRSAMaker generates a new RSA keypair and creates an RSAMaker.
func NewRSAMaker() (*RSAMaker, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("rsa generate key: %w", err)
	}
	return NewRSAMakerFromPrivateKey(privateKey)
}

// NewRSAMakerWithRedis loads signing keys from Redis and only generates a
// new key when no valid key is found. This decouples key lifetime from process
// lifetime so restarts do not invalidate existing tokens.
func NewRSAMakerWithRedis(redisClient *redis.Client, keyTTL time.Duration) (*RSAMaker, error) {
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}
	if keyTTL < 90*24*time.Hour {
		keyTTL = 90 * 24 * time.Hour
	}

	maker := &RSAMaker{
		publicKeys:       make(map[string]*rsa.PublicKey),
		privateKeysByKid: make(map[string]*rsa.PrivateKey),
		expiresByKid:     make(map[string]time.Time),
		keyStore:         newRSARedisKeyStore(redisClient),
		keyTTL:           keyTTL,
	}

	ctx := context.Background()
	kids, err := maker.keyStore.loadAllKids(ctx)
	if err != nil {
		return nil, err
	}

	currentKid, err := maker.keyStore.loadCurrentKID(ctx)
	if err != nil {
		return nil, err
	}

	for _, kid := range kids {
		meta, err := maker.keyStore.loadKeyMeta(ctx, kid)
		if err != nil || meta == nil {
			continue
		}

		pubKey, err := maker.keyStore.loadPublicKey(ctx, kid)
		if err != nil || pubKey == nil {
			continue
		}

		maker.publicKeys[kid] = pubKey
		maker.expiresByKid[kid] = meta.ExpiresAt

		if meta.Status == KeyStatusActive {
			privKey, err := maker.keyStore.loadPrivateKey(ctx, kid)
			if err == nil && privKey != nil {
				maker.privateKeysByKid[kid] = privKey
			}
		}

		if kid == currentKid && meta.Status == KeyStatusActive {
			maker.currentKeyID = currentKid
			maker.privateKey = maker.privateKeysByKid[currentKid]
		}
	}

	if maker.currentKeyID != "" && maker.privateKey != nil {
		return maker, nil
	}

	if _, err := maker.RotateKey(); err != nil {
		return nil, err
	}
	return maker, nil
}

// NewRSAMakerFromPrivateKey creates an RSAMaker from an existing private key.
func NewRSAMakerFromPrivateKey(privateKey *rsa.PrivateKey) (*RSAMaker, error) {
	if privateKey == nil {
		return nil, errors.New("private key is nil")
	}

	keyID := fmt.Sprintf("key-%d", time.Now().Unix())
	expiresAt := time.Now().Add(10 * 365 * 24 * time.Hour).UTC()

	m := &RSAMaker{
		privateKey:       privateKey,
		currentKeyID:     keyID,
		publicKeys:       make(map[string]*rsa.PublicKey),
		privateKeysByKid: make(map[string]*rsa.PrivateKey),
		expiresByKid:     make(map[string]time.Time),
		keyTTL:           10 * 365 * 24 * time.Hour,
	}

	m.publicKeys[keyID] = &privateKey.PublicKey
	m.privateKeysByKid[keyID] = privateKey
	m.expiresByKid[keyID] = expiresAt

	return m, nil
}

// CreateRefreshToken mints a new refresh token signed with RS256.
func (m *RSAMaker) CreateRefreshToken(
	userID, email, name, familyID, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *RefreshPayload, error) {
	if err := m.ensureCurrentSigningKey(); err != nil {
		return "", nil, err
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid user id: %w", err)
	}
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid family id: %w", err)
	}

	payload, err := NewRefreshPayload(uid, email, name, fid, dpopKeyThumbprint, duration)
	if err != nil {
		return "", nil, err
	}

	m.mu.RLock()
	currentKid := m.currentKeyID
	privateKey := m.privateKey
	m.mu.RUnlock()

	payload.KeyID = currentKid

	claims := jwt.MapClaims{
		"jti":        payload.JTI.String(),
		"sub":        payload.UserID.String(),
		"email":      payload.Email,
		"name":       payload.Name,
		"token_type": string(payload.TokenType),
		"family_id":  payload.FamilyID.String(),
		"iat":        payload.IssuedAt.Unix(),
		"exp":        payload.ExpiredAt.Unix(),
	}

	if payload.DPoPKeyThumbprint != "" {
		claims["dpop_key_thumbprint"] = payload.DPoPKeyThumbprint
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = currentKid

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", nil, fmt.Errorf("sign token: %w", err)
	}

	return signed, payload, nil
}

// CreateAccessToken mints a new access token signed with RS256.
func (m *RSAMaker) CreateAccessToken(
	userID, email, name, familyID, refreshJTI, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *AccessPayload, error) {
	if err := m.ensureCurrentSigningKey(); err != nil {
		return "", nil, err
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid user id: %w", err)
	}
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid family id: %w", err)
	}
	rjti, err := uuid.Parse(refreshJTI)
	if err != nil {
		return "", nil, fmt.Errorf("invalid refresh jti: %w", err)
	}

	payload, err := NewAccessPayload(uid, email, name, fid, rjti, dpopKeyThumbprint, duration)
	if err != nil {
		return "", nil, err
	}

	m.mu.RLock()
	currentKid := m.currentKeyID
	privateKey := m.privateKey
	m.mu.RUnlock()

	payload.KeyID = currentKid

	claims := jwt.MapClaims{
		"jti":         payload.JTI.String(),
		"sub":         payload.UserID.String(),
		"email":       payload.Email,
		"name":        payload.Name,
		"token_type":  string(payload.TokenType),
		"family_id":   payload.FamilyID.String(),
		"refresh_jti": payload.RefreshJTI.String(),
		"iat":         payload.IssuedAt.Unix(),
		"exp":         payload.ExpiredAt.Unix(),
	}

	if payload.DPoPKeyThumbprint != "" {
		claims["dpop_key_thumbprint"] = payload.DPoPKeyThumbprint
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = currentKid

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", nil, fmt.Errorf("sign token: %w", err)
	}

	return signed, payload, nil
}

// VerifyAccessToken parses and validates an access token using public key from cache.
func (m *RSAMaker) VerifyAccessToken(tokenStr string) (*AccessPayload, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.MapClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			kid, ok := token.Header["kid"].(string)
			if !ok || kid == "" {
				return nil, errors.New("missing kid in token header")
			}

			pubKey, exists := m.getValidPublicKey(kid)
			if !exists {
				return nil, fmt.Errorf("unknown key id: %s", kid)
			}

			return pubKey, nil
		},
	)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	payload := &AccessPayload{}

	jti, ok := (*claims)["jti"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	jtiUuid, err := uuid.Parse(jti)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.JTI = jtiUuid

	sub, ok := (*claims)["sub"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	uid, err := uuid.Parse(sub)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.UserID = uid

	payload.Email, _ = (*claims)["email"].(string)
	payload.Name, _ = (*claims)["name"].(string)
	payload.TokenType = TokenTypeAccess

	familyID, ok := (*claims)["family_id"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.FamilyID = fid

	refreshJti, ok := (*claims)["refresh_jti"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	rjti, err := uuid.Parse(refreshJti)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.RefreshJTI = rjti

	iat, ok := (*claims)["iat"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	payload.IssuedAt = time.Unix(int64(iat), 0)

	exp, ok := (*claims)["exp"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	payload.ExpiredAt = time.Unix(int64(exp), 0)

	if time.Now().After(payload.ExpiredAt) {
		return nil, ErrExpiredToken
	}

	payload.KeyID, _ = token.Header["kid"].(string)

	if thumbprint, ok := (*claims)["dpop_key_thumbprint"].(string); ok {
		payload.DPoPKeyThumbprint = thumbprint
	}

	return payload, nil
}

// VerifyRefreshToken parses and validates a refresh token using public key from cache.
func (m *RSAMaker) VerifyRefreshToken(tokenStr string) (*RefreshPayload, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.MapClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			kid, ok := token.Header["kid"].(string)
			if !ok || kid == "" {
				return nil, errors.New("missing kid in token header")
			}

			pubKey, exists := m.getValidPublicKey(kid)
			if !exists {
				return nil, fmt.Errorf("unknown key id: %s", kid)
			}

			return pubKey, nil
		},
	)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	payload := &RefreshPayload{}

	jti, ok := (*claims)["jti"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	jtiUuid, err := uuid.Parse(jti)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.JTI = jtiUuid

	sub, ok := (*claims)["sub"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	uid, err := uuid.Parse(sub)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.UserID = uid

	payload.Email, _ = (*claims)["email"].(string)
	payload.Name, _ = (*claims)["name"].(string)
	payload.TokenType = TokenTypeRefresh

	familyID, ok := (*claims)["family_id"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	payload.FamilyID = fid

	iat, ok := (*claims)["iat"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	payload.IssuedAt = time.Unix(int64(iat), 0)

	exp, ok := (*claims)["exp"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	payload.ExpiredAt = time.Unix(int64(exp), 0)

	if time.Now().After(payload.ExpiredAt) {
		return nil, ErrExpiredToken
	}

	payload.KeyID, _ = token.Header["kid"].(string)

	if thumbprint, ok := (*claims)["dpop_key_thumbprint"].(string); ok {
		payload.DPoPKeyThumbprint = thumbprint
	}

	return payload, nil
}

// RotateKey generates a new RSA keypair and makes it the current signing key.
func (m *RSAMaker) RotateKey() (string, error) {
	newPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("rsa generate key: %w", err)
	}

	newKeyID := fmt.Sprintf("key-%d", time.Now().Unix())
	expiresAt := time.Now().UTC().Add(m.keyTTL)
	if m.keyTTL <= 0 {
		expiresAt = time.Now().UTC().Add(10 * 365 * 24 * time.Hour)
	}

	m.mu.Lock()

	pastKeyID := m.currentKeyID

	m.privateKey = newPrivateKey
	m.currentKeyID = newKeyID
	m.publicKeys[newKeyID] = &newPrivateKey.PublicKey
	m.privateKeysByKid[newKeyID] = newPrivateKey
	m.expiresByKid[newKeyID] = expiresAt
	m.mu.Unlock()

	if m.keyStore != nil {
		ctx := context.Background()
		if pastKeyID != "" {
			_ = m.keyStore.retireKey(ctx, pastKeyID)
		}
		if err := m.keyStore.storeKey(ctx, newKeyID, newPrivateKey, m.keyTTL); err != nil {
			return "", err
		}
	}

	return newKeyID, nil
}

// GetPublicKeys returns all active public keys keyed by their kid for JWKS endpoint.
func (m *RSAMaker) GetPublicKeys() map[string]*rsa.PublicKey {
	now := time.Now().UTC()
	m.mu.RLock()
	keys := make(map[string]*rsa.PublicKey)
	for kid, pubKey := range m.publicKeys {
		expiresAt, ok := m.expiresByKid[kid]
		if ok && now.After(expiresAt) {
			continue
		}
		keys[kid] = pubKey
	}
	m.mu.RUnlock()
	return keys
}

// GetCurrentKeyID returns the kid of the current signing key.
func (m *RSAMaker) GetCurrentKeyID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentKeyID
}

// JWKSKey represents a key in JWKS format (RFC 7517).
type JWKSKey struct {
	KTY string `json:"kty"`
	Use string `json:"use"`
	KID string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

// JWKSResponse is the response structure for the JWKS endpoint.
type JWKSResponse struct {
	Keys []JWKSKey `json:"keys"`
}

// ExportPublicKeys exports all public keys in JWKS format for the endpoint.
func (m *RSAMaker) ExportPublicKeys() ([]byte, error) {
	response := JWKSResponse{Keys: []JWKSKey{}}
	now := time.Now().UTC()

	m.mu.RLock()

	for kid, pubKey := range m.publicKeys {
		expiresAt, ok := m.expiresByKid[kid]
		if ok && now.After(expiresAt) {
			continue
		}

		n := base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes())

		eBytes := []byte{byte(pubKey.E >> 16), byte(pubKey.E >> 8), byte(pubKey.E)}
		i := 0
		for i < len(eBytes)-1 && eBytes[i] == 0 {
			i++
		}
		e := base64.RawURLEncoding.EncodeToString(eBytes[i:])

		jwksKey := JWKSKey{
			KTY: "RSA",
			Use: "sig",
			KID: kid,
			N:   n,
			E:   e,
			Alg: "RS256",
		}

		response.Keys = append(response.Keys, jwksKey)
	}
	m.mu.RUnlock()

	return json.Marshal(response)
}

func (m *RSAMaker) ensureCurrentSigningKey() error {
	m.mu.RLock()
	currentKid := m.currentKeyID
	privateKey := m.privateKey
	expiresAt, hasExpiry := m.expiresByKid[currentKid]
	m.mu.RUnlock()

	if currentKid != "" && privateKey != nil {
		// Rotate 60 days before expiration, leaving 30 days active signing lifetime.
		if !hasExpiry || time.Now().UTC().Add(60*24*time.Hour).Before(expiresAt) {
			return nil
		}
	}

	if _, err := m.RotateKey(); err != nil {
		return fmt.Errorf("rotate expired signing key: %w", err)
	}
	return nil
}

func (m *RSAMaker) getValidPublicKey(kid string) (*rsa.PublicKey, bool) {
	now := time.Now().UTC()
	m.mu.RLock()
	pubKey, exists := m.publicKeys[kid]
	expiresAt, hasExpiry := m.expiresByKid[kid]
	m.mu.RUnlock()
	if !exists {
		return nil, false
	}
	if hasExpiry && now.After(expiresAt) {
		return nil, false
	}
	return pubKey, true
}
