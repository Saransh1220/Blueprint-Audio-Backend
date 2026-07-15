package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, "development", cfg.Server.Environment)
	assert.False(t, cfg.Server.SecureCookies)
	assert.Equal(t, "default-dev-secret", cfg.JWT.Secret)
	assert.Equal(t, 24*time.Hour, cfg.JWT.Expiry)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, "5432", cfg.Database.Port)
	assert.False(t, cfg.Migration.AutoRun)
	assert.Equal(t, "db/migrations", cfg.Migration.Path)
	assert.True(t, cfg.Redis.Enabled)
}

func TestConfigHelpers(t *testing.T) {
	t.Setenv("CONFIG_HELPER_VALUE", "")
	assert.Equal(t, "fallback", getEnv("CONFIG_HELPER_VALUE", "fallback"))
	assert.Equal(t, time.Minute, parseDuration("not-a-duration", time.Minute))
	originalFromFile, hadFromFile := os.LookupEnv("FROM_FILE")
	_ = os.Unsetenv("FROM_FILE")
	t.Cleanup(func() {
		if hadFromFile {
			_ = os.Setenv("FROM_FILE", originalFromFile)
		} else {
			_ = os.Unsetenv("FROM_FILE")
		}
	})
	workspace, err := filepath.Abs(".")
	require.NoError(t, err)
	t.Setenv("TEMP", workspace)
	path := filepath.Join(t.TempDir(), "config-helper.test.env")
	require.NoError(t, os.WriteFile(path, []byte("# comment\nFROM_FILE = 'value'\nBROKEN\n"), 0o600))
	loadDotEnvIfPresent(path)
	assert.Equal(t, "value", os.Getenv("FROM_FILE"))
}

func TestLoad_CustomValues(t *testing.T) {
	os.Clearenv()

	// Set custom values
	os.Setenv("PORT", "9000")
	os.Setenv("ALLOWED_ORIGINS", "https://example.com")
	os.Setenv("ENV", "production")
	os.Setenv("JWT_SECRET", "my-secret")
	os.Setenv("JWT_EXPIRATION", "2h")
	os.Setenv("DB_HOST", "db-server")
	os.Setenv("DB_PORT", "15432")
	os.Setenv("DB_USER", "admin")
	os.Setenv("DB_PASSWORD", "secret")
	os.Setenv("DB_NAME", "production")
	os.Setenv("DB_SSLMODE", "require")
	os.Setenv("AUTO_MIGRATE", "true")
	os.Setenv("MIGRATIONS_PATH", "/app/migrations")
	os.Setenv("REDIS_ENABLED", "false")
	os.Setenv("REDIS_HOST", "redis-server")
	os.Setenv("REDIS_PORT", "6380")

	cfg := Load()

	// Verify custom values
	assert.Equal(t, "9000", cfg.Server.Port)
	assert.Equal(t, "https://example.com", cfg.Server.AllowedOrigins)
	assert.Equal(t, "production", cfg.Server.Environment)
	assert.True(t, cfg.Server.SecureCookies)
	assert.Equal(t, "my-secret", cfg.JWT.Secret)
	assert.Equal(t, 2*time.Hour, cfg.JWT.Expiry)
	assert.Equal(t, "db-server", cfg.Database.Host)
	assert.Equal(t, "15432", cfg.Database.Port)
	assert.Equal(t, "admin", cfg.Database.User)
	assert.Equal(t, "secret", cfg.Database.Password)
	assert.Equal(t, "production", cfg.Database.DBName)
	assert.Equal(t, "require", cfg.Database.SSLMode)
	assert.True(t, cfg.Migration.AutoRun)
	assert.Equal(t, "/app/migrations", cfg.Migration.Path)
	assert.False(t, cfg.Redis.Enabled)
	assert.Equal(t, "redis-server", cfg.Redis.Host)
	assert.Equal(t, "6380", cfg.Redis.Port)
}

func TestLoad_DodoAndCurrencyConfig(t *testing.T) {
	os.Clearenv()
	os.Setenv("DODO_PAYMENTS_API_KEY", "dodo_key")
	os.Setenv("DODO_PAYMENTS_PRODUCT_ID", "pdt_123")
	os.Setenv("DODO_PAYMENTS_WEBHOOK_KEY", "whsec_123")
	os.Setenv("DODO_PAYMENTS_API_URL", "https://test.dodopayments.com")
	os.Setenv("INR_USD_RATE", "0.0119")

	cfg := Load()

	assert.Equal(t, "dodo_key", cfg.Dodo.APIKey)
	assert.Equal(t, "pdt_123", cfg.Dodo.ProductID)
	assert.Equal(t, "whsec_123", cfg.Dodo.WebhookKey)
	assert.Equal(t, "https://test.dodopayments.com", cfg.Dodo.APIURL)
	assert.Equal(t, "0.0119", cfg.Currency.INRUSDRate)
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
