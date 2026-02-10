package repository_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGOrderRepository_CreateAndGetByID(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()
	orderID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()
	razorID := "order_123"
	order := &domain.Order{
		ID:              orderID,
		UserID:          userID,
		SpecID:          specID,
		LicenseType:     "Basic",
		Amount:          1000,
		Currency:        "INR",
		RazorpayOrderID: &razorID,
		Status:          domain.OrderStatusPending,
		Notes:           map[string]any{"k": "v"},
		ExpiresAt:       time.Now().Add(time.Hour),
	}

	mock.ExpectExec("INSERT INTO orders").
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.Create(ctx, order))

	notes, _ := json.Marshal(order.Notes)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "spec_id", "license_type", "amount", "currency",
		"razorpay_order_id", "status", "notes", "created_at", "updated_at", "expires_at",
	}).AddRow(orderID, userID, specID, "Basic", 1000, "INR", razorID, "pending", notes, time.Now(), time.Now(), time.Now())

	mock.ExpectQuery("SELECT id, user_id, spec_id, license_type, amount, currency").
		WithArgs(orderID).WillReturnRows(rows)
	out, err := repo.GetByID(ctx, orderID)
	require.NoError(t, err)
	assert.Equal(t, orderID, out.ID)

	mock.ExpectQuery("SELECT id, user_id, spec_id, license_type, amount, currency").
		WithArgs(uuid.New()).WillReturnError(sql.ErrNoRows)
	_, err = repo.GetByID(ctx, uuid.New())
	assert.Error(t, err)
}

func TestPGOrderRepository_OtherMethods(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()
	id := uuid.New()
	userID := uuid.New()

	mock.ExpectExec("UPDATE orders SET status = \\$1, updated_at = NOW\\(\\) WHERE id = \\$2").
		WithArgs(domain.OrderStatusPaid, id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.UpdateStatus(ctx, id, domain.OrderStatusPaid))

	rows := sqlmock.NewRows([]string{"id", "user_id", "spec_id", "license_type", "amount", "currency", "status", "created_at", "updated_at", "expires_at"}).
		AddRow(id, userID, uuid.New(), "Basic", 1000, "INR", "pending", time.Now(), time.Now(), time.Now())
	mock.ExpectQuery("SELECT \\* FROM orders WHERE user_id = \\$1 ORDER BY created_at DESC LIMIT \\$2 OFFSET \\$3").
		WithArgs(userID, 20, 0).WillReturnRows(rows)
	orders, err := repo.ListByUser(ctx, userID, 20, 0)
	require.NoError(t, err)
	assert.Len(t, orders, 1)

	razorID := "order_razor"
	mock.ExpectQuery("SELECT \\* FROM orders WHERE razorpay_order_id = \\$1").
		WithArgs(razorID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "spec_id", "license_type", "amount", "currency", "status", "razorpay_order_id"}).
			AddRow(id, userID, uuid.New(), "Basic", 1000, "INR", "pending", razorID))
	order, err := repo.GetByRazorpayID(ctx, razorID)
	require.NoError(t, err)
	assert.Equal(t, razorID, *order.RazorpayOrderID)

	mock.ExpectQuery("SELECT \\* FROM orders WHERE razorpay_order_id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)
	_, err = repo.GetByRazorpayID(ctx, "missing")
	assert.Error(t, err)
}
