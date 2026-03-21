package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const refreshTokenPrefix = "refresh_token:"

// TokenStore manages refresh token lifecycle in Redis.
// Each refresh token is stored as:
//
//	KEY:  refresh_token:{jti}
//	VAL:  {user_id}
//	TTL:  duration of refresh token
type TokenStore struct {
	client *redis.Client
}

// New creates a TokenStore from a connected Redis client.
func New(client *redis.Client) *TokenStore {
	return &TokenStore{client: client}
}

// NewClientFromURL parses the Redis URL and returns a connected client.
func NewClientFromURL(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)
	return client, nil
}

// redisKey returns the full Redis key for a given refresh token JTI.
func redisKey(jti string) string {
	return refreshTokenPrefix + jti
}

// StoreRefreshToken persists a refresh token JTI in Redis.
// Call this immediately after minting a refresh token.
func (s *TokenStore) StoreRefreshToken(ctx context.Context, jti, userID string, ttl time.Duration) error {
	return s.client.Set(ctx, redisKey(jti), userID, ttl).Err()
}

// ValidateRefreshToken checks if the refresh token JTI is still valid (not revoked).
// Returns the user ID associated with the JTI, or an error if it doesn't exist.
func (s *TokenStore) ValidateRefreshToken(ctx context.Context, jti string) (string, error) {
	userID, err := s.client.Get(ctx, redisKey(jti)).Result()
	if err == redis.Nil {
		return "", ErrTokenRevoked
	}
	if err != nil {
		return "", fmt.Errorf("redis get error: %w", err)
	}
	return userID, nil
}

// RevokeRefreshToken deletes a refresh token from Redis, effectively logging out the user.
// Returns nil if the key didn't exist (idempotent).
func (s *TokenStore) RevokeRefreshToken(ctx context.Context, jti string) error {
	if err := s.client.Del(ctx, redisKey(jti)).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("redis del error: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically revokes the old JTI and stores the new one.
// This is called during token refresh to prevent replay attacks.
func (s *TokenStore) RotateRefreshToken(ctx context.Context, oldJTI, newJTI, userID string, ttl time.Duration) error {
	pipe := s.client.Pipeline()
	pipe.Del(ctx, redisKey(oldJTI))
	pipe.Set(ctx, redisKey(newJTI), userID, ttl)
	_, err := pipe.Exec(ctx)
	return err
}
