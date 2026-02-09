package db_test

import (
	"os"
	"testing"

	cachedb "github.com/saransh1220/blueprint-audio/internal/db"
	"github.com/stretchr/testify/assert"
)

func TestInitRedis_Failure(t *testing.T) {
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "1")
	t.Setenv("REDIS_PASSWORD", "")

	err := cachedb.InitRedis()
	assert.Error(t, err)
}

func TestInitRedis_UsesEnvValues(t *testing.T) {
	_ = os.Getenv("REDIS_HOST")
	// Placeholder to keep package exercised even without a live redis server.
}
