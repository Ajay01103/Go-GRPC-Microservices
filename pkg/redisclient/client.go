package redisclient

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClientFromURL parses a Redis URL and returns a connected client.
func NewClientFromURL(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	return redis.NewClient(opts), nil
}