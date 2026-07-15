package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func specRow(id, producerID uuid.UUID) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle"}).
		AddRow(id, producerID, "Track", "beat", "wav", 120, "C", 10.0, "image", "preview", 90, true, "Producer", "")
}

func TestPGSpecRepository_GetBySlugAndShortCode(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewSpecRepository(db)
	id, producerID := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, value string
		get         func(context.Context, string) (*domain.Spec, error)
	}{{"slug", "track", repo.GetBySlug}, {"short code", "ABC123", repo.GetByShortCode}} {
		t.Run(tc.name, func(t *testing.T) {
			mock.ExpectQuery("SELECT s.\\*, u.display_name").WithArgs(tc.value).WillReturnRows(specRow(id, producerID))
			mock.ExpectQuery("SELECT \\* FROM license_options").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
			mock.ExpectQuery("SELECT g.\\* FROM genres").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at"}))
			spec, err := tc.get(context.Background(), tc.value)
			require.NoError(t, err)
			assert.Equal(t, id, spec.ID)
		})
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPGSpecRepository_HomeQueries(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewSpecRepository(db)
	ctx := context.Background()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FILTER").WillReturnRows(sqlmock.NewRows([]string{"total_live_beats", "new_releases_7d", "total_producers"}).AddRow(10, 2, 3))
	stats, err := repo.GetHomepageStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, stats.TotalLiveBeats)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) AS count").WithArgs("trending", "24h").WillReturnRows(sqlmock.NewRows([]string{"count", "calculated_at"}).AddRow(2, time.Now()))
	fresh, err := repo.GetRankingFreshness(ctx, "trending", "24h")
	require.NoError(t, err)
	assert.Equal(t, 2, fresh.Count)
	mock.ExpectQuery("SELECT s.\\*, u.display_name as producer_name").WithArgs(8).WillReturnRows(sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle"}))
	newest, err := repo.GetNewestBeats(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, newest)
	mock.ExpectQuery("SELECT[\\s\\S]*FROM beat_rankings").WithArgs("trending", "24h", 8).WillReturnRows(sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle", "rank", "score", "previous_rank", "metrics", "calculated_at"}))
	ranked, err := repo.GetRankedSpecs(ctx, "trending", "24h", 0)
	require.NoError(t, err)
	assert.Empty(t, ranked)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPGSpecRepository_HomeQueriesWithRows(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewSpecRepository(db)
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()
	mock.ExpectQuery("SELECT s.\\*, u.display_name as producer_name").WithArgs(1).WillReturnRows(specRow(id, producerID))
	mock.ExpectQuery("SELECT sg.spec_id, g.\\* FROM genres").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id", "name", "slug", "created_at"}))
	mock.ExpectQuery("SELECT \\* FROM license_options").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
	newest, err := repo.GetNewestBeats(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, newest, 1)
	mock.ExpectQuery("SELECT[\\s\\S]*FROM beat_rankings").WithArgs("trending", "24h", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle", "rank", "score", "previous_rank", "metrics", "calculated_at"}).AddRow(id, producerID, "Track", "beat", "wav", 120, "C", 10.0, "image", "preview", 90, true, "Producer", "", 1, 9.5, 2, []byte(`{"plays": 3}`), time.Now()))
	mock.ExpectQuery("SELECT sg.spec_id, g.\\* FROM genres").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id", "name", "slug", "created_at"}))
	mock.ExpectQuery("SELECT \\* FROM license_options").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
	ranked, err := repo.GetRankedSpecs(ctx, "trending", "24h", 1)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 1, ranked[0].Rank)
	assert.Equal(t, 2, *ranked[0].PreviousRank)
	assert.NoError(t, mock.ExpectationsWereMet())
}
