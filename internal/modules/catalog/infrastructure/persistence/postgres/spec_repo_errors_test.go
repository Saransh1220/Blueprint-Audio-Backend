package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSpecBoom = errors.New("spec boom")

func newSpecRepoForTest(t *testing.T) (*postgres.PgSpecRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, cleanup := newMockDB(t)
	return postgres.NewSpecRepository(db), mock, cleanup
}

func TestPGSpecRepository_Create_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	producerID := uuid.New()
	newSpec := func() *domain.Spec {
		return &domain.Spec{
			ProducerID: producerID,
			Title:      "t",
			Genres:     []domain.Genre{{Name: "Trap", Slug: "trap"}},
			Licenses:   []domain.LicenseOption{{LicenseType: "basic", Name: "Basic"}},
		}
	}

	t.Run("success assigns ids and defaults", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		genreID := uuid.New()
		spec := newSpec()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`INSERT INTO genres`).WithArgs(sqlmock.AnyArg(), "Trap", "trap", sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(genreID))
		mock.ExpectExec(`INSERT INTO spec_genres`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO license_options`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.Create(ctx, spec))
		assert.NotEqual(t, uuid.Nil, spec.ID)
		assert.Equal(t, domain.ProcessingStatusPending, spec.ProcessingStatus)
		assert.Equal(t, spec.ID, spec.Licenses[0].SpecID)
		assert.NotEqual(t, uuid.Nil, spec.Licenses[0].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Create(ctx, newSpec()), errSpecBoom)
	})

	steps := []struct {
		name  string
		setup func(m sqlmock.Sqlmock)
		spec  func() *domain.Spec
		match string
	}{
		{"spec insert error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnError(errSpecBoom)
		}, newSpec, ""},
		{"analytics insert error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnError(errSpecBoom)
		}, newSpec, ""},
		{"genre upsert error is wrapped with genre name", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(`INSERT INTO genres`).WillReturnError(errSpecBoom)
		}, newSpec, "failed to get or create genre Trap"},
		{"spec_genres insert error with existing genre id", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_genres`).WillReturnError(errSpecBoom)
		}, func() *domain.Spec {
			s := newSpec()
			s.Genres = []domain.Genre{{ID: uuid.New(), Name: "Trap"}}
			return s
		}, ""},
		{"license insert error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO license_options`).WillReturnError(errSpecBoom)
		}, func() *domain.Spec {
			s := newSpec()
			s.Genres = nil
			return s
		}, ""},
		{"commit error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit().WillReturnError(errSpecBoom)
		}, func() *domain.Spec { return &domain.Spec{ID: uuid.New(), CreatedAt: time.Now()} }, ""},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			tc.setup(mock)
			err := repo.Create(ctx, tc.spec())
			require.ErrorIs(t, err, errSpecBoom)
			if tc.match != "" {
				assert.Contains(t, err.Error(), tc.match)
			}
		})
	}
}

