package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadAppConfig_Defaults(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "d")
	t.Setenv("PORT", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_EXPIRATION", "")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg := loadAppConfig()
	assert.Contains(t, cfg.dsn, "host=localhost")
	assert.Equal(t, "8080", cfg.port)
	assert.Equal(t, "default-dev-secret", cfg.jwtSecret)
	assert.Equal(t, 24*time.Hour, cfg.jwtExpiry)
	assert.Equal(t, "http://localhost:4200", cfg.allowedOrigins)
}

func TestLoadAppConfig_Custom(t *testing.T) {
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "15432")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "pass")
	t.Setenv("DB_NAME", "blueprint")
	t.Setenv("PORT", "9999")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("JWT_EXPIRATION", "2h")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com")

	cfg := loadAppConfig()
	assert.Contains(t, cfg.dsn, "port=15432")
	assert.Contains(t, cfg.dsn, "dbname=blueprint")
	assert.Equal(t, "9999", cfg.port)
	assert.Equal(t, "secret", cfg.jwtSecret)
	assert.Equal(t, 2*time.Hour, cfg.jwtExpiry)
	assert.Equal(t, "https://app.example.com", cfg.allowedOrigins)
}
