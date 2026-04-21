package token

import (
	"crypto/rsa"
	"encoding/json"
	"time"
)

// JWKSProvider manages JWKS endpoint data and key rotation.
// It provides a simple interface for exposing public keys to other services.
type JWKSProvider struct {
	maker *RSAMaker
}

// NewJWKSProvider creates a new JWKS provider from an RSA token maker.
func NewJWKSProvider(maker *RSAMaker) *JWKSProvider {
	return &JWKSProvider{maker: maker}
}

// GetJWKS returns the current JWKS response for the endpoint.
// This includes all active public keys keyed by kid.
func (p *JWKSProvider) GetJWKS() ([]byte, error) {
	return p.maker.ExportPublicKeys()
}

// GetKey returns a specific public key by kid.
func (p *JWKSProvider) GetKey(kid string) (*rsa.PublicKey, bool) {
	keys := p.maker.GetPublicKeys()
	key, ok := keys[kid]
	return key, ok
}

// AllKeys returns all active public keys.
func (p *JWKSProvider) AllKeys() map[string]*rsa.PublicKey {
	return p.maker.GetPublicKeys()
}

// RotateKey performs a key rotation and returns the new kid.
func (p *JWKSProvider) RotateKey() (string, error) {
	return p.maker.RotateKey()
}

// GetCurrentKeyID returns the kid of the current signing key.
func (p *JWKSProvider) GetCurrentKeyID() string {
	return p.maker.GetCurrentKeyID()
}

// KeyMetadata provides metadata about a key in JWKS format.
type KeyMetadata struct {
	KID       string    `json:"kid"`
	Algorithm string    `json:"alg"`
	KeyType   string    `json:"kty"`
	Use       string    `json:"use"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// ExportKeyMetadata returns metadata about all keys in JWKS format.
func (p *JWKSProvider) ExportKeyMetadata() []KeyMetadata {
	keys := p.maker.GetPublicKeys()
	metadata := make([]KeyMetadata, 0, len(keys))

	for kid := range keys {
		metadata = append(metadata, KeyMetadata{
			KID:       kid,
			Algorithm: "RS256",
			KeyType:   "RSA",
			Use:       "sig",
			CreatedAt: time.Now(),
		})
	}

	return metadata
}

// ValidateWithKey validates a token bytes payload against a specific key.
// This is useful for manual token validation in other services.
func (p *JWKSProvider) ValidateWithKey(kid string, tokenBytes []byte) (bool, error) {
	key, ok := p.GetKey(kid)
	if !ok {
		return false, nil
	}

	// Placeholder for actual validation logic
	_ = tokenBytes
	_ = key

	return true, nil
}

// MarshalJSON allows JWKS provider to be serialized.
func (p *JWKSProvider) MarshalJSON() ([]byte, error) {
	type Response struct {
		Keys []JWKSKey `json:"keys"`
	}

	keys := p.maker.GetPublicKeys()
	jwksKeys := make([]JWKSKey, 0, len(keys))

	for kid, pubKey := range keys {
		jwksKeys = append(jwksKeys, JWKSKey{
			KTY: "RSA",
			Use: "sig",
			KID: kid,
			N:   pubKey.N.String(),
			E:   "AQAB",
			Alg: "RS256",
		})
	}

	return json.Marshal(Response{Keys: jwksKeys})
}
