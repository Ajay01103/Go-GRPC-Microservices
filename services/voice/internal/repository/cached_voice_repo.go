package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	voiceCacheTTL = 10 * time.Minute
	listCacheTTL  = 15 * time.Minute
	voiceDocTTL   = 30 * time.Minute
)

// CachedVoiceRepo adds Redis cache and RediSearch acceleration over the base repo.
type CachedVoiceRepo struct {
	repo   Repository
	redis  *redis.Client
	logger *zap.Logger
	// redisSearchEnabled gates FT.* usage when RediSearch is unavailable.
	redisSearchEnabled bool
}

// NewCachedVoiceRepo wraps a base repository with Redis cache behavior.
func NewCachedVoiceRepo(repo Repository, redisClient *redis.Client, logger *zap.Logger) *CachedVoiceRepo {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CachedVoiceRepo{repo: repo, redis: redisClient, logger: logger, redisSearchEnabled: true}
}

// SetRediSearchEnabled toggles FT.* usage at runtime.
func (c *CachedVoiceRepo) SetRediSearchEnabled(enabled bool) {
	c.redisSearchEnabled = enabled
}

func voiceCacheKey(userID, id string) string {
	return fmt.Sprintf("voice:cache:%s:%s", userID, id)
}

func voiceDocKey(userID, id string) string {
	return fmt.Sprintf("voice:doc:%s:%s", userID, id)
}

func listCacheKey(scope ListScope, userID string) string {
	if scope == "" {
		scope = ListScopeAll
	}
	return fmt.Sprintf("voices:list:%s:%s", scope, userID)
}

// GetVoiceByID fetches a voice by id, then caches and indexes it.
func (c *CachedVoiceRepo) GetVoiceByID(ctx context.Context, id string) (db.Voice, error) {
	voice, err := c.repo.GetVoiceByID(ctx, id)
	if err != nil {
		return db.Voice{}, err
	}

	c.cacheVoice(ctx, voice)
	if c.redisSearchEnabled {
		c.upsertVoiceDoc(ctx, voice)
	}

	return voice, nil
}

// GetVoiceByIDAndUser performs cache-aside lookup for a single voice.
func (c *CachedVoiceRepo) GetVoiceByIDAndUser(ctx context.Context, id, userID string) (db.Voice, error) {
	key := voiceCacheKey(userID, id)
	cached, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var voice db.Voice
		if unmarshalErr := json.Unmarshal(cached, &voice); unmarshalErr == nil {
			return voice, nil
		}
	}

	voice, err := c.repo.GetVoiceByIDAndUser(ctx, id, userID)
	if err != nil {
		return db.Voice{}, err
	}

	c.cacheVoice(ctx, voice)
	if c.redisSearchEnabled {
		c.upsertVoiceDoc(ctx, voice)
	}

	return voice, nil
}

// ListVoices caches non-search list reads and uses RediSearch for query paths.
func (c *CachedVoiceRepo) ListVoices(ctx context.Context, params ListVoicesParams) ([]db.ListCustomVoicesRow, error) {
	if strings.TrimSpace(params.Query) != "" {
		return c.searchVoices(ctx, params)
	}

	key := listCacheKey(params.Scope, params.UserID)
	cached, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var rows []db.ListCustomVoicesRow
		if unmarshalErr := json.Unmarshal(cached, &rows); unmarshalErr == nil {
			return rows, nil
		}
	}

	rows, err := c.repo.ListVoices(ctx, params)
	if err != nil {
		return nil, err
	}

	if payload, marshalErr := json.Marshal(rows); marshalErr == nil {
		if setErr := c.redis.Set(ctx, key, payload, listCacheTTL).Err(); setErr != nil {
			c.logger.Warn("list cache set failed", zap.Error(setErr), zap.String("key", key))
		}
	}

	if !c.redisSearchEnabled {
		return rows, nil
	}

	// Only index unambiguous scoped reads. All-scope mixes ownership and cannot
	// be safely tagged without owner metadata in list rows.
	switch params.Scope {
	case ListScopeCustom:
		c.upsertRowsAsDocs(ctx, rows, params.UserID)
	case ListScopeSystem:
		c.upsertRowsAsDocs(ctx, rows, systemUserID)
	}

	return rows, nil
}