func TestPGSpecRepository_GetVariants_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()

	getters := []struct {
		name string
		call func(r *postgres.PgSpecRepository) (*domain.Spec, error)
		arg  interface{}
	}{
		{"GetByID", func(r *postgres.PgSpecRepository) (*domain.Spec, error) { return r.GetByID(ctx, id) }, id},
		{"GetBySlug", func(r *postgres.PgSpecRepository) (*domain.Spec, error) { return r.GetBySlug(ctx, "slug") }, "slug"},
		{"GetByShortCode", func(r *postgres.PgSpecRepository) (*domain.Spec, error) { return r.GetByShortCode(ctx, "ABC") }, "ABC"},
		{"GetByIDSystem", func(r *postgres.PgSpecRepository) (*domain.Spec, error) { return r.GetByIDSystem(ctx, id) }, id},
		{"FindByIDIncludingDeleted", func(r *postgres.PgSpecRepository) (*domain.Spec, error) {
			return r.FindByIDIncludingDeleted(ctx, id)
		}, id},
	}
	for _, g := range getters {
		t.Run(g.name+"/success", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectQuery(`SELECT s\.\*, u\.display_name`).WithArgs(g.arg).WillReturnRows(specRow(id, producerID))
			mock.ExpectQuery(`FROM license_options`).WithArgs(id).WillReturnRows(
				sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name"}).AddRow(uuid.New(), id, "basic", "Basic"))
			mock.ExpectQuery(`FROM genres g JOIN spec_genres`).WithArgs(id).WillReturnRows(
				sqlmock.NewRows([]string{"id", "name", "slug"}).AddRow(uuid.New(), "Trap", "trap"))
			spec, err := g.call(repo)
			require.NoError(t, err)
			assert.Equal(t, id, spec.ID)
			require.Len(t, spec.Licenses, 1)
			require.Len(t, spec.Genres, 1)
			assert.Equal(t, "trap", spec.Genres[0].Slug)
			require.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(g.name+"/spec query error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectQuery(`SELECT s\.\*, u\.display_name`).WillReturnError(errSpecBoom)
			_, err := g.call(repo)
			require.ErrorIs(t, err, errSpecBoom)
		})
		t.Run(g.name+"/license query error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectQuery(`SELECT s\.\*, u\.display_name`).WillReturnRows(specRow(id, producerID))
			mock.ExpectQuery(`FROM license_options`).WillReturnError(errSpecBoom)
			_, err := g.call(repo)
			require.ErrorIs(t, err, errSpecBoom)
		})
		t.Run(g.name+"/genre query error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectQuery(`SELECT s\.\*, u\.display_name`).WillReturnRows(specRow(id, producerID))
			mock.ExpectQuery(`FROM license_options`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(`FROM genres g JOIN spec_genres`).WillReturnError(errSpecBoom)
			_, err := g.call(repo)
			require.ErrorIs(t, err, errSpecBoom)
		})
	}
}

func listSpecRow(id, producerID uuid.UUID, total int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle", "total_count"}).
		AddRow(id, producerID, "Track", "beat", "wav", 120, "C", 10.0, "image", "preview", 90, true, "Producer", "", total)
}

