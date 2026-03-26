package redisstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const refreshTokenPrefix = "refresh_token:"

// atomicRotateScript atomically validates that the old JTI key exists, deletes it,
// and stores the new JTI — all in a single Redis round-trip.
//
// KEYS[1] = old JTI key  (refresh_token:{oldJTI})
// KEYS[2] = new JTI key  (refresh_token:{newJTI})
// ARGV[1] = TTL in seconds for the new key
// ARGV[2] = userID value to store under the new key
//
// Returns "OK" on success, or a Redis error reply "TOKEN_REVOKED" when the old
// key is missing (already consumed / never existed — treat as revoked).
var atomicRotateScript = redis.NewScript(`
local val = redis.call('GET', KEYS[1])
if not val then
  return redis.error_reply('TOKEN_REVOKED')
end
redis.call('DEL', KEYS[1])
redis.call('SETEX', KEYS[2], ARGV[1], ARGV[2])
return 'OK'
`)

// atomicClaimScript atomically checks that a JTI key exists and deletes it.
// Used as the fast-fail atomic gate at the start of the refresh flow — before
// any database or token-minting work — to close the concurrent-request race.
//
// KEYS[1] = JTI key to claim (refresh_token:{jti})
//
// Returns "OK" on success, or "TOKEN_REVOKED" if the key was absent.
var atomicClaimScript = redis.NewScript(`
local val = redis.call('GET', KEYS[1])
if not val then
  return redis.error_reply('TOKEN_REVOKED')
end
redis.call('DEL', KEYS[1])
return 'OK'
`)

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
// userID is stored as a string value (matches the sqlc-generated db.User.ID type).
// Call this after successfully minting a refresh token.
func (s *TokenStore) StoreRefreshToken(ctx context.Context, jti, userID string, ttl time.Duration) error {
	return s.client.Set(ctx, redisKey(jti), userID, ttl).Err()
}

// ValidateRefreshToken checks if the refresh token JTI is still valid (not revoked).
// Returns the user ID associated with the JTI, or ErrTokenRevoked if the key is missing.
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

// ClaimRefreshToken atomically asserts the JTI key exists then deletes it in one
// Lua round-trip — the fast-fail gate at the start of the refresh flow.
//
// Once claimed, the old session is unconditionally gone. On success, the caller
// must store a new JTI via StoreRefreshToken to re-establish a session.
//
// Returns ErrTokenRevoked when the key is absent: this means either a legitimate
// logout, a concurrent replay, or a stolen-token replay — all should be refused.
func (s *TokenStore) ClaimRefreshToken(ctx context.Context, jti string) error {
	result := atomicClaimScript.Run(ctx, s.client, []string{redisKey(jti)})
	if err := result.Err(); err != nil {
		if strings.Contains(err.Error(), "TOKEN_REVOKED") {
			return ErrTokenRevoked
		}
		return fmt.Errorf("redis claim: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically validates that oldJTI exists in Redis, deletes it,
// and stores newJTI — all in a single Lua script (one round-trip, zero race window).
//
// Prefer ClaimRefreshToken + StoreRefreshToken when you need to perform work
// (DB fetch, token minting) between the two operations.
//
// Returns ErrTokenRevoked when oldJTI is already gone, which signals a concurrent
// replay or a theft attempt.
func (s *TokenStore) RotateRefreshToken(
	ctx context.Context,
	oldJTI, newJTI, userID string,
	ttl time.Duration,
) error {
	result := atomicRotateScript.Run(ctx, s.client,
		[]string{redisKey(oldJTI), redisKey(newJTI)},
		int64(ttl.Seconds()),
		userID,
	)
	if err := result.Err(); err != nil {
		if strings.Contains(err.Error(), "TOKEN_REVOKED") {
			return ErrTokenRevoked
		}
		return fmt.Errorf("redis rotate: %w", err)
	}
	return nil
}

// RevokeAllUserTokens scans Redis for every refresh token belonging to userID
// and deletes them all — used when token reuse is detected (possible theft).
//
// Uses SCAN to iterate the keyspace in pages so it never blocks the Redis server.
// This is an emergency / security path, not the hot path.
func (s *TokenStore) RevokeAllUserTokens(ctx context.Context, userID string) error {
	pattern := refreshTokenPrefix + "*"
	var cursor uint64
	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("redis scan: %w", err)
		}
		for _, key := range keys {
			val, getErr := s.client.Get(ctx, key).Result()
			if getErr != nil {
				continue // key expired between SCAN and GET — harmless
			}
			if val == userID {
				s.client.Del(ctx, key) //nolint:errcheck — best-effort revocation
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
