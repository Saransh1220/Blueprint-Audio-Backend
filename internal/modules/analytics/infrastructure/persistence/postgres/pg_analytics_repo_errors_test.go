package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	analyticsPostgres "github.com/saransh1220/blueprint-audio/internal/modules/analytics/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDB = errors.New("db failure")

func TestPGAnalyticsRepository_GetSpecAnalytics_Errors(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	specID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM spec_analytics`).WithArgs(specID).WillReturnError(errDB)
	_, err := repo.GetSpecAnalytics(ctx, specID)
	assert.ErrorIs(t, err, errDB)

	mock.ExpectQuery(`SELECT \* FROM spec_analytics`).WithArgs(specID).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO spec_analytics`).WithArgs(specID).WillReturnError(errDB)
	_, err = repo.GetSpecAnalytics(ctx, specID)
	assert.ErrorIs(t, err, errDB)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPGAnalyticsRepository_IsFavorited_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(errDB)
	ok, err := repo.IsFavorited(context.Background(), uuid.New(), uuid.New())
	assert.False(t, ok)
	assert.ErrorIs(t, err, errDB)
}

func TestPGAnalyticsRepository_Increment_Errors(t *testing.T) {
	ctx := context.Background()
	specID := uuid.New()

	type fn func(repo interface {
		IncrementPlayCount(context.Context, uuid.UUID) error
		IncrementFreeDownloadCount(context.Context, uuid.UUID) error
	}) error
	calls := map[string]fn{
		"play": func(r interface {
			IncrementPlayCount(context.Context, uuid.UUID) error
			IncrementFreeDownloadCount(context.Context, uuid.UUID) error
		}) error {
			return r.IncrementPlayCount(ctx, specID)
		},
		"download": func(r interface {
			IncrementPlayCount(context.Context, uuid.UUID) error
			IncrementFreeDownloadCount(context.Context, uuid.UUID) error
		}) error {
			return r.IncrementFreeDownloadCount(ctx, specID)
		},
	}

	for name, call := range calls {
		t.Run(name+" begin error", func(t *testing.T) {
			db, mock, cleanup := newMockDB(t)
			defer cleanup()
			mock.ExpectBegin().WillReturnError(errDB)
			err := call(analyticsPostgres.NewAnalyticsRepository(db))
			assert.ErrorIs(t, err, errDB)
		})
		t.Run(name+" totals error", func(t *testing.T) {
			db, mock, cleanup := newMockDB(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnError(errDB)
			mock.ExpectRollback()
			err := call(analyticsPostgres.NewAnalyticsRepository(db))
			assert.ErrorIs(t, err, errDB)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+" event error", func(t *testing.T) {
			db, mock, cleanup := newMockDB(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`INSERT INTO analytics_events`).WillReturnError(errDB)
			mock.ExpectRollback()
			err := call(analyticsPostgres.NewAnalyticsRepository(db))
			assert.ErrorIs(t, err, errDB)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+" commit error", func(t *testing.T) {
			db, mock, cleanup := newMockDB(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`INSERT INTO analytics_events`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit().WillReturnError(errDB)
			err := call(analyticsPostgres.NewAnalyticsRepository(db))
			assert.ErrorIs(t, err, errDB)
		})
	}
}

func TestPGAnalyticsRepository_AddFavorite_Branches(t *testing.T) {
	ctx := context.Background()
	userID, specID := uuid.New(), uuid.New()

	t.Run("begin error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errDB)
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID), errDB)
	})
	t.Run("insert error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO user_favorites`).WillReturnError(errDB)
		mock.ExpectRollback()
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID), errDB)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("already favorited is a no-op", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO user_favorites`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		assert.NoError(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("count error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO user_favorites`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnError(errDB)
		mock.ExpectRollback()
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID), errDB)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("event error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO user_favorites`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO analytics_events`).WillReturnError(errDB)
		mock.ExpectRollback()
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID), errDB)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("commit error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO user_favorites`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO analytics_events`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errDB)
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).AddFavorite(ctx, userID, specID), errDB)
	})
}

