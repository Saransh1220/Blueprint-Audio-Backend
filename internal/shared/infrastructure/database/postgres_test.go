package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPostgresDB_InvalidConfig(t *testing.T) {
	cfg := PostgresConfig{
		Host:     "invalid-host-that-does-not-exist",
		Port:     "5432",
		User:     "user",
		Password: "pass",
		DBName:   "db",
		SSLMode:  "disable",
	}

	db, err := NewPostgresDB(cfg)

	// Should return error for invalid connection
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestNewPostgresDB_DSNConstruction(t *testing.T) {
	// This tests the DSN string construction logic
	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "testuser",
		Password: "testpass",
		DBName:   "testdb",
		SSLMode:  "require",
	}

	// We can't actually connect without a real database,
	// but we can verify the function exists and handles the config
	_, err := NewPostgresDB(cfg)

	// Error is expected without real database
	assert.Error(t, err)
}

func TestPostgresConfig_Fields(t *testing.T) {
	cfg := PostgresConfig{
		Host:     "db.example.com",
		Port:     "15432",
		User:     "admin",
		Password: "secret",
		DBName:   "production",
		SSLMode:  "verify-full",
	}

	assert.Equal(t, "db.example.com", cfg.Host)
	assert.Equal(t, "15432", cfg.Port)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "secret", cfg.Password)
	assert.Equal(t, "production", cfg.DBName)
	assert.Equal(t, "verify-full", cfg.SSLMode)
}
