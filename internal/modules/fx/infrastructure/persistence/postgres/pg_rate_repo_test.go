package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	return sqlx.NewDb(sqlDB, "sqlmock"), mock, func() { _ = sqlDB.Close() }
}

func TestPgRateRepository_Append(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	rate := &domain.Rate{
		Base:   "USD",
		Quote:  "INR",
		Rate:   big.NewRat(8312, 100),
		Source: "frankfurter",
		AsOf:   now,
	}

	mock.ExpectQuery("INSERT INTO fx_rates").
		WithArgs("USD", "INR", "83.12000000", "frankfurter", now, "accepted").
		WillReturnRows(sqlmock.NewRows([]string{"id", "fetched_at"}).AddRow(int64(42), now))

	err := repo.Append(ctx, rate)
	require.NoError(t, err)
	assert.Equal(t, int64(42), rate.ID)
	assert.Equal(t, "83.12000000", rate.RateDecimal)

	// Error case: db failure
	mock.ExpectQuery("INSERT INTO fx_rates").
		WillReturnError(errors.New("db write failed"))
	err = repo.Append(ctx, rate)
	assert.Error(t, err)
}

func TestPgRateRepository_AppendBatch(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	rates := []*domain.Rate{
		{Base: "USD", Quote: "INR", Rate: big.NewRat(8312, 100), Source: "frankfurter", AsOf: now},
		{Base: "INR", Quote: "USD", Rate: big.NewRat(12, 1000), Source: "frankfurter", AsOf: now},
	}

	mock.ExpectBegin()
	mock.ExpectPrepare("INSERT INTO fx_rates")
	mock.ExpectQuery("INSERT INTO fx_rates").
		WithArgs("USD", "INR", "83.12000000", "frankfurter", now, "accepted").
		WillReturnRows(sqlmock.NewRows([]string{"id", "fetched_at"}).AddRow(int64(1), now))
	mock.ExpectQuery("INSERT INTO fx_rates").
		WithArgs("INR", "USD", "0.01200000", "frankfurter", now, "accepted").
		WillReturnRows(sqlmock.NewRows([]string{"id", "fetched_at"}).AddRow(int64(2), now))
	mock.ExpectCommit()

	err := repo.AppendBatch(ctx, rates)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rates[0].ID)
	assert.Equal(t, int64(2), rates[1].ID)

	// Empty slice is a no-op
	require.NoError(t, repo.AppendBatch(ctx, nil))
}

func TestPgRateRepository_Latest(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "base", "quote", "rate", "source", "as_of", "fetched_at", "status"}).
		AddRow(int64(10), "USD", "INR", "83.12000000", "frankfurter", now, now, "accepted")

	mock.ExpectQuery("SELECT id, base, quote, rate::TEXT AS rate").
		WithArgs("USD", "INR").
		WillReturnRows(rows)

	rate, err := repo.Latest(ctx, "USD", "INR")
	require.NoError(t, err)
	assert.Equal(t, int64(10), rate.ID)
	assert.Equal(t, "83.12000000", rate.RateDecimal)
	assert.Equal(t, 0, rate.Rate.Cmp(big.NewRat(8312, 100)))

	// Not found case
	mock.ExpectQuery("SELECT id, base, quote, rate::TEXT AS rate").
		WithArgs("USD", "EUR").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.Latest(ctx, "USD", "EUR")
	assert.ErrorIs(t, err, domain.ErrNoRate)
}

func TestPgRateRepository_History(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	filter := domain.HistoryFilter{
		Base:   "USD",
		Quote:  "INR",
		Status: domain.RateStatusAccepted,
		Limit:  10,
		Offset: 0,
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM fx_rates WHERE 1=1 AND base = \\$1 AND quote = \\$2 AND status = \\$3").
		WithArgs("USD", "INR", "accepted").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{"id", "base", "quote", "rate", "source", "as_of", "fetched_at", "status"}).
		AddRow(int64(1), "USD", "INR", "83.12000000", "frankfurter", now, now, "accepted")

	mock.ExpectQuery("SELECT id, base, quote, rate::TEXT AS rate.*ORDER BY fetched_at DESC").
		WithArgs("USD", "INR", "accepted", 10, 0).
		WillReturnRows(rows)

	results, total, err := repo.History(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "USD", results[0].Base)
	assert.Equal(t, "INR", results[0].Quote)
}

func TestPgRateRepository_LatestPerPair(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "base", "quote", "rate", "source", "as_of", "fetched_at", "status"}).
		AddRow(int64(1), "USD", "INR", "83.12000000", "frankfurter", now, now, "accepted").
		AddRow(int64(2), "INR", "USD", "0.01200000", "frankfurter", now, now, "accepted")

	mock.ExpectQuery("SELECT DISTINCT ON \\(base, quote\\)").
		WillReturnRows(rows)

	results, err := repo.LatestPerPair(ctx)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "USD", results[0].Base)
	assert.Equal(t, "INR", results[1].Base)
}

func TestPgRateRepository_TryLock(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewRateRepository(db)
	ctx := context.Background()

	// 1. Lock acquired
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs("fx_refresh").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	unlock, acquired, err := repo.TryLock(ctx, "fx_refresh")
	require.NoError(t, err)
	assert.True(t, acquired)
	assert.NotNil(t, unlock)

	// Test unlock execution
	mock.ExpectQuery("SELECT pg_advisory_unlock").
		WithArgs("fx_refresh").
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true))
	require.NoError(t, unlock())

	// 2. Lock not acquired (busy)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs("fx_refresh").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	unlock, acquired, err = repo.TryLock(ctx, "fx_refresh")
	require.NoError(t, err)
	assert.False(t, acquired)
	assert.Nil(t, unlock)
}