func TestPGSpecRepository_List_SortsFiltersAndErrors(t *testing.T) {
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()

	sorts := map[string]string{
		"":           "ORDER BY s.created_at DESC",
		"newest":     "ORDER BY s.created_at DESC",
		"oldest":     "ORDER BY s.created_at ASC",
		"price_asc":  "ORDER BY s.base_price ASC",
		"price_desc": "ORDER BY s.base_price DESC",
		"bpm_asc":    "ORDER BY s.bpm ASC",
		"bpm_desc":   "ORDER BY s.bpm DESC",
	}
	for sortKey, clause := range sorts {
		t.Run("sort "+sortKey, func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectQuery(clause+` LIMIT \$2 OFFSET \$3`).WithArgs(float64(0), 5, 10).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			specs, total, err := repo.List(ctx, domain.SpecFilter{Sort: sortKey, Limit: 5, Offset: 10})
			require.NoError(t, err)
			assert.Empty(t, specs)
			assert.Zero(t, total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("all filters add clauses and arguments in order", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`s\.category = \$1[\s\S]*ILIKE ANY\(\$2::text\[\]\)[\s\S]*s\.tags @> \$3[\s\S]*s\.title ILIKE \$4[\s\S]*s\.bpm >= \$5[\s\S]*s\.bpm <= \$6[\s\S]*s\.base_price >= \$7[\s\S]*s\.base_price <= \$8[\s\S]*s\.key = \$9[\s\S]*s\.moods && \$10[\s\S]*s\.instruments && \$11[\s\S]*s\.duration >= \$12[\s\S]*s\.duration <= \$13[\s\S]*LIMIT \$14 OFFSET \$15`).
			WithArgs("beat", sqlmock.AnyArg(), sqlmock.AnyArg(), "%dark%", 80, 160, 5.0, 50.0, "C",
				sqlmock.AnyArg(), sqlmock.AnyArg(), 30, 200, 20, 0).
			WillReturnRows(listSpecRow(id, producerID, 7))
		mock.ExpectQuery(`FROM genres g\s+JOIN spec_genres`).WillReturnRows(
			sqlmock.NewRows([]string{"spec_id", "id", "name", "slug"}).AddRow(id, uuid.New(), "Trap", "trap"))
		mock.ExpectQuery(`FROM license_options WHERE spec_id IN`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name"}).
				AddRow(uuid.New(), id, "basic", "Basic").
				AddRow(uuid.New(), uuid.New(), "basic", "Orphan"))

		specs, total, err := repo.List(ctx, domain.SpecFilter{
			Category: "beat", Genres: []string{"trap"}, Tags: []string{"dark"}, Search: "dark",
			MinBPM: 80, MaxBPM: 160, MinPrice: 5, MaxPrice: 50, Key: "C",
			Moods: []string{"dark"}, Instruments: []string{"808"},
			MinDuration: 30, MaxDuration: 200, Limit: 20,
		})
		require.NoError(t, err)
		assert.Equal(t, 7, total)
		require.Len(t, specs, 1)
		require.Len(t, specs[0].Genres, 1)
		require.Len(t, specs[0].Licenses, 1, "orphan license must not be attached")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("main query error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT s\.\*`).WillReturnError(errSpecBoom)
		_, _, err := repo.List(ctx, domain.SpecFilter{})
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("genre fetch error is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT s\.\*`).WillReturnRows(listSpecRow(id, producerID, 1))
		mock.ExpectQuery(`FROM genres g`).WillReturnError(errSpecBoom)
		_, _, err := repo.List(ctx, domain.SpecFilter{})
		require.ErrorIs(t, err, errSpecBoom)
		assert.Contains(t, err.Error(), "failed to fetch genres")
	})
	t.Run("license fetch error is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT s\.\*`).WillReturnRows(listSpecRow(id, producerID, 1))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id"}))
		mock.ExpectQuery(`FROM license_options`).WillReturnError(errSpecBoom)
		_, _, err := repo.List(ctx, domain.SpecFilter{})
		require.ErrorIs(t, err, errSpecBoom)
		assert.Contains(t, err.Error(), "failed to fetch licenses")
	})
}

func TestPGSpecRepository_ListByUserID_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()

	t.Run("success attaches relations", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`WHERE s\.producer_id = \$1`).WithArgs(producerID, 10, 0).
			WillReturnRows(listSpecRow(id, producerID, 3))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(
			sqlmock.NewRows([]string{"spec_id", "id", "name", "slug"}).AddRow(id, uuid.New(), "Trap", "trap"))
		mock.ExpectQuery(`FROM license_options`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name"}).AddRow(uuid.New(), id, "basic", "Basic"))
		specs, total, err := repo.ListByUserID(ctx, producerID, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		require.Len(t, specs, 1)
		assert.Len(t, specs[0].Genres, 1)
		assert.Len(t, specs[0].Licenses, 1)
	})
	t.Run("empty", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`WHERE s\.producer_id = \$1`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		specs, total, err := repo.ListByUserID(ctx, producerID, 10, 0)
		require.NoError(t, err)
		assert.Empty(t, specs)
		assert.Zero(t, total)
	})
	t.Run("query error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`WHERE s\.producer_id = \$1`).WillReturnError(errSpecBoom)
		_, _, err := repo.ListByUserID(ctx, producerID, 10, 0)
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("genre error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`WHERE s\.producer_id = \$1`).WillReturnRows(listSpecRow(id, producerID, 1))
		mock.ExpectQuery(`FROM genres g`).WillReturnError(errSpecBoom)
		_, _, err := repo.ListByUserID(ctx, producerID, 10, 0)
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("license error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`WHERE s\.producer_id = \$1`).WillReturnRows(listSpecRow(id, producerID, 1))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id"}))
		mock.ExpectQuery(`FROM license_options`).WillReturnError(errSpecBoom)
		_, _, err := repo.ListByUserID(ctx, producerID, 10, 0)
		require.ErrorIs(t, err, errSpecBoom)
	})
}