func TestPGAnalyticsRepository_RemoveFavorite_Branches(t *testing.T) {
	ctx := context.Background()
	userID, specID := uuid.New(), uuid.New()

	t.Run("begin error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errDB)
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).RemoveFavorite(ctx, userID, specID), errDB)
	})
	t.Run("delete error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM user_favorites`).WillReturnError(errDB)
		mock.ExpectRollback()
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).RemoveFavorite(ctx, userID, specID), errDB)
	})
	t.Run("not favorited is a no-op", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM user_favorites`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		assert.NoError(t, analyticsPostgres.NewAnalyticsRepository(db).RemoveFavorite(ctx, userID, specID))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("decrement error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM user_favorites`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE spec_analytics`).WillReturnError(errDB)
		mock.ExpectRollback()
		assert.ErrorIs(t, analyticsPostgres.NewAnalyticsRepository(db).RemoveFavorite(ctx, userID, specID), errDB)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPGAnalyticsRepository_QueryErrors(t *testing.T) {
	ctx := context.Background()
	pid := uuid.New()

	calls := map[string]func(r domain.AnalyticsRepository) error{
		"license counts": func(r domain.AnalyticsRepository) error { _, err := r.GetLicensePurchaseCounts(ctx, pid); return err },
		"total plays":    func(r domain.AnalyticsRepository) error { _, err := r.GetTotalPlays(ctx, pid, 7); return err },
		"total favs":     func(r domain.AnalyticsRepository) error { _, err := r.GetTotalFavorites(ctx, pid, 7); return err },
		"total dl":       func(r domain.AnalyticsRepository) error { _, err := r.GetTotalDownloads(ctx, pid, 7); return err },
		"total revenue":  func(r domain.AnalyticsRepository) error { _, err := r.GetTotalRevenue(ctx, pid, 7); return err },
		"rev by license": func(r domain.AnalyticsRepository) error {
			_, err := r.GetRevenueByLicenseGlobal(ctx, pid, 7)
			return err
		},
		"plays by day": func(r domain.AnalyticsRepository) error { _, err := r.GetPlaysByDay(ctx, pid, 7); return err },
		"dl by day":    func(r domain.AnalyticsRepository) error { _, err := r.GetDownloadsByDay(ctx, pid, 7); return err },
		"rev by day":   func(r domain.AnalyticsRepository) error { _, err := r.GetRevenueByDay(ctx, pid, 7); return err },
		"top specs":    func(r domain.AnalyticsRepository) error { _, err := r.GetTopSpecs(ctx, pid, 5, "plays"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			db, mock, cleanup := newMockDB(t)
			defer cleanup()
			mock.ExpectQuery(`.`).WillReturnError(errDB)
			assert.ErrorIs(t, call(analyticsPostgres.NewAnalyticsRepository(db)), errDB)
		})
	}
}

func TestPGAnalyticsRepository_DaysDefaultsAndRevenueByLicense(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	pid := uuid.New()

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM analytics_events`).WithArgs(pid, 30).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))
	n, err := repo.GetTotalPlays(ctx, pid, -1)
	require.NoError(t, err)
	assert.Equal(t, 9, n)

	mock.ExpectQuery(`o\.license_type, COALESCE`).WithArgs(pid, 30).
		WillReturnRows(sqlmock.NewRows([]string{"license_type", "revenue"}).AddRow("basic", 10.5).AddRow("premium", 40.0))
	m, err := repo.GetRevenueByLicenseGlobal(ctx, pid, 0)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"basic": 10.5, "premium": 40.0}, m)

	for _, sortBy := range []string{"revenue", "plays", ""} {
		mock.ExpectQuery(`FROM specs s`).WithArgs(pid, 3).
			WillReturnRows(sqlmock.NewRows([]string{"spec_id", "title", "plays", "downloads", "revenue"}))
		_, err = repo.GetTopSpecs(ctx, pid, 3, sortBy)
		require.NoError(t, err, sortBy)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func favRows(favAt time.Time, ids ...uuid.UUID) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"favorited_at", "id", "producer_id", "title", "price_currency", "producer_name"})
	for _, id := range ids {
		rows.AddRow(favAt, id, uuid.New(), "Beat "+id.String()[:4], "USD", "Producer")
	}
	return rows
}

