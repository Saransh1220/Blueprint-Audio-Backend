package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getEnvOrDefault retrieves an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func TestPgSpecRepository_List_Genres(t *testing.T) {
	// Load config (assuming .env is in root)
	// We might need to manually set env vars if .env loading fails from subfolder
	// For simplicity, let's assume we can connect using default or env vars.
	// We'll skip if DB connection fails.

	cfg := config.Load() // You might need to adjust path logic in Load if it relies on CWD

	// Override with test-specific env vars or fallback to local defaults
	// Use TEST_DB_* env vars to avoid conflicts with regular DB_* variables
	if host := getEnvOrDefault("TEST_DB_HOST", "localhost"); host != "" {
		cfg.Database.Host = host
	}
	if port := getEnvOrDefault("TEST_DB_PORT", "5433"); port != "" {
		cfg.Database.Port = port
	}
	if user := getEnvOrDefault("TEST_DB_USER", "postgres"); user != "" {
		cfg.Database.User = user
	}
	if password := getEnvOrDefault("TEST_DB_PASSWORD", "postgres"); password != "" {
		cfg.Database.Password = password
	}
	if dbName := getEnvOrDefault("TEST_DB_NAME", "blueprint-audio"); dbName != "" {
		cfg.Database.DBName = dbName
	}
	cfg.Database.SSLMode = getEnvOrDefault("TEST_DB_SSLMODE", "disable")

	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		t.Skipf("Skipping integration test: failed to connect to DB: %v", err)
	}
	defer db.Close()

	repo := postgres.NewSpecRepository(db)
	ctx := context.Background()

	// 1. Create a Seed Spec with Genre
	genreName := "TestGenre_" + uuid.New().String()
	spec := &domain.Spec{
		ID:           uuid.New(),
		ProducerID:   uuid.New(), // We might need a real user if FK constraints exist? Yes, users table.
		ProducerName: "Test Producer",
		Title:        "Genre Test Spec",
		Category:     domain.CategoryBeat,
		BasePrice:    25.0,
		Description:  "Test Description",
		Duration:     120,
		ImageUrl:     "http://example.com/image.jpg",
		PreviewUrl:   "http://example.com/preview.mp3",
		BPM:          120,
		Key:          "C Minor",
		Genres: []domain.Genre{
			{ID: uuid.Nil, Name: genreName, Slug: genreName},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// We need a user to insert spec (FK)
	// Hack: Insert a dummy user directly using sqlx
	uniqueEmail := "test_" + uuid.New().String() + "@example.com"
	_, err = db.Exec("INSERT INTO users (id, email, password_hash, name, display_name, role, created_at, updated_at) VALUES ($1, $2, 'hash', 'Test Producer', 'Test Producer', 'producer', NOW(), NOW()) ON CONFLICT (id) DO NOTHING", spec.ProducerID, uniqueEmail)
	if err != nil {
		t.Logf("Failed to insert user: %v", err)
	}
	require.NoError(t, err)

	// Also need to ensure genres exist or let repo create them (repo logic creates if missing)
	// The repo.Create logic handles genre creation.

	err = repo.Create(ctx, spec)
	if err != nil {
		t.Logf("Failed to create spec: %v", err)
	}
	require.NoError(t, err, "Failed to create seed spec")

	// Clean up after
	defer func() {
		db.Exec("DELETE FROM specs WHERE id = $1", spec.ID)
		db.Exec("DELETE FROM users WHERE id = $1", spec.ProducerID)
		db.Exec("DELETE FROM genres WHERE slug = $1", genreName)
	}()

	// 2. Test List with Genre Filter
	filter := domain.SpecFilter{
		Genres: []string{genreName},
		Limit:  10,
	}

	specs, count, err := repo.List(ctx, filter)
	require.NoError(t, err, "List failed with genre filter")
	assert.GreaterOrEqual(t, count, 1)
	assert.Len(t, specs, 1)
	assert.Equal(t, spec.Title, specs[0].Title)
}
