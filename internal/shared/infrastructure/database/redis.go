package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// NewRedis creates a new Redis client
func NewRedis(cfg RedisConfig) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}