func TestPGSpecRepository_HomeQueries_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()

	t.Run("homepage stats error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`COUNT\(\*\) FILTER`).WillReturnError(errSpecBoom)
		stats, err := repo.GetHomepageStats(ctx)
		require.ErrorIs(t, err, errSpecBoom)
		assert.Nil(t, stats)
	})
	t.Run("ranking freshness error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`FROM beat_rankings`).WillReturnError(errSpecBoom)
		_, err := repo.GetRankingFreshness(ctx, "trending", "24h")
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("ranking freshness with null timestamp leaves zero time", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`FROM beat_rankings`).WillReturnRows(
			sqlmock.NewRows([]string{"count", "calculated_at"}).AddRow(0, nil))
		f, err := repo.GetRankingFreshness(ctx, "top_charts", "7d")
		require.NoError(t, err)
		assert.Zero(t, f.Count)
		assert.True(t, f.CalculatedAt.IsZero())
	})
	t.Run("newest beats query error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`s\.category = 'beat'`).WithArgs(3).WillReturnError(errSpecBoom)
		_, err := repo.GetNewestBeats(ctx, 3)
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("newest beats hydration genre error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`s\.category = 'beat'`).WillReturnRows(specRow(id, producerID))
		mock.ExpectQuery(`FROM genres g`).WillReturnError(errSpecBoom)
		_, err := repo.GetNewestBeats(ctx, 3)
		require.ErrorIs(t, err, errSpecBoom)
		assert.Contains(t, err.Error(), "failed to fetch genres")
	})
	t.Run("newest beats hydration license error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`s\.category = 'beat'`).WillReturnRows(specRow(id, producerID))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id"}))
		mock.ExpectQuery(`FROM license_options`).WillReturnError(errSpecBoom)
		_, err := repo.GetNewestBeats(ctx, 3)
		require.ErrorIs(t, err, errSpecBoom)
		assert.Contains(t, err.Error(), "failed to fetch licenses")
	})
	t.Run("newest beats hydrates genres and licenses", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`s\.category = 'beat'`).WillReturnRows(specRow(id, producerID))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(
			sqlmock.NewRows([]string{"spec_id", "id", "name", "slug"}).
				AddRow(id, uuid.New(), "Trap", "trap").
				AddRow(uuid.New(), uuid.New(), "Other", "other"))
		mock.ExpectQuery(`FROM license_options`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name"}).AddRow(uuid.New(), id, "basic", "Basic"))
		beats, err := repo.GetNewestBeats(ctx, 3)
		require.NoError(t, err)
		require.Len(t, beats, 1)
		assert.Len(t, beats[0].Genres, 1)
		assert.Len(t, beats[0].Licenses, 1)
	})

	rankedCols := []string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "producer_name", "producer_handle", "rank", "score", "previous_rank", "metrics", "calculated_at"}
	t.Run("ranked specs map rank, previous rank and metrics", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		calc := time.Now()
		other := uuid.New()
		mock.ExpectQuery(`FROM beat_rankings br`).WithArgs("top_charts", "7d", 2).WillReturnRows(
			sqlmock.NewRows(rankedCols).
				AddRow(id, producerID, "A", "beat", "wav", 120, "C", 10.0, "i", "p", 90, true, "P", "", 1, 9.5, 4, []byte(`{"plays":3}`), calc).
				AddRow(other, producerID, "B", "beat", "wav", 120, "C", 10.0, "i", "p", 90, true, "P", "", 2, 7.0, nil, []byte(`{}`), calc))
		mock.ExpectQuery(`FROM genres g`).WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id"}))
		mock.ExpectQuery(`FROM license_options`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		rows, err := repo.GetRankedSpecs(ctx, "top_charts", "7d", 2)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.NotNil(t, rows[0].PreviousRank)
		assert.Equal(t, 4, *rows[0].PreviousRank)
		assert.Nil(t, rows[1].PreviousRank)
		assert.Equal(t, 1, rows[0].Rank)
		assert.InDelta(t, 9.5, rows[0].Score, 0.001)
		assert.JSONEq(t, `{"plays":3}`, string(rows[0].MetricsJSON))
	})
	t.Run("ranked specs query error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`FROM beat_rankings br`).WillReturnError(errSpecBoom)
		_, err := repo.GetRankedSpecs(ctx, "trending", "24h", 1)
		require.ErrorIs(t, err, errSpecBoom)
	})
	t.Run("ranked specs hydration error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(`FROM beat_rankings br`).WillReturnRows(
			sqlmock.NewRows(rankedCols).AddRow(id, producerID, "A", "beat", "wav", 120, "C", 10.0, "i", "p", 90, true, "P", "", 1, 9.5, nil, []byte(`{}`), time.Now()))
		mock.ExpectQuery(`FROM genres g`).WillReturnError(errSpecBoom)
		_, err := repo.GetRankedSpecs(ctx, "trending", "24h", 1)
		require.ErrorIs(t, err, errSpecBoom)
	})
}

