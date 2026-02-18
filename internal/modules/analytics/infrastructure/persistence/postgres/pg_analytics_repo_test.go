package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	analyticsPostgres "github.com/saransh1220/blueprint-audio/internal/modules/analytics/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGAnalyticsRepository_GetSpecAnalyticsAndIsFavorited(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	specID := uuid.New()
	userID := uuid.New()

	mock.ExpectQuery("SELECT \\* FROM spec_analytics WHERE spec_id = \\$1").
		WithArgs(specID).WillReturnError(sql.ErrNoRows)
	rows := sqlmock.NewRows([]string{
		"spec_id", "play_count", "favorite_count", "free_download_count", "total_purchase_count", "created_at", "updated_at",
	}).AddRow(specID, 0, 0, 0, 0, time.Now(), time.Now())
	mock.ExpectQuery("INSERT INTO spec_analytics \\(spec_id\\)").
		WithArgs(specID).WillReturnRows(rows)

	out, err := repo.GetSpecAnalytics(ctx, specID)
	require.NoError(t, err)
	assert.Equal(t, specID, out.SpecID)

	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM user_favorites WHERE user_id = \\$1 AND spec_id = \\$2\\)").
		WithArgs(userID, specID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	fav, err := repo.IsFavorited(ctx, userID, specID)
	require.NoError(t, err)
	assert.True(t, fav)
}

func TestPGAnalyticsRepository_IncrementAndFavoriteTx(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	specID := uuid.New()
	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO spec_analytics \\(spec_id, play_count\\)").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO analytics_events \\(spec_id, event_type\\) VALUES \\(\\$1, 'play'\\)").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.IncrementPlayCount(ctx, specID))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO user_favorites \\(user_id, spec_id\\)").
		WithArgs(userID, specID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO spec_analytics \\(spec_id, favorite_count\\)").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO analytics_events \\(spec_id, event_type, user_id\\) VALUES \\(\\$1, 'favorite', \\$2\\)").
		WithArgs(specID, userID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.AddFavorite(ctx, userID, specID))

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM user_favorites WHERE user_id = \\$1 AND spec_id = \\$2").
		WithArgs(userID, specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE spec_analytics").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.RemoveFavorite(ctx, userID, specID))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO spec_analytics \\(spec_id, free_download_count\\)").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO analytics_events \\(spec_id, event_type\\) VALUES \\(\\$1, 'download'\\)").
		WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.IncrementFreeDownloadCount(ctx, specID))
}

func TestPGAnalyticsRepository_OverviewQueries(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	producerID := uuid.New()
	specID := uuid.New()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM analytics_events").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
	plays, err := repo.GetTotalPlays(ctx, producerID, 30)
	require.NoError(t, err)
	assert.Equal(t, 10, plays)

	mock.ExpectQuery("SELECT o\\.license_type, COALESCE\\(SUM\\(o\\.amount\\), 0\\) / 100\\.0 as revenue").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"license_type", "revenue"}).AddRow("Basic", 10.5))
	rev, err := repo.GetRevenueByLicenseGlobal(ctx, producerID, 30)
	require.NoError(t, err)
	assert.Equal(t, 10.5, rev["Basic"])

	mock.ExpectQuery("SELECT s\\.id as spec_id, s\\.title, COALESCE\\(sa\\.play_count, 0\\) as plays, COALESCE\\(sa\\.free_download_count, 0\\) as downloads, COALESCE\\(SUM\\(o\\.amount\\), 0\\) / 100\\.0 as revenue").
		WithArgs(producerID, 5).
		WillReturnRows(sqlmock.NewRows([]string{"spec_id", "title", "plays", "downloads", "revenue"}).AddRow(specID.String(), "Track", 9, 5, 20.0))
	top, err := repo.GetTopSpecs(ctx, producerID, 5, "plays")
	require.NoError(t, err)
	assert.Len(t, top, 1)
}

func TestPGAnalyticsRepository_AdditionalCoverage(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	producerID := uuid.New()
	specID := uuid.New()

	mock.ExpectQuery("SELECT\\s+o\\.license_type,\\s+COUNT\\(\\*\\) as count").
		WithArgs(specID).
		WillReturnRows(sqlmock.NewRows([]string{"license_type", "count"}).AddRow("Basic", 2))
	counts, err := repo.GetLicensePurchaseCounts(ctx, specID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts["Basic"])

	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM analytics_events").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	v, err := repo.GetTotalFavorites(ctx, producerID, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, v)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM analytics_events").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
	v, err = repo.GetTotalDownloads(ctx, producerID, 0)
	require.NoError(t, err)
	assert.Equal(t, 4, v)

	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(amount\\), 0\\) / 100\\.0").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(12.5))
	rev, err := repo.GetTotalRevenue(ctx, producerID, 0)
	require.NoError(t, err)
	assert.Equal(t, 12.5, rev)

	mock.ExpectQuery("SELECT\\s+to_char\\(date_trunc\\('day', ae\\.created_at\\), 'YYYY-MM-DD'\\) as date,\\s+COUNT\\(\\*\\) as count").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"date", "count"}).AddRow("2026-02-15", 7))
	plays, err := repo.GetPlaysByDay(ctx, producerID, 0)
	require.NoError(t, err)
	require.Len(t, plays, 1)
	assert.Equal(t, 7, plays[0].Count)

	mock.ExpectQuery("SELECT\\s+to_char\\(date_trunc\\('day', ae\\.created_at\\), 'YYYY-MM-DD'\\) as date,\\s+COUNT\\(\\*\\) as count").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"date", "count"}).AddRow("2026-02-15", 5))
	downloads, err := repo.GetDownloadsByDay(ctx, producerID, 0)
	require.NoError(t, err)
	require.Len(t, downloads, 1)
	assert.Equal(t, 5, downloads[0].Count)

	mock.ExpectQuery("SELECT\\s+to_char\\(date_trunc\\('day', o\\.created_at\\), 'YYYY-MM-DD'\\) as date,\\s+COALESCE\\(SUM\\(o\\.amount\\), 0\\) / 100\\.0 as revenue").
		WithArgs(producerID, 30).
		WillReturnRows(sqlmock.NewRows([]string{"date", "revenue"}).AddRow("2026-02-15", 33.3))
	revenueByDay, err := repo.GetRevenueByDay(ctx, producerID, 0)
	require.NoError(t, err)
	require.Len(t, revenueByDay, 1)
	assert.Equal(t, 33.3, revenueByDay[0].Revenue)

	mock.ExpectQuery("SELECT\\s+s\\.id as spec_id,\\s+s\\.title").
		WithArgs(producerID, 2).
		WillReturnRows(sqlmock.NewRows([]string{"spec_id", "title", "plays", "downloads", "revenue"}).AddRow(specID.String(), "Track", 2, 1, 9.9))
	top, err := repo.GetTopSpecs(ctx, producerID, 2, "downloads")
	require.NoError(t, err)
	assert.Len(t, top, 1)
}
