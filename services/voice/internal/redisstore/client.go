package redisstore

import (
	"github.com/redis/go-redis/v9"

	"github.com/go-grpc-sqlc/pkg/redisclient"
)

// NewClientFromURL parses the Redis URL and returns a configured client.
func NewClientFromURL(redisURL string) (*redis.Client, error) {
	return redisclient.NewClientFromURL(redisURL)
}
