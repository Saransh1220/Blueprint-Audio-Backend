package config

import (
	"os"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
)

// Config holds all configuration for the application
type Config struct {
	Server      ServerConfig
	Database    database.PostgresConfig
	Redis       database.RedisConfig
	JWT         JWTConfig
	Razorpay    RazorpayConfig
	FileStorage FileStorageConfig
	Google      GoogleConfig
}

// GoogleConfig holds Google OAuth configuration
type GoogleConfig struct {
	ClientID string
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port           string
	AllowedOrigins string
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

// RazorpayConfig holds Razorpay payment gateway configuration
type RazorpayConfig struct {
	KeyID     string
	KeySecret string
}

// FileStorageConfig holds file storage configuration
type FileStorageConfig struct {
	UseS3            bool
	S3Region         string
	S3Endpoint       string
	S3PublicEndpoint string
	S3AccessKey      string
	S3SecretKey      string
	S3BucketName     string
	S3UseSSL         bool
	LocalPath        string
}

// Load reads configuration from environment variables
func Load() Config {
	return Config{
		Server: ServerConfig{
			Port:           getEnv("PORT", "8080"),
			AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:4200"),
		},
		Database: database.PostgresConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "blueprint"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: database.RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       0,
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "default-dev-secret"),
			Expiry: parseDuration(getEnv("JWT_EXPIRATION", "24h"), 24*time.Hour),
		},
		Razorpay: RazorpayConfig{
			KeyID:     getEnv("RAZORPAY_KEY_ID", ""),
			KeySecret: getEnv("RAZORPAY_KEY_SECRET", ""),
		},
		FileStorage: FileStorageConfig{
			UseS3:            getEnv("USE_S3", "true") == "true",
			S3Region:         getEnv("S3_REGION", "us-east-1"),
			S3Endpoint:       getEnv("S3_ENDPOINT", ""),
			S3PublicEndpoint: getEnv("S3_PUBLIC_ENDPOINT", getEnv("S3_ENDPOINT", "")),
			S3AccessKey:      getEnv("S3_ACCESS_KEY", ""),
			S3SecretKey:      getEnv("S3_SECRET_KEY", ""),
			S3BucketName:     getEnv("S3_BUCKET", ""),
			S3UseSSL:         getEnv("S3_USE_SSL", "true") == "true",
			LocalPath:        getEnv("LOCAL_STORAGE_PATH", "./uploads"),
		},
		Google: GoogleConfig{
			ClientID: getEnv("GOOGLE_CLIENT_ID", ""),
		},
	}
}

// getEnv reads an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseDuration parses a duration string or returns a default value
func parseDuration(value string, defaultValue time.Duration) time.Duration {
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	return defaultValue
}