func TestPGAnalyticsRepository_ListUserFavorites_SuccessAndPagination(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := analyticsPostgres.NewAnalyticsRepository(db)
	ctx := context.Background()
	userID := uuid.New()
	favAt := time.Now().UTC().Truncate(time.Second)
	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()

	// limit=2 -> LIMIT 3; 3 rows returned means hasMore
	mock.ExpectQuery(`FROM user_favorites uf`).WithArgs(userID, 3).WillReturnRows(favRows(favAt, id1, id2, id3))
	for _, id := range []uuid.UUID{id1, id2} {
		mock.ExpectQuery(`FROM license_options`).WithArgs(id).
			WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "name", "price"}).AddRow(uuid.New(), id, "Basic", 10.0))
		mock.ExpectQuery(`FROM genres g`).WithArgs(id).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug"}).AddRow(uuid.New(), "Trap", "trap"))
	}

	page, err := repo.ListUserFavorites(ctx, userID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.True(t, page.HasMore)
	require.NotNil(t, page.NextCursor)
	assert.Equal(t, id2, page.NextCursor.SpecID)
	assert.True(t, favAt.Equal(page.NextCursor.FavoritedAt))
	assert.Equal(t, id1, page.Items[0].Spec.ID)
	assert.Equal(t, "Producer", page.Items[0].Spec.ProducerName)
	require.Len(t, page.Items[0].Spec.Licenses, 1)
	assert.Equal(t, "Basic", page.Items[0].Spec.Licenses[0].Name)
	require.Len(t, page.Items[0].Spec.Genres, 1)
	assert.Equal(t, "trap", page.Items[0].Spec.Genres[0].Slug)

	// With cursor: extra args, fewer rows than limit -> no more
	cursor := &domain.FavoriteCursor{FavoritedAt: favAt, SpecID: id2}
	mock.ExpectQuery(`uf\.spec_id\) < \(\$2, \$3\)`).WithArgs(userID, favAt, id2, 21).WillReturnRows(favRows(favAt, id3))
	mock.ExpectQuery(`FROM license_options`).WithArgs(id3).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`FROM genres g`).WithArgs(id3).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	page, err = repo.ListUserFavorites(ctx, userID, 0, cursor) // 0 -> default 20
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.False(t, page.HasMore)
	assert.Nil(t, page.NextCursor)

	// Over-max limit also falls back to default 20
	mock.ExpectQuery(`FROM user_favorites uf`).WithArgs(userID, 21).WillReturnRows(favRows(favAt))
	page, err = repo.ListUserFavorites(ctx, userID, 1000, nil)
	require.NoError(t, err)
	assert.Empty(t, page.Items)
	assert.NotNil(t, page.Items)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPGAnalyticsRepository_ListUserFavorites_Errors(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	favAt := time.Now()
	id := uuid.New()

	t.Run("query error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectQuery(`FROM user_favorites uf`).WillReturnError(errDB)
		_, err := analyticsPostgres.NewAnalyticsRepository(db).ListUserFavorites(ctx, userID, 5, nil)
		assert.ErrorIs(t, err, errDB)
		assert.Contains(t, err.Error(), "ListUserFavorites query")
	})
	t.Run("scan error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		rows := sqlmock.NewRows([]string{"favorited_at", "id"}).AddRow("not-a-time", "not-a-uuid")
		mock.ExpectQuery(`FROM user_favorites uf`).WillReturnRows(rows)
		_, err := analyticsPostgres.NewAnalyticsRepository(db).ListUserFavorites(ctx, userID, 5, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ListUserFavorites scan")
	})
	t.Run("rows error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		rows := favRows(favAt, id).RowError(0, errDB)
		mock.ExpectQuery(`FROM user_favorites uf`).WillReturnRows(rows)
		_, err := analyticsPostgres.NewAnalyticsRepository(db).ListUserFavorites(ctx, userID, 5, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ListUserFavorites rows")
	})
	t.Run("licenses error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectQuery(`FROM user_favorites uf`).WillReturnRows(favRows(favAt, id))
		mock.ExpectQuery(`FROM license_options`).WillReturnError(errDB)
		_, err := analyticsPostgres.NewAnalyticsRepository(db).ListUserFavorites(ctx, userID, 5, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ListUserFavorites licenses")
	})
	t.Run("genres error", func(t *testing.T) {
		db, mock, cleanup := newMockDB(t)
		defer cleanup()
		mock.ExpectQuery(`FROM user_favorites uf`).WillReturnRows(favRows(favAt, id))
		mock.ExpectQuery(`FROM license_options`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery(`FROM genres g`).WillReturnError(errDB)
		_, err := analyticsPostgres.NewAnalyticsRepository(db).ListUserFavorites(ctx, userID, 5, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ListUserFavorites genres")
	})
}
