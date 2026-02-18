package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
	"github.com/stretchr/testify/require"
)

func TestPgSpecRepository_List_PartialSearch(t *testing.T) {
	cfg := config.Load()

	// Minimal DB connection setup (reusing logic from spec_repo_test.go)
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

	// 1. Create Seed Data
	producerID := uuid.New()
	specID := uuid.New()

	// Create Producer User
	_, err = db.Exec("INSERT INTO users (id, email, password_hash, name, display_name, role, created_at, updated_at) VALUES ($1, $2, 'hash', 'Search Producer', 'Search Producer', 'producer', NOW(), NOW()) ON CONFLICT (id) DO NOTHING", producerID, "search_"+uuid.New().String()+"@example.com")
	require.NoError(t, err)

	spec := &domain.Spec{
		ID:          specID,
		ProducerID:  producerID,
		Title:       "Atmospheric Trap Beat",
		Category:    domain.CategoryBeat,
		BasePrice:   25.0,
		Description: "Dark moody vibes",
		Duration:    120,
		ImageUrl:    "http://example.com/image.jpg",
		PreviewUrl:  "http://example.com/preview.mp3",
		BPM:         140,
		Key:         "C Minor",
		Tags:        []string{"trap", "dark", "moody"},
		Genres:      []domain.Genre{},
		Licenses:    []domain.LicenseOption{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = repo.Create(ctx, spec)
	require.NoError(t, err)

	defer func() {
		db.Exec("DELETE FROM specs WHERE id = $1", specID)
		db.Exec("DELETE FROM users WHERE id = $1", producerID)
	}()

	// 2. Test Cases
	tests := []struct {
		name        string
		query       string
		shouldFound bool
	}{
		{
			name:        "Exact Title Match - case insensitive",
			query:       "atmospheric",
			shouldFound: true,
		},
		{
			name:        "Partial Title Match",
			query:       "atmos",
			shouldFound: true,
		},
		{
			name:        "Exact Tag Match",
			query:       "trap",
			shouldFound: true,
		},
		{
			name:        "Partial Tag Match (Expected Failure currently)",
			query:       "tra",
			shouldFound: true,
		},
		{
			name:        "Partial Tag Match 2",
			query:       "moo", // matches "moody"
			shouldFound: true,
		},
		{
			name:        "No Match",
			query:       "xyz123",
			shouldFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filter := domain.SpecFilter{
				Search: tc.query,
				Limit:  10,
			}
			specs, _, err := repo.List(ctx, filter)
			require.NoError(t, err)

			if tc.shouldFound {
				if len(specs) == 0 {
					t.Errorf("Expected to find spec with query '%s', but found none", tc.query)
				}
			} else {
				if len(specs) > 0 {
					// We need to check if the found spec is OUR spec, because DB might have other data
					for _, s := range specs {
						if s.ID == specID {
							t.Errorf("Expected NOT to find our spec with query '%s', but found it", tc.query)
						}
					}
				}
			}
		})
	}
}