func TestPGSpecRepository_RecalculateBeatRankings(t *testing.T) {
	ctx := context.Background()
	sections := []struct {
		section  string
		period   string
		interval string
		lockKey  string
		sqlMatch string
	}{
		{domain.HomeSectionTrending, domain.HomePeriod24H, "24 hours", "beat_rankings:trending:24h", `WITH current_metrics AS`},
		{domain.HomeSectionTrending, domain.HomePeriod7D, "7 days", "beat_rankings:trending:7d", `WITH current_metrics AS`},
		{domain.HomeSectionTopCharts, domain.HomePeriod30D, "30 days", "beat_rankings:top_charts:30d", `WITH event_metrics AS`},
	}
	lockQ := `SELECT pg_try_advisory_xact_lock`
	for _, s := range sections {
		name := s.section + "/" + s.period
		t.Run(name+"/success", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WithArgs(s.lockKey).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectExec(s.sqlMatch).WithArgs(s.interval, s.period).WillReturnResult(sqlmock.NewResult(0, 5))
			mock.ExpectCommit()
			require.NoError(t, repo.RecalculateBeatRankings(ctx, s.section, s.period))
			require.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+"/begin error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin().WillReturnError(errSpecBoom)
			require.ErrorIs(t, repo.RecalculateBeatRankings(ctx, s.section, s.period), errSpecBoom)
		})
		t.Run(name+"/lock error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnError(errSpecBoom)
			mock.ExpectRollback()
			require.ErrorIs(t, repo.RecalculateBeatRankings(ctx, s.section, s.period), errSpecBoom)
			require.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+"/lock busy skips refresh", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
			mock.ExpectRollback()
			err := repo.RecalculateBeatRankings(ctx, s.section, s.period)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "another worker holds the lock")
			require.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+"/exec error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectExec(s.sqlMatch).WillReturnError(errSpecBoom)
			mock.ExpectRollback()
			require.ErrorIs(t, repo.RecalculateBeatRankings(ctx, s.section, s.period), errSpecBoom)
			require.NoError(t, mock.ExpectationsWereMet())
		})
		t.Run(name+"/commit error", func(t *testing.T) {
			repo, mock, cleanup := newSpecRepoForTest(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectExec(s.sqlMatch).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit().WillReturnError(errSpecBoom)
			require.ErrorIs(t, repo.RecalculateBeatRankings(ctx, s.section, s.period), errSpecBoom)
		})
	}
}

