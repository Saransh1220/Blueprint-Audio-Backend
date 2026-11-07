package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPaymentService_GetOrderStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("order not found returns error", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		userID := uuid.New()
		or.On("GetByID", ctx, orderID).Return(nil, errors.New("sql: no rows")).Once()

		res, err := s.GetOrderStatus(ctx, orderID, userID)
		assert.Nil(t, res)
		assert.EqualError(t, err, "order not found")
	})

	t.Run("user id mismatch returns order not found", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		ownerID := uuid.New()
		requesterID := uuid.New()
		order := &domain.Order{
			ID:       orderID,
			UserID:   ownerID,
			Status:   domain.OrderStatusPending,
			Provider: domain.ProviderRazorpay,
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		res, err := s.GetOrderStatus(ctx, orderID, requesterID)
		assert.Nil(t, res)
		assert.EqualError(t, err, "order not found")
	})

	t.Run("pending status returns retry_after_seconds=2 and nil license", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		userID := uuid.New()
		order := &domain.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    domain.OrderStatusPending,
			Provider:  domain.ProviderRazorpay,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		res, err := s.GetOrderStatus(ctx, orderID, userID)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, orderID, res.OrderID)
		assert.Equal(t, domain.OrderStatusPending, res.Status)
		assert.Equal(t, domain.ProviderRazorpay, res.Provider)
		assert.Equal(t, 2, res.RetryAfterSeconds)
		assert.Nil(t, res.License)
	})

	t.Run("processing status returns retry_after_seconds=2", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		userID := uuid.New()
		order := &domain.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    domain.OrderStatusProcessing,
			Provider:  domain.ProviderDodo,
			ExpiresAt: time.Now().Add(5 * time.Minute),
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		res, err := s.GetOrderStatus(ctx, orderID, userID)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, domain.OrderStatusProcessing, res.Status)
		assert.Equal(t, 2, res.RetryAfterSeconds)
		assert.Nil(t, res.License)
	})

	t.Run("failed status returns retry_after_seconds=0", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		userID := uuid.New()
		order := &domain.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    domain.OrderStatusFailed,
			Provider:  domain.ProviderRazorpay,
			ExpiresAt: time.Now().Add(-5 * time.Minute),
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		res, err := s.GetOrderStatus(ctx, orderID, userID)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, domain.OrderStatusFailed, res.Status)
		assert.Equal(t, 0, res.RetryAfterSeconds)
		assert.Nil(t, res.License)
	})

	t.Run("paid status returns retry_after_seconds=0 and populated license", func(t *testing.T) {
		s, or, _, lr, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		userID := uuid.New()
		licenseID := uuid.New()
		order := &domain.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    domain.OrderStatusPaid,
			Provider:  domain.ProviderRazorpay,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		license := &domain.License{
			ID:         licenseID,
			OrderID:    orderID,
			UserID:     userID,
			LicenseKey: "LIC-RW-TEST-12345",
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()
		lr.On("GetByOrderID", ctx, orderID).Return(license, nil).Once()

		res, err := s.GetOrderStatus(ctx, orderID, userID)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, domain.OrderStatusPaid, res.Status)
		assert.Equal(t, 0, res.RetryAfterSeconds)
		require.NotNil(t, res.License)
		assert.Equal(t, licenseID, res.License.ID)
		assert.Equal(t, "LIC-RW-TEST-12345", res.License.LicenseKey)
	})
}