func (c *CachedVoiceRepo) searchVoices(ctx context.Context, params ListVoicesParams) ([]db.ListCustomVoicesRow, error) {
	if !c.redisSearchEnabled {
		return c.repo.ListVoices(ctx, params)
	}

	query := buildRediSearchQuery(params)

	result, err := c.redis.Do(ctx,
		"FT.SEARCH", redisVoiceSearchIndex, query,
		"RETURN", "7", "id", "name", "description", "category", "language", "variant", "userID",
		"LIMIT", "0", "200",
	).Slice()
	if err != nil {
		if isUnknownRedisCommandErr(err) {
			c.redisSearchEnabled = false
			c.logger.Warn("redisearch disabled; command unsupported by redis", zap.Error(err))
			return c.repo.ListVoices(ctx, params)
		}
		c.logger.Warn("redisearch failed, using db fallback", zap.Error(err), zap.String("query", query))
		return c.repo.ListVoices(ctx, params)
	}

	rows, parseErr := parseRediSearchResults(result)
	if parseErr != nil {
		c.logger.Warn("redisearch parse failed, using db fallback", zap.Error(parseErr))
		return c.repo.ListVoices(ctx, params)
	}
	return rows, nil
}

func buildRediSearchQuery(params ListVoicesParams) string {
	scope := params.Scope
	if scope == "" {
		scope = ListScopeAll
	}

	searchTerm := strings.TrimSpace(params.Query)
	searchTerm = escapeSearchTerm(searchTerm)
	if searchTerm == "" {
		searchTerm = "*"
	}

	switch scope {
	case ListScopeCustom:
		return fmt.Sprintf("@userID:{%s} @name|description:(%s)", escapeTagValue(params.UserID), searchTerm)
	case ListScopeSystem:
		return fmt.Sprintf("@userID:{%s} @name|description:(%s)", systemUserID, searchTerm)
	default:
		return fmt.Sprintf("(@userID:{%s}|@userID:{%s}) @name|description:(%s)", escapeTagValue(params.UserID), systemUserID, searchTerm)
	}
}

func escapeTagValue(value string) string {
	replacer := strings.NewReplacer(
		"-", `\\-`,
		"{", `\\{`,
		"}", `\\}`,
		"|", `\\|`,
		" ", `\\ `,
	)
	return replacer.Replace(value)
}

