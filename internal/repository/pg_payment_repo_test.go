package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGPaymentRepository_CreateAndGets(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewPaymentRepository(db)
	ctx := context.Background()
	id := uuid.New()
	orderID := uuid.New()
	paymentID := "pay_123"

	p := &domain.Payment{
		ID:                id,
		OrderID:           orderID,
		RazorpayPaymentID: paymentID,
		RazorpaySignature: "sig",
		Amount:            1000,
		Currency:          "INR",
		Status:            domain.PaymentStatusCaptured,
	}
	mock.ExpectExec("INSERT INTO payments").
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.Create(ctx, p))

	rows := sqlmock.NewRows([]string{
		"id", "order_id", "razorpay_payment_id", "razorpay_signature", "amount", "currency",
		"status", "created_at", "updated_at",
	}).AddRow(id, orderID, paymentID, "sig", 1000, "INR", "captured", time.Now(), time.Now())

	mock.ExpectQuery("SELECT \\* FROM payments WHERE id = \\$1").WithArgs(id).WillReturnRows(rows)
	got, err := repo.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)

	rows = sqlmock.NewRows([]string{
		"id", "order_id", "razorpay_payment_id", "razorpay_signature", "amount", "currency",
		"status", "created_at", "updated_at",
	}).AddRow(id, orderID, paymentID, "sig", 1000, "INR", "captured", time.Now(), time.Now())
	mock.ExpectQuery("SELECT \\* FROM payments WHERE order_id = \\$1").WithArgs(orderID).WillReturnRows(rows)
	_, err = repo.GetByOrderID(ctx, orderID)
	require.NoError(t, err)

	rows = sqlmock.NewRows([]string{
		"id", "order_id", "razorpay_payment_id", "razorpay_signature", "amount", "currency",
		"status", "created_at", "updated_at",
	}).AddRow(id, orderID, paymentID, "sig", 1000, "INR", "captured", time.Now(), time.Now())
	mock.ExpectQuery("SELECT \\* FROM payments WHERE razorpay_payment_id = \\$1").WithArgs(paymentID).WillReturnRows(rows)
	_, err = repo.GetByRazorpayID(ctx, paymentID)
	require.NoError(t, err)
}