func TestPaymentService_HandleWebhook_Comprehensive(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves order by ProviderCheckoutID when OrderID is nil (Razorpay)", func(t *testing.T) {
		s, or, pr, lr, _, _, _, _ := newPaymentSvc()
		s.emailSender = nil
		rzpCheckoutID := "order_rzp_webhook_100"
		orderID := uuid.New()
		userID := uuid.New()

		fakeOf(s, domain.ProviderRazorpay).webhook = &domain.PaymentConfirmation{
			OrderID:             uuid.Nil,
			ProviderCheckoutID:  rzpCheckoutID,
			ProviderPaymentID:   "pay_rzp_webhook_200",
			ReportedAmountMinor: 49900,
			ReportedCurrency:    "INR",
		}

		order := &domain.Order{
			ID:                 orderID,
			UserID:             userID,
			Provider:           domain.ProviderRazorpay,
			ProviderCheckoutID: &rzpCheckoutID,
			Status:             domain.OrderStatusPending,
			Amount:             49900,
			Currency:           "INR",
			LicenseType:        "Unlimited",
			Notes:              map[string]any{"license_option_id": uuid.New().String()},
		}

		or.On("GetByProviderCheckoutID", ctx, domain.ProviderRazorpay, rzpCheckoutID).Return(order, nil).Once()
		or.On("MarkPaidIfPayable", ctx, orderID, "pay_rzp_webhook_200").Return(true, nil).Once()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderRazorpay, []byte("{}"), nil)
		require.NoError(t, err)
	})

	t.Run("amount mismatch returns ErrAmountMismatch", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		order := &domain.Order{
			ID:       orderID,
			Provider: domain.ProviderDodo,
			Status:   domain.OrderStatusPending,
			Amount:   1000,
			Currency: "USD",
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_1",
			ReportedAmountMinor: 2000, // mismatch!
			ReportedCurrency:    "USD",
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		assert.ErrorIs(t, err, domain.ErrAmountMismatch)
	})

	t.Run("currency mismatch returns ErrAmountMismatch", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		order := &domain.Order{
			ID:       orderID,
			Provider: domain.ProviderDodo,
			Status:   domain.OrderStatusPending,
			Amount:   1000,
			Currency: "USD",
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_1",
			ReportedAmountMinor: 1000,
			ReportedCurrency:    "EUR", // mismatch!
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		assert.ErrorIs(t, err, domain.ErrAmountMismatch)
	})

	t.Run("D5-b: failed order with verified payment transitions to paid", func(t *testing.T) {
		s, or, pr, lr, _, _, _, _ := newPaymentSvc()
		s.emailSender = nil
		orderID := uuid.New()
		userID := uuid.New()
		order := &domain.Order{
			ID:          orderID,
			UserID:      userID,
			Provider:    domain.ProviderDodo,
			Status:      domain.OrderStatusFailed, // was marked failed e.g. from verify glitch
			Amount:      1500,
			Currency:    "USD",
			LicenseType: "Standard",
			Notes:       map[string]any{"license_option_id": uuid.New().String()},
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_revived",
			ReportedAmountMinor: 1500,
			ReportedCurrency:    "USD",
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()
		or.On("MarkPaidIfPayable", ctx, orderID, "pay_dodo_revived").Return(true, nil).Once()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		require.NoError(t, err)
	})

	t.Run("D5-b: cancelled order rejected as not payable", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		order := &domain.Order{
			ID:       orderID,
			Provider: domain.ProviderDodo,
			Status:   domain.OrderStatusCancelled,
			Amount:   1500,
			Currency: "USD",
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_canc",
			ReportedAmountMinor: 1500,
			ReportedCurrency:    "USD",
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		assert.ErrorIs(t, err, domain.ErrOrderNotPayable)
	})

	t.Run("D5-b: refunded order rejected as not payable", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		orderID := uuid.New()
		order := &domain.Order{
			ID:       orderID,
			Provider: domain.ProviderDodo,
			Status:   domain.OrderStatusRefunded,
			Amount:   1500,
			Currency: "USD",
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_ref",
			ReportedAmountMinor: 1500,
			ReportedCurrency:    "USD",
		}
		or.On("GetByID", ctx, orderID).Return(order, nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		assert.ErrorIs(t, err, domain.ErrOrderNotPayable)
	})

	t.Run("duplicate delivery where MarkPaidIfPayable returns false is idempotent", func(t *testing.T) {
		s, or, pr, lr, _, _, _, _ := newPaymentSvc()
		s.emailSender = nil
		orderID := uuid.New()
		userID := uuid.New()
		order := &domain.Order{
			ID:          orderID,
			UserID:      userID,
			Provider:    domain.ProviderDodo,
			Status:      domain.OrderStatusPending,
			Amount:      1000,
			Currency:    "USD",
			LicenseType: "Standard",
		}
		fakeOf(s, domain.ProviderDodo).webhook = &domain.PaymentConfirmation{
			OrderID:             orderID,
			ProviderPaymentID:   "pay_dodo_dup",
			ReportedAmountMinor: 1000,
			ReportedCurrency:    "USD",
		}
		existingLicense := &domain.License{
			ID:      uuid.New(),
			OrderID: orderID,
			UserID:  userID,
		}

		or.On("GetByID", ctx, orderID).Return(order, nil).Once()
		// Another concurrent process won the race and marked it paid:
		or.On("MarkPaidIfPayable", ctx, orderID, "pay_dodo_dup").Return(false, nil).Once()
		lr.On("GetByOrderID", ctx, orderID).Return(existingLicense, nil).Once()

		err := s.HandleWebhook(ctx, domain.ProviderDodo, []byte("{}"), nil)
		require.NoError(t, err)
		pr.AssertNotCalled(t, "Create")
	})
}
