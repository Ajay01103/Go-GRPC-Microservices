package token

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// RemoteValidator validates JWT tokens using a remote JWKS endpoint.
// It caches public keys locally and validates tokens without calling auth service on every request.
// This allows other microservices to validate tokens independently.
type RemoteValidator struct {
	jwksUrl    string
	publicKeys map[string]*rsa.PublicKey
	mu         sync.RWMutex
	lastFetch  time.Time
	cacheTTL   time.Duration
	httpClient *http.Client
}

// NewRemoteValidator creates a new RemoteValidator that fetches keys from the given JWKS URL.
func NewRemoteValidator(jwksUrl string) *RemoteValidator {
	return &RemoteValidator{
		jwksUrl:    jwksUrl,
		publicKeys: make(map[string]*rsa.PublicKey),
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
	isCached := len(v.publicKeys) > 0 && time.Since(v.lastFetch) < v.cacheTTL
	v.mu.RUnlock()

	if isCached {
		return nil
	}

	return v.refreshKeys()
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
	keys := make(map[string]*rsa.PublicKey)
	for _, key := range jwksResp.Keys {
		if key.Alg != "RS256" {
			continue
		}

		pubKey, err := parseRSAPublicKey(key)
		if err != nil {
			return fmt.Errorf("parse key %s: %w", key.KID, err)
		}

		keys[key.KID] = pubKey
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

	// For RemoteValidator, we use RSAMaker logic since tokens are RS256
	// We need to validate against the cached public keys
	tokenMaker := &RSAMaker{
		publicKeys: publicKeys,
	}
	return tokenMaker.VerifyAccessToken(tokenStr)
}

// validateRefreshTokenWithCache validates a refresh token using cached keys.
func (v *RemoteValidator) validateRefreshTokenWithCache(tokenStr string) (*RefreshPayload, error) {
	v.mu.RLock()
	publicKeys := v.publicKeys
	v.mu.RUnlock()

	if len(publicKeys) == 0 {
		return nil, errors.New("no public keys cached")
	}

	// For RemoteValidator, we use RSAMaker logic since tokens are RS256
	// We need to validate against the cached public keys
	tokenMaker := &RSAMaker{
		publicKeys: publicKeys,
	}
	return tokenMaker.VerifyRefreshToken(tokenStr)
}

// parseRSAPublicKey extracts an RSA public key from a JWKS key entry.
func parseRSAPublicKey(key JWKSKey) (*rsa.PublicKey, error) {
	if key.KTY != "RSA" {
		return nil, errors.New("not an RSA key")
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus n: %w", err)
	}
	if len(nBytes) == 0 {
		return nil, errors.New("empty modulus n")
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent e: %w", err)
	}
	if len(eBytes) == 0 {
		return nil, errors.New("empty exponent e")
	}

	eBig := new(big.Int).SetBytes(eBytes)
	if !eBig.IsInt64() {
		return nil, errors.New("exponent does not fit int64")
	}
	e := int(eBig.Int64())
	if e < 3 || e%2 == 0 {
		return nil, errors.New("invalid RSA exponent")
	}

	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}

	if pub.N == nil || pub.N.Sign() <= 0 {
		return nil, errors.New("invalid RSA modulus")
	}
	if pub.N.BitLen() < 2048 {
		return nil, errors.New("rsa modulus too small")
	}

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
// Use RSAMaker in the auth service for token creation.
func (v *RemoteValidator) CreateAccessToken(
	userID, email, name, familyID, refreshJTI, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *AccessPayload, error) {
	return "", nil, errors.New("CreateAccessToken not implemented in RemoteValidator; use RSAMaker in auth service")
}

// CreateRefreshToken is not implemented for RemoteValidator.
// RemoteValidator is for validation only, not token creation.
// Use RSAMaker in the auth service for token creation.
func (v *RemoteValidator) CreateRefreshToken(
	userID, email, name, familyID, dpopKeyThumbprint string,
	duration time.Duration,
) (string, *RefreshPayload, error) {
	return "", nil, errors.New("CreateRefreshToken not implemented in RemoteValidator; use RSAMaker in auth service")
}

// GetCurrentKeyID returns an empty key id because RemoteValidator only verifies tokens.
func (v *RemoteValidator) GetCurrentKeyID() string {
	return ""
}
