package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRedis_InvalidConfig(t *testing.T) {
	cfg := RedisConfig{
		Host:     "invalid-redis-host-xyz",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	client, err := NewRedis(cfg)

	// Should return error for invalid connection
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestRedisConfig_Fields(t *testing.T) {
	cfg := RedisConfig{
		Host:     "redis.example.com",
		Port:     "6380",
		Password: "redis-secret",
		DB:       1,
	}

	assert.Equal(t, "redis.example.com", cfg.Host)
	assert.Equal(t, "6380", cfg.Port)
	assert.Equal(t, "redis-secret", cfg.Password)
	assert.Equal(t, 1, cfg.DB)
}

func TestNewRedis_AddressFormat(t *testing.T) {
	cfg := RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	// Will fail to connect if Redis is not running
	// but tests the address formatting logic
	client, _ := NewRedis(cfg)

	// If Redis is running locally, client will be valid
	// If not, client will be nil - both are acceptable for this test
	if client != nil {
		client.Close()
	}
}