func escapeSearchTerm(value string) string {
	replacer := strings.NewReplacer(
		`\\`, " ",
		`"`, " ",
		"(", " ",
		")", " ",
		"|", " ",
		"@", " ",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func parseRediSearchResults(result []interface{}) ([]db.ListCustomVoicesRow, error) {
	if len(result) <= 1 {
		return nil, nil
	}

	rows := make([]db.ListCustomVoicesRow, 0, (len(result)-1)/2)
	for i := 1; i+1 < len(result); i += 2 {
		fields, ok := result[i+1].([]interface{})
		if !ok {
			continue
		}

		fieldMap := make(map[string]string)
		for j := 0; j+1 < len(fields); j += 2 {
			key := toString(fields[j])
			val := toString(fields[j+1])
			fieldMap[key] = val
		}

		desc := pgtype.Text{}
		if d, ok := fieldMap["description"]; ok && d != "" {
			desc = pgtype.Text{String: d, Valid: true}
		}

		category := db.VoiceCategory(fieldMap["category"])
		if category == "" {
			category = db.VoiceCategoryGENERAL
		}

		variant := db.VoiceVariant(fieldMap["variant"])
		if variant == "" {
			variant = db.VoiceVariantMALE
		}

		rows = append(rows, db.ListCustomVoicesRow{
			ID:          fieldMap["id"],
			Name:        fieldMap["name"],
			Description: desc,
			Category:    category,
			Language:    fieldMap["language"],
			Variant:     variant,
		})
	}

	return rows, nil
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func isUnknownRedisCommandErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command")
}

// DeleteVoice invalidates related cache entries after deleting from DB.
func (c *CachedVoiceRepo) DeleteVoice(ctx context.Context, id, userID string) error {
	if err := c.repo.DeleteVoice(ctx, id, userID); err != nil {
		return err
	}
	c.InvalidateVoice(ctx, id, userID)
	return nil
}

// InvalidateVoice removes single/list cache keys and search doc for a voice.
func (c *CachedVoiceRepo) InvalidateVoice(ctx context.Context, id, userID string) {
	keys := []string{
		voiceCacheKey(userID, id),
		listCacheKey(ListScopeCustom, userID),
		listCacheKey(ListScopeAll, userID),
	}
	if userID == systemUserID {
		keys = append(keys, listCacheKey(ListScopeSystem, systemUserID))
	}

	if err := c.redis.Del(ctx, keys...).Err(); err != nil {
		c.logger.Warn("cache invalidation delete failed", zap.Error(err))
	}

	docKey := voiceDocKey(userID, id)
	_ = c.redis.Del(ctx, docKey).Err()
	if c.redisSearchEnabled {
		_ = c.redis.Do(ctx, "FT.DEL", redisVoiceSearchIndex, docKey).Err()
	}

	if userID == systemUserID {
		c.deleteKeysByPattern(ctx, "voices:list:all:*")
	}
}

func (c *CachedVoiceRepo) deleteKeysByPattern(ctx context.Context, pattern string) {
	var cursor uint64
	for {
		keys, nextCursor, err := c.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			c.logger.Warn("cache pattern scan failed", zap.Error(err), zap.String("pattern", pattern))
			return
		}
		if len(keys) > 0 {
			if err := c.redis.Del(ctx, keys...).Err(); err != nil {
				c.logger.Warn("cache pattern delete failed", zap.Error(err), zap.String("pattern", pattern))
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

func (c *CachedVoiceRepo) cacheVoice(ctx context.Context, voice db.Voice) {
	payload, err := json.Marshal(voice)
	if err != nil {
		return
	}
	if err := c.redis.Set(ctx, voiceCacheKey(voice.UserID, voice.ID), payload, voiceCacheTTL).Err(); err != nil {
		c.logger.Warn("voice cache set failed", zap.Error(err), zap.String("voice_id", voice.ID))
	}
}

func (c *CachedVoiceRepo) upsertRowsAsDocs(ctx context.Context, rows []db.ListCustomVoicesRow, userID string) {
	for _, row := range rows {
		doc := map[string]interface{}{
			"id":       row.ID,
			"name":     row.Name,
			"category": string(row.Category),
			"language": row.Language,
			"variant":  string(row.Variant),
			"userID":   userID,
		}
		if row.Description.Valid {
			doc["description"] = row.Description.String
		} else {
			doc["description"] = ""
		}

		key := voiceDocKey(userID, row.ID)
		if err := c.redis.HSet(ctx, key, doc).Err(); err != nil {
			continue
		}
		_ = c.redis.Expire(ctx, key, voiceDocTTL).Err()
	}
}

func (c *CachedVoiceRepo) upsertVoiceDoc(ctx context.Context, voice db.Voice) {
	doc := map[string]interface{}{
		"id":       voice.ID,
		"name":     voice.Name,
		"category": string(voice.Category),
		"language": voice.Language,
		"variant":  string(voice.Variant),
		"userID":   voice.UserID,
	}
	if voice.Description.Valid {
		doc["description"] = voice.Description.String
	} else {
		doc["description"] = ""
	}

	key := voiceDocKey(voice.UserID, voice.ID)
	if err := c.redis.HSet(ctx, key, doc).Err(); err != nil {
		c.logger.Warn("redisearch doc upsert failed", zap.Error(err), zap.String("voice_id", voice.ID))
		return
	}
	if err := c.redis.Expire(ctx, key, voiceDocTTL).Err(); err != nil {
		c.logger.Warn("redisearch doc ttl failed", zap.Error(err), zap.String("voice_id", voice.ID))
	}
}