func TestPGSpecRepository_Delete_Paths(t *testing.T) {
	ctx := context.Background()
	id, producerID := uuid.New(), uuid.New()
	countQ := `SELECT COUNT\(\*\) FROM licenses WHERE spec_id = \$1`
	count := func(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"count"}).AddRow(n) }

	t.Run("count error is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WithArgs(id).WillReturnError(errSpecBoom)
		err := repo.Delete(ctx, id, producerID)
		require.ErrorIs(t, err, errSpecBoom)
		assert.Contains(t, err.Error(), "failed to check license existence")
	})
	t.Run("purchased spec is soft deleted", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(2))
		mock.ExpectExec(`UPDATE specs SET is_deleted = TRUE`).WithArgs(id, producerID).WillReturnResult(sqlmock.NewResult(0, 1))
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), domain.ErrSpecSoftDeleted)
	})
	t.Run("soft delete exec error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(1))
		mock.ExpectExec(`UPDATE specs SET is_deleted`).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), errSpecBoom)
	})
	t.Run("soft delete rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(1))
		mock.ExpectExec(`UPDATE specs SET is_deleted`).WillReturnResult(sqlmock.NewErrorResult(errSpecBoom))
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), errSpecBoom)
	})
	t.Run("soft delete of other producer's spec is not found", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(1))
		mock.ExpectExec(`UPDATE specs SET is_deleted`).WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), domain.ErrSpecNotFound)
	})
	t.Run("hard delete success", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(0))
		mock.ExpectExec(`DELETE FROM specs WHERE id = \$1 AND producer_id = \$2`).WithArgs(id, producerID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.Delete(ctx, id, producerID))
	})
	t.Run("hard delete foreign key violation", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(0))
		mock.ExpectExec(`DELETE FROM specs`).WillReturnError(errors.New("violates foreign key constraint orders_spec_id_fkey"))
		err := repo.Delete(ctx, id, producerID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete spec with existing dependencies")
	})
	t.Run("hard delete other error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(0))
		mock.ExpectExec(`DELETE FROM specs`).WillReturnError(errSpecBoom)
		err := repo.Delete(ctx, id, producerID)
		require.ErrorIs(t, err, errSpecBoom)
		assert.NotContains(t, err.Error(), "cannot delete spec")
	})
	t.Run("hard delete rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(0))
		mock.ExpectExec(`DELETE FROM specs`).WillReturnResult(sqlmock.NewErrorResult(errSpecBoom))
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), errSpecBoom)
	})
	t.Run("hard delete nothing matched", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectQuery(countQ).WillReturnRows(count(0))
		mock.ExpectExec(`DELETE FROM specs`).WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.Delete(ctx, id, producerID), domain.ErrSpecNotFound)
	})
}

