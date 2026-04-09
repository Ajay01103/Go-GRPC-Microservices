package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

const redisVoiceSearchIndex = "voices_idx"

// BootstrapRediSearchIndex creates the voice search index if it does not exist.
func BootstrapRediSearchIndex(ctx context.Context, client *redis.Client) error {
	err := client.Do(ctx,
		"FT.CREATE", redisVoiceSearchIndex,
		"ON", "HASH",
		"PREFIX", "1", "voice:doc:",
		"SCHEMA",
		"id", "TEXT",
		"name", "TEXT", "WEIGHT", "5.0",
		"description", "TEXT",
		"category", "TAG",
		"language", "TAG",
		"variant", "TAG",
		"userID", "TAG",
	).Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "Index already exists") {
		return nil
	}
	return fmt.Errorf("repository: bootstrap redisearch index: %w", err)
}

// IsRediSearchUnsupportedError returns true when the Redis server does not support FT.* commands.
func IsRediSearchUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unknown command")
}
