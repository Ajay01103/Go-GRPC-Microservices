package redisstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-grpc-sqlc/pkg/redisclient"
)

const (
	refreshActivePrefix    = "rt:active:"
	refreshBlacklistPrefix = "rt:blacklist:"
	refreshRotatedPrefix   = "rt:rotated:"
	refreshUserSetPrefix   = "rt:user:"
)

const rotatedGraceTTL = 15 * time.Second

// atomicRotateFamilyScript verifies active family state and rotates it in one
// round-trip, while blacklisting the old token hash.
//
// KEYS[1] active family key (rt:active:{familyId})
// KEYS[2] blacklist key for old hash (rt:blacklist:{oldHash})
// ARGV[1] expected user id
// ARGV[2] expected old token hash
// ARGV[3] expected old jkt
// ARGV[4] blacklist ttl seconds
// ARGV[5] new active record json
// ARGV[6] active ttl seconds
//
// Returns one of: OK, FAMILY_NOT_FOUND, USER_MISMATCH, HASH_MISMATCH, JKT_MISMATCH, KID_MISMATCH
var atomicRotateFamilyScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return 'FAMILY_NOT_FOUND'
end

local rec = cjson.decode(raw)
if rec['user_id'] ~= ARGV[1] then
  return 'USER_MISMATCH'
end
if rec['token_hash'] ~= ARGV[2] then
  return 'HASH_MISMATCH'
end

local expectedJKT = ARGV[3]
local storedJKT = rec['jkt']
if not storedJKT then
  storedJKT = ''
end
if expectedJKT ~= '' and storedJKT ~= expectedJKT then
  return 'JKT_MISMATCH'
end

local storedKID = rec['signing_kid'] or ''
local expectedKID = ARGV[8]
if storedKID ~= '' and expectedKID ~= '' and storedKID ~= expectedKID then
  return 'KID_MISMATCH'
end

