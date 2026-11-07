package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/require"
)

func expectEmptyLicensePrices(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM license_option_prices WHERE license_option_id IN`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func TestPricesUpsertRollsBackAllCurrenciesOnConflict(t *testing.T) {
	repo, mock, cleanup := newSpecRepoForTest(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO license_option_prices`).WillReturnRows(sqlmock.NewRows([]string{"id", "revision", "amount_major"}).AddRow(uuid.New(), 2, 12.0))
	mock.ExpectQuery(`INSERT INTO license_option_prices`).WillReturnRows(sqlmock.NewRows([]string{"id", "revision", "amount_major"}))
	mock.ExpectRollback()
	err := repo.UpsertLicensePrices(context.Background(), uuid.New(), []money.StoredPrice{
		{Currency: "INR", AmountMinor: 1200, Revision: 1, IsActive: true},
		{Currency: "USD", AmountMinor: 1500, Revision: 1, IsActive: true},
	})
	require.ErrorIs(t, err, money.ErrStalePrice)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPriceHydrationErrorIsNotHidden(t *testing.T) {
	repo, mock, cleanup := newSpecRepoForTest(t)
	defer cleanup()
	id, producer := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT s`).WillReturnRows(specRow(id, producer))
	mock.ExpectQuery(`FROM license_options`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	boom := errors.New("price query failed")
	mock.ExpectQuery(`FROM license_option_prices`).WillReturnError(boom)
	_, err := repo.GetByID(context.Background(), id)
	require.ErrorIs(t, err, boom)
	require.NoError(t, mock.ExpectationsWereMet())
}
