package storage

import (
	"context"
	"fmt"
	"time"

	"heart-rate-server/internal/config"

	"github.com/go-redis/redis/v8"
)

const RedisRequestTimeout = 5 * time.Second

func InitRedis(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), RedisRequestTimeout)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return client, nil
}
