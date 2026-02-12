package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear environment
	os.Clearenv()

	// Set required vars
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "user")
	os.Setenv("DB_PASSWORD", "pass")
	os.Setenv("DB_NAME", "test")

	cfg := Load()

	// Verify defaults
	assert.Equal(t, "8080", cfg.Server.Port)
	assert.Equal(t, "http://localhost:4200", cfg.Server.AllowedOrigins)
	assert.Equal(t, "default-dev-secret", cfg.JWT.Secret)
	assert.Equal(t, 24*time.Hour, cfg.JWT.Expiry)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, "5432", cfg.Database.Port)
}

func TestLoad_CustomValues(t *testing.T) {
	os.Clearenv()

	// Set custom values
	os.Setenv("PORT", "9000")
	os.Setenv("ALLOWED_ORIGINS", "https://example.com")
	os.Setenv("JWT_SECRET", "my-secret")
	os.Setenv("JWT_EXPIRATION", "2h")
	os.Setenv("DB_HOST", "db-server")
	os.Setenv("DB_PORT", "15432")
	os.Setenv("DB_USER", "admin")
	os.Setenv("DB_PASSWORD", "secret")
	os.Setenv("DB_NAME", "production")
	os.Setenv("DB_SSLMODE", "require")
	os.Setenv("REDIS_HOST", "redis-server")
	os.Setenv("REDIS_PORT", "6380")

	cfg := Load()

	// Verify custom values
	assert.Equal(t, "9000", cfg.Server.Port)
	assert.Equal(t, "https://example.com", cfg.Server.AllowedOrigins)
	assert.Equal(t, "my-secret", cfg.JWT.Secret)
	assert.Equal(t, 2*time.Hour, cfg.JWT.Expiry)
	assert.Equal(t, "db-server", cfg.Database.Host)
	assert.Equal(t, "15432", cfg.Database.Port)
	assert.Equal(t, "admin", cfg.Database.User)
	assert.Equal(t, "secret", cfg.Database.Password)
	assert.Equal(t, "production", cfg.Database.DBName)
	assert.Equal(t, "require", cfg.Database.SSLMode)
	assert.Equal(t, "redis-server", cfg.Redis.Host)
	assert.Equal(t, "6380", cfg.Redis.Port)
}

func TestLoad_JWTExpirationParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected time.Duration
	}{
		{"hours", "48h", 48 * time.Hour},
		{"minutes", "30m", 30 * time.Minute},
		{"mixed", "1h30m", 90 * time.Minute},
		{"invalid_uses_default", "invalid", 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("DB_HOST", "localhost")
			os.Setenv("DB_PORT", "5432")
			os.Setenv("DB_USER", "user")
			os.Setenv("DB_PASSWORD", "pass")
			os.Setenv("DB_NAME", "test")
			os.Setenv("JWT_EXPIRATION", tt.value)

			cfg := Load()
			assert.Equal(t, tt.expected, cfg.JWT.Expiry)
		})
	}
}
