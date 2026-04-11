package redisstore

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewClientFromURL(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	return redis.NewClient(opts), nil
}
