package token

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

const jwksRefreshSingleflightKey = "jwks_refresh"

// RemoteValidator validates JWT tokens using a remote JWKS endpoint.
// It caches public keys locally and validates tokens without calling auth service on every request.
// This allows other microservices to validate tokens independently.
type RemoteValidator struct {
	jwksUrl    string
	publicKeys map[string]cachedJWK
	mu         sync.RWMutex
	lastFetch  time.Time
	cacheTTL   time.Duration
	httpClient *http.Client
	refreshGrp singleflight.Group
}

type cachedJWK struct {
	alg string
	key interface{}
}

// NewRemoteValidator creates a new RemoteValidator that fetches keys from the given JWKS URL.
func NewRemoteValidator(jwksUrl string) *RemoteValidator {
	return &RemoteValidator{
		jwksUrl:    jwksUrl,
		publicKeys: make(map[string]cachedJWK),
		cacheTTL:   1 * time.Hour,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// VerifyAccessToken validates an access token using cached public keys.
func (v *RemoteValidator) VerifyAccessToken(tokenStr string) (*AccessPayload, error) {
	// Ensure keys are cached
	if err := v.ensureKeysCached(); err != nil {
		return nil, fmt.Errorf("ensure keys cached: %w", err)
	}

	// Validate token
	return v.validateAccessTokenWithCache(tokenStr)
}

// VerifyRefreshToken validates a refresh token using cached public keys.
func (v *RemoteValidator) VerifyRefreshToken(tokenStr string) (*RefreshPayload, error) {
	// Ensure keys are cached
	if err := v.ensureKeysCached(); err != nil {
		return nil, fmt.Errorf("ensure keys cached: %w", err)
	}

	// Validate token
	return v.validateRefreshTokenWithCache(tokenStr)
}

// ensureKeysCached fetches keys from JWKS endpoint if cache is stale.
func (v *RemoteValidator) ensureKeysCached() error {
	v.mu.RLock()
	hasCachedKeys := len(v.publicKeys) > 0
	isCached := hasCachedKeys && time.Since(v.lastFetch) < v.cacheTTL
	v.mu.RUnlock()

	if isCached {
		return nil
	}

	refreshErr := v.refreshKeysSingleflight()
	if refreshErr == nil {
		return nil
	}

	// If refresh fails but there are stale keys, continue to use them.
	v.mu.RLock()
	hasStaleFallback := len(v.publicKeys) > 0
	v.mu.RUnlock()
	if hasStaleFallback {
		return nil
	}

	return refreshErr
}

func (v *RemoteValidator) refreshKeysSingleflight() error {
	_, err, _ := v.refreshGrp.Do(jwksRefreshSingleflightKey, func() (interface{}, error) {
		return nil, v.refreshKeys()
	})
	return err
}

// refreshKeys fetches the latest public keys from the JWKS endpoint.
func (v *RemoteValidator) refreshKeys() error {
	resp, err := v.httpClient.Get(v.jwksUrl)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jwks endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var jwksResp JWKSResponse
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return fmt.Errorf("unmarshal jwks: %w", err)
	}

	// Parse keys and cache them
	keys := make(map[string]cachedJWK)
	for _, key := range jwksResp.Keys {
		alg := normalizedJWKAlg(key)
		if alg == "" {
			continue
		}

		switch alg {
		case jwt.SigningMethodEdDSA.Alg():
			pubKey, err := parseEd25519PublicKey(key)
			if err != nil {
				return fmt.Errorf("parse key %s: %w", key.KID, err)
			}
			keys[key.KID] = cachedJWK{alg: alg, key: pubKey}
		default:
			continue
		}
	}

	if len(keys) == 0 {
		return errors.New("jwks contains no valid signing keys")
	}

	v.mu.Lock()
	v.publicKeys = keys
	v.lastFetch = time.Now()
	v.mu.Unlock()

	return nil
}

// validateAccessTokenWithCache validates an access token using cached keys.
func (v *RemoteValidator) validateAccessTokenWithCache(tokenStr string) (*AccessPayload, error) {
	v.mu.RLock()
	publicKeys := v.publicKeys
	v.mu.RUnlock()

	if len(publicKeys) == 0 {
		return nil, errors.New("no public keys cached")
	}

	claims := &AccessTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims,
		func(token *jwt.Token) (interface{}, error) {
			return keyForTokenHeader(publicKeys, token)
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	payload, err := accessPayloadFromTokenClaims(claims)
	if err != nil {
		return nil, err
	}

	payload.KeyID, _ = token.Header["kid"].(string)

	return payload, nil
}

// validateRefreshTokenWithCache validates a refresh token using cached keys.
func (v *RemoteValidator) validateRefreshTokenWithCache(tokenStr string) (*RefreshPayload, error) {
	v.mu.RLock()
	publicKeys := v.publicKeys
	v.mu.RUnlock()

	if len(publicKeys) == 0 {
		return nil, errors.New("no public keys cached")
	}

	claims := &RefreshTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims,
		func(token *jwt.Token) (interface{}, error) {
			return keyForTokenHeader(publicKeys, token)
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	payload, err := refreshPayloadFromTokenClaims(claims)
	if err != nil {
		return nil, err
	}

	payload.KeyID, _ = token.Header["kid"].(string)

	return payload, nil
}

func keyForTokenHeader(publicKeys map[string]cachedJWK, token *jwt.Token) (interface{}, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errors.New("missing kid in token header")
	}

	entry, exists := publicKeys[kid]
	if !exists {
		return nil, fmt.Errorf("unknown key id: %s", kid)
	}

	headerAlg, _ := token.Header["alg"].(string)
	if entry.alg != "" && headerAlg != "" && entry.alg != headerAlg {
		return nil, fmt.Errorf("algorithm mismatch for key id %s", kid)
	}

	switch entry.alg {
	case jwt.SigningMethodEdDSA.Alg():
		if token.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
	default:
		return nil, fmt.Errorf("unsupported signing algorithm: %s", entry.alg)
	}

	return entry.key, nil
}

func normalizedJWKAlg(key JWKSKey) string {
	if key.Alg != "" {
		if key.Alg == jwt.SigningMethodEdDSA.Alg() {
			return key.Alg
		}
		return ""
	}

	switch {
	case key.KTY == "OKP" && key.CRV == "Ed25519":
		return jwt.SigningMethodEdDSA.Alg()
	default:
		return ""
	}
}

func parseEd25519PublicKey(key JWKSKey) (ed25519.PublicKey, error) {
	if key.KTY != "OKP" {
		return nil, errors.New("not an okp key")
	}
	if key.CRV != "Ed25519" {
		return nil, errors.New("unsupported okp curve")
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, fmt.Errorf("decode ed25519 x: %w", err)
	}
	if len(xBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid ed25519 public key size")
	}

	pub := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pub, xBytes)
	return pub, nil
}

// SetCacheTTL sets how long keys should be cached locally.
func (v *RemoteValidator) SetCacheTTL(ttl time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cacheTTL = ttl
}

// CreateAccessToken is not implemented for RemoteValidator.
// RemoteValidator is for validation only, not token creation.
// Use the auth service token maker for token creation.
func (v *RemoteValidator) CreateAccessToken(
	userID, email, name, familyID, refreshJTI, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *AccessPayload, error) {
	return "", nil, errors.New("CreateAccessToken not implemented in RemoteValidator; use auth service token maker")
}

// CreateRefreshToken is not implemented for RemoteValidator.
// RemoteValidator is for validation only, not token creation.
// Use the auth service token maker for token creation.
func (v *RemoteValidator) CreateRefreshToken(
	userID, email, name, familyID, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *RefreshPayload, error) {
	return "", nil, errors.New("CreateRefreshToken not implemented in RemoteValidator; use auth service token maker")
}

// GetCurrentKeyID returns an empty key id because RemoteValidator only verifies tokens.
func (v *RemoteValidator) GetCurrentKeyID() string {
	return ""
}