func TestPGSpecRepository_Update_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	specID, producerID, existingID := uuid.New(), uuid.New(), uuid.New()
	updateSpecQ := `UPDATE specs\s+SET title`
	existingQ := `SELECT \* FROM license_options WHERE spec_id = \$1 AND is_deleted = FALSE`
	insertQ := `INSERT INTO license_options`
	updateLicQ := `UPDATE license_options SET\s+license_type`
	usageQ := `SELECT COUNT\(\*\) FROM licenses WHERE license_option_id = \$1`
	existingRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name"}).
			AddRow(existingID, specID, "basic", "Basic")
	}
	base := func(licenses []domain.LicenseOption) *domain.Spec {
		return &domain.Spec{ID: specID, ProducerID: producerID, Title: "t", Licenses: licenses}
	}
	begin := func(m sqlmock.Sqlmock) {
		m.ExpectBegin()
		m.ExpectExec(updateSpecQ).WillReturnResult(sqlmock.NewResult(0, 1))
	}

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base(nil)), errSpecBoom)
	})
	t.Run("spec update error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(updateSpecQ).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base(nil)), errSpecBoom)
	})
	t.Run("rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(updateSpecQ).WillReturnResult(sqlmock.NewErrorResult(errSpecBoom))
		require.ErrorIs(t, repo.Update(ctx, base(nil)), errSpecBoom)
	})
	t.Run("spec not owned or missing", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(updateSpecQ).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		require.ErrorIs(t, repo.Update(ctx, base(nil)), domain.ErrSpecNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("nil licenses skips license sync and commits", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectCommit()
		require.NoError(t, repo.Update(ctx, base(nil)))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("commit error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectCommit().WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base(nil)), errSpecBoom)
	})
	t.Run("existing licenses select error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WithArgs(specID).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{})), errSpecBoom)
	})
	t.Run("matched-by-type license update error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectExec(updateLicQ).WillReturnError(errSpecBoom)
		spec := base([]domain.LicenseOption{{LicenseType: "basic", Name: "Basic"}})
		require.ErrorIs(t, repo.Update(ctx, spec), errSpecBoom)
		assert.Equal(t, existingID, spec.Licenses[0].ID, "ID should be reused from the license of the same type")
	})
	t.Run("new license insert error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(insertQ).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{{LicenseType: "premium"}})), errSpecBoom)
	})
	t.Run("unknown id update error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(updateLicQ).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{{ID: uuid.New(), LicenseType: "premium"}})), errSpecBoom)
	})
	t.Run("unknown id that does not exist is inserted", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(updateLicQ).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(insertQ).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Update(ctx, base([]domain.LicenseOption{{ID: uuid.New(), LicenseType: "premium"}})))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("unknown id insert fallback error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(updateLicQ).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(insertQ).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{{ID: uuid.New(), LicenseType: "premium"}})), errSpecBoom)
	})
	t.Run("removed license usage count error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectQuery(usageQ).WithArgs(existingID).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{})), errSpecBoom)
	})
	t.Run("removed license in use is soft deleted", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectQuery(usageQ).WithArgs(existingID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
		mock.ExpectExec(`UPDATE license_options SET is_deleted = TRUE`).WithArgs(existingID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Update(ctx, base([]domain.LicenseOption{})))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("removed license soft delete error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectQuery(usageQ).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
		mock.ExpectExec(`UPDATE license_options SET is_deleted = TRUE`).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{})), errSpecBoom)
	})
	t.Run("removed unused license is hard deleted", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectQuery(usageQ).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec(`DELETE FROM license_options WHERE id = \$1`).WithArgs(existingID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Update(ctx, base([]domain.LicenseOption{})))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("removed unused license hard delete error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		begin(mock)
		mock.ExpectQuery(existingQ).WillReturnRows(existingRows())
		mock.ExpectQuery(usageQ).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec(`DELETE FROM license_options`).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.Update(ctx, base([]domain.LicenseOption{})), errSpecBoom)
	})
}

func TestPGSpecRepository_UpdateFilesAndStatus_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	t.Run("invalid waveform peaks json", func(t *testing.T) {
		repo, _, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		bad := "not-json"
		err := repo.UpdateFilesAndStatus(ctx, id, map[string]*string{"waveform_peaks": &bad}, domain.ProcessingStatusCompleted)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode waveform peaks")
	})
	t.Run("exec error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectExec(`UPDATE specs SET processing_status`).WillReturnError(errSpecBoom)
		require.ErrorIs(t, repo.UpdateFilesAndStatus(ctx, id, nil, domain.ProcessingStatusFailed), errSpecBoom)
	})
	t.Run("rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectExec(`UPDATE specs SET processing_status`).WillReturnResult(sqlmock.NewErrorResult(errSpecBoom))
		require.ErrorIs(t, repo.UpdateFilesAndStatus(ctx, id, nil, domain.ProcessingStatusFailed), errSpecBoom)
	})
	t.Run("no row updated", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		mock.ExpectExec(`UPDATE specs SET processing_status`).WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.UpdateFilesAndStatus(ctx, id, nil, domain.ProcessingStatusFailed), domain.ErrSpecNotFound)
	})
	t.Run("all file columns are included", func(t *testing.T) {
		repo, mock, cleanup := newSpecRepoForTest(t)
		defer cleanup()
		img, prev, wav, stems, peaks := "i", "p", "w", "s", "[1,2,3]"
		mock.ExpectExec(`image_url = \?[\s\S]*preview_url = \?[\s\S]*wav_url = \?[\s\S]*stems_url = \?[\s\S]*waveform_peaks = \?`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		err := repo.UpdateFilesAndStatus(ctx, id, map[string]*string{
			"image_url": &img, "preview_url": &prev, "wav_url": &wav, "stems_url": &stems, "waveform_peaks": &peaks,
		}, domain.ProcessingStatusCompleted)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
