package dpop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// DPoPStore manages DPoP proof replay protection in Redis.
// It tracks which DPoP proofs have been used to prevent reuse attacks.
type DPoPStore struct {
	client *redis.Client
}

const (
	dPoPProofPrefix = "dpop_proof:"
	dPoPNoncePrefix = "dpop_nonce:"
)

// NewDPoPStore creates a new DPoP store from a Redis client.
func NewDPoPStore(client *redis.Client) *DPoPStore {
	return &DPoPStore{client: client}
}

// RecordProof records that a DPoP proof has been used.
// This prevents the same proof from being used twice (replay attack).
// The TTL matches the proof's validity window.
func (s *DPoPStore) RecordProof(ctx context.Context, proofJTI string, ttl time.Duration) error {
	key := dPoPProofPrefix + proofJTI
	if err := s.client.Set(ctx, key, "used", ttl).Err(); err != nil {
		return fmt.Errorf("redis record proof: %w", err)
	}
	return nil
}

// IsProofUsed checks if a DPoP proof has already been used.
func (s *DPoPStore) IsProofUsed(ctx context.Context, proofJTI string) (bool, error) {
	result, err := s.client.Get(ctx, dPoPProofPrefix+proofJTI).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis check proof: %w", err)
	}
	return result == "used", nil
}

// StoreNonce stores a server-issued nonce for DPoP challenges.
// This is used to prevent DPoP replay across different endpoints.
func (s *DPoPStore) StoreNonce(ctx context.Context, nonce string, ttl time.Duration) error {
	key := dPoPNoncePrefix + nonce
	if err := s.client.Set(ctx, key, "challenge", ttl).Err(); err != nil {
		return fmt.Errorf("redis store nonce: %w", err)
	}
	return nil
}

// ValidateNonce checks if a nonce is valid and invalidates it.
// After validation, the nonce can only be used once.
func (s *DPoPStore) ValidateNonce(ctx context.Context, nonce string) (bool, error) {
	key := dPoPNoncePrefix + nonce
	result, err := s.client.GetDel(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis validate nonce: %w", err)
	}
	return result == "challenge", nil
}

// BindKeyToToken creates a binding between a DPoP key and a token.
// This ensures a token can only be used with proofs from the same key.
func (s *DPoPStore) BindKeyToToken(ctx context.Context, tokenJTI, keyThumbprint string, ttl time.Duration) error {
	key := fmt.Sprintf("dpop_binding:%s", tokenJTI)
	if err := s.client.Set(ctx, key, keyThumbprint, ttl).Err(); err != nil {
		return fmt.Errorf("redis bind key: %w", err)
	}
	return nil
}

// GetKeyBinding retrieves the DPoP key binding for a token.
// Returns the key thumbprint that should be used with this token.
func (s *DPoPStore) GetKeyBinding(ctx context.Context, tokenJTI string) (string, error) {
	key := fmt.Sprintf("dpop_binding:%s", tokenJTI)
	result, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("redis get binding: %w", err)
	}
	return result, nil
}

// GenerateNonce generates a random server nonce for DPoP challenges.
func GenerateNonce() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:])
}