redis.call('SETEX', KEYS[2], ARGV[4], 'revoked')
redis.call('SETEX', KEYS[1], ARGV[6], ARGV[5])
redis.call('SADD',  KEYS[3], ARGV[7])
return 'OK'
`)

// TokenStore manages refresh token lifecycle in Redis using:
// - allowlist: rt:active:{familyId}
// - blacklist: rt:blacklist:{tokenHash}
// - user families: rt:user:{userId}:families
type TokenStore struct {
	client *redis.Client
}

type ActiveRefreshTokenRecord struct {
	UserID     string `json:"user_id"`
	TokenHash  string `json:"token_hash"`
	JKT        string `json:"jkt,omitempty"`
	ExpiresAt  string `json:"expires_at"`
	RefreshJTI string `json:"refresh_jti"`
	SigningKID string `json:"signing_kid"`
	IssuedAt   int64  `json:"issued_at"`
}

type RotateOutcome string

const (
	RotateSuccess        RotateOutcome = "OK"
	RotateFamilyNotFound RotateOutcome = "FAMILY_NOT_FOUND"
	RotateUserMismatch   RotateOutcome = "USER_MISMATCH"
	RotateHashMismatch   RotateOutcome = "HASH_MISMATCH"
	RotateJKTMismatch    RotateOutcome = "JKT_MISMATCH"
	RotateKIDMismatch    RotateOutcome = "KID_MISMATCH"
)

// New creates a TokenStore from a connected Redis client.
func New(client *redis.Client) *TokenStore {
	return &TokenStore{client: client}
}

// NewClientFromURL parses the Redis URL and returns a connected client.
func NewClientFromURL(redisURL string) (*redis.Client, error) {
	return redisclient.NewClientFromURL(redisURL)
}

func activeFamilyKey(familyID string) string {
	return refreshActivePrefix + familyID
}

func blacklistKey(tokenHash string) string {
	return refreshBlacklistPrefix + tokenHash
}

func userFamiliesKey(userID string) string {
	return fmt.Sprintf("%s%s:families", refreshUserSetPrefix, userID)
}

func rotatedGraceKey(tokenHash string) string {
	return refreshRotatedPrefix + tokenHash
}

// RefreshTokenState captures the refresh-token state needed by the auth service
// in a single Redis round-trip.
type RefreshTokenState struct {
	Blacklisted   bool
	GraceFamilyID string
	FamilyKID     string
	ActiveRecord   *ActiveRefreshTokenRecord
}

// LoadRefreshTokenState fetches blacklist, rotated-grace, and active family data
// together so the refresh path does not pay multiple sequential Redis RTTs.
func (s *TokenStore) LoadRefreshTokenState(ctx context.Context, familyID, tokenHash string) (*RefreshTokenState, error) {
	pipe := s.client.Pipeline()
	blacklistCmd := pipe.Get(ctx, blacklistKey(tokenHash))
	graceCmd := pipe.Get(ctx, rotatedGraceKey(tokenHash))
	activeCmd := pipe.Get(ctx, activeFamilyKey(familyID))

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis load refresh state: %w", err)
	}

	state := &RefreshTokenState{}

	blacklistVal, err := blacklistCmd.Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis get blacklist: %w", err)
	}
	state.Blacklisted = err == nil && blacklistVal != ""

	graceFamilyID, err := graceCmd.Result()
	if err == nil {
		state.GraceFamilyID = graceFamilyID
	} else if err != redis.Nil {
		return nil, fmt.Errorf("redis get rotated grace key: %w", err)
	}

	raw, err := activeCmd.Result()
	if err == redis.Nil {
		return nil, ErrFamilyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get active family: %w", err)
	}

	var rec ActiveRefreshTokenRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return nil, fmt.Errorf("unmarshal active record: %w", err)
	}
	state.ActiveRecord = &rec
	state.FamilyKID = rec.SigningKID

	return state, nil
}

func (s *TokenStore) StoreFamilyActiveToken(ctx context.Context, familyID string, rec ActiveRefreshTokenRecord, ttl time.Duration) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal active record: %w", err)
	}
	if err := s.client.Set(ctx, activeFamilyKey(familyID), raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis set active family: %w", err)
	}
	if err := s.upsertUserFamily(ctx, rec.UserID, familyID); err != nil {
		return err
	}
	return nil
}

func (s *TokenStore) GetFamilyActiveToken(ctx context.Context, familyID string) (*ActiveRefreshTokenRecord, error) {
	raw, err := s.client.Get(ctx, activeFamilyKey(familyID)).Result()
	if err == redis.Nil {
		return nil, ErrFamilyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get active family: %w", err)
	}
	var rec ActiveRefreshTokenRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return nil, fmt.Errorf("unmarshal active record: %w", err)
	}
	return &rec, nil
}

func (s *TokenStore) IsTokenHashBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	_, err := s.client.Get(ctx, blacklistKey(tokenHash)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis get blacklist: %w", err)
	}
	return true, nil
}

func (s *TokenStore) BlacklistTokenHash(ctx context.Context, tokenHash string, ttl time.Duration) error {
	if err := s.client.Set(ctx, blacklistKey(tokenHash), "revoked", ttl).Err(); err != nil {
		return fmt.Errorf("redis set blacklist: %w", err)
	}
	return nil
}

func (s *TokenStore) RotateFamilyActiveToken(
	ctx context.Context,
	familyID, userID, oldTokenHash, oldJKT, signingKID string,
	newRecord ActiveRefreshTokenRecord,
	activeTTL, blacklistTTL time.Duration,
) (RotateOutcome, error) {
	raw, err := json.Marshal(newRecord)
	if err != nil {
		return "", fmt.Errorf("marshal new active record: %w", err)
	}

	result, err := atomicRotateFamilyScript.Run(ctx, s.client,
		[]string{activeFamilyKey(familyID), blacklistKey(oldTokenHash), userFamiliesKey(userID)},
		userID,
		oldTokenHash,
		oldJKT,
		int64(blacklistTTL.Seconds()),
		string(raw),
		int64(activeTTL.Seconds()),
		familyID,
		signingKID,
	).Result()
	if err != nil {
		return "", fmt.Errorf("redis rotate family: %w", err)
	}

	outcome, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected redis rotate response type: %T", result)
	}

	if RotateOutcome(outcome) == RotateSuccess {
		if err := s.client.Set(ctx, rotatedGraceKey(oldTokenHash), familyID, rotatedGraceTTL).Err(); err != nil {
			// Best effort marker: missing grace should degrade to strict anti-replay behavior.
		}
	}

	return RotateOutcome(outcome), nil
}

func (s *TokenStore) RevokeFamily(ctx context.Context, familyID string, blacklistTTL time.Duration) error {
	rec, err := s.GetFamilyActiveToken(ctx, familyID)
	if err != nil && err != ErrFamilyNotFound {
		return err
	}
	if rec != nil && rec.TokenHash != "" {
		if blErr := s.BlacklistTokenHash(ctx, rec.TokenHash, blacklistTTL); blErr != nil {
			return blErr
		}
	}
	if err := s.client.Del(ctx, activeFamilyKey(familyID)).Err(); err != nil {
		return fmt.Errorf("redis del active family: %w", err)
	}
	return nil
}

func (s *TokenStore) LogoutFamily(ctx context.Context, userID, familyID, tokenHash string, blacklistTTL time.Duration) error {
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, activeFamilyKey(familyID))
	pipe.SRem(ctx, userFamiliesKey(userID), familyID)
	if tokenHash != "" {
		pipe.Set(ctx, blacklistKey(tokenHash), "revoked", blacklistTTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis logout family: %w", err)
	}
	return nil
}

func (s *TokenStore) RevokeAllUserFamilies(ctx context.Context, userID string, blacklistTTL time.Duration) error {
	families, err := s.listUserFamiliesPruned(ctx, userID)
	if err != nil {
		return err
	}
	if len(families) == 0 {
		return nil
	}

	pipe := s.client.TxPipeline()
	for _, familyID := range families {
		rec, recErr := s.GetFamilyActiveToken(ctx, familyID)
		if recErr != nil && recErr != ErrFamilyNotFound {
			return recErr
		}
		if rec != nil && rec.TokenHash != "" {
			pipe.Set(ctx, blacklistKey(rec.TokenHash), "revoked", blacklistTTL)
		}
		pipe.Del(ctx, activeFamilyKey(familyID))
	}
	pipe.Del(ctx, userFamiliesKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis revoke all families: %w", err)
	}

	return nil
}

func HashTokenSHA256(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}

func (s *TokenStore) AddFamilyToUser(ctx context.Context, userID, familyID string) error {
	return s.upsertUserFamily(ctx, userID, familyID)
}

func (s *TokenStore) RemoveFamilyFromUser(ctx context.Context, userID, familyID string) error {
	if err := s.client.SRem(ctx, userFamiliesKey(userID), familyID).Err(); err != nil {
		return fmt.Errorf("redis srem family: %w", err)
	}
	return nil
}

func (s *TokenStore) ListUserFamilies(ctx context.Context, userID string) ([]string, error) {
	return s.listUserFamiliesPruned(ctx, userID)
}

func (s *TokenStore) upsertUserFamily(ctx context.Context, userID, familyID string) error {
	pipe := s.client.TxPipeline()
	pipe.SAdd(ctx, userFamiliesKey(userID), familyID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis upsert user family: %w", err)
	}
	return nil
}

func (s *TokenStore) listUserFamiliesPruned(ctx context.Context, userID string) ([]string, error) {
	families, err := s.client.SMembers(ctx, userFamiliesKey(userID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis smembers families: %w", err)
	}
	if len(families) == 0 {
		return nil, nil
	}

	pipe := s.client.Pipeline()
	existsCmds := make([]*redis.IntCmd, len(families))
	for idx, familyID := range families {
		existsCmds[idx] = pipe.Exists(ctx, activeFamilyKey(familyID))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis exists family keys: %w", err)
	}

	activeFamilies := make([]string, 0, len(families))
	staleFamilies := make([]string, 0)
	for idx, familyID := range families {
		exists, existsErr := existsCmds[idx].Result()
		if existsErr != nil {
			return nil, fmt.Errorf("redis exists family key: %w", existsErr)
		}
		if exists > 0 {
			activeFamilies = append(activeFamilies, familyID)
			continue
		}
		staleFamilies = append(staleFamilies, familyID)
	}

	if len(staleFamilies) > 0 {
		staleArgs := make([]interface{}, len(staleFamilies))
		for i := range staleFamilies {
			staleArgs[i] = staleFamilies[i]
		}
		if err := s.client.SRem(ctx, userFamiliesKey(userID), staleArgs...).Err(); err != nil {
			return nil, fmt.Errorf("redis srem stale families: %w", err)
		}
	}

	return activeFamilies, nil
}

// GetFamilyKID returns the signing_kid stored in the active family record.
func (s *TokenStore) GetFamilyKID(ctx context.Context, familyID string) (string, error) {
	raw, err := s.client.Get(ctx, activeFamilyKey(familyID)).Result()
	if err == redis.Nil {
		return "", ErrFamilyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redis get family kid: %w", err)
	}

	var partial struct {
		SigningKID string `json:"signing_kid"`
	}
	if err := json.Unmarshal([]byte(raw), &partial); err != nil {
		return "", fmt.Errorf("unmarshal family kid: %w", err)
	}
	return partial.SigningKID, nil
}

func (s *TokenStore) GetRotatedTokenGraceFamilyID(ctx context.Context, oldTokenHash string) (string, error) {
	value, err := s.client.Get(ctx, rotatedGraceKey(oldTokenHash)).Result()
	if err == redis.Nil {
		return "", ErrGraceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redis get rotated grace key: %w", err)
	}

	return value, nil
}
