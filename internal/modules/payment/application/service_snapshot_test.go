package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	sharedmoney "github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockSnapshotter struct {
	mock.Mock
}

func (m *mockSnapshotter) Snapshot(ctx context.Context, orderCurrency string) (*domain.OrderFXSnapshot, error) {
	args := m.Called(ctx, orderCurrency)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderFXSnapshot), args.Error(1)
}

func TestPaymentService_FXSnapshotDoesNotAlterOrBlockOrder(t *testing.T) {
	specID := uuid.New()
	licenseOptionID := uuid.New()
	userID := uuid.New()
	priceID := uuid.New()

	spec := &catalogDomain.Spec{
		ID:               specID,
		Title:            "Snapshot Beat",
		PriceCurrency:    "USD",
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
		Licenses: []catalogDomain.LicenseOption{
			{
				ID:            licenseOptionID,
				SpecID:        specID,
				LicenseType:   catalogDomain.LicenseBasic,
				Name:          "Basic License",
				PriceCurrency: "USD",
				Prices: []sharedmoney.StoredPrice{
					{
						ID:          priceID,
						Currency:    "USD",
						AmountMinor: 1999,
						AmountMajor: 19.99,
						IsActive:    true,
						Revision:    1,
					},
				},
			},
		},
	}

	tests := []struct {
		name                 string
		snapshotterReturn    *domain.OrderFXSnapshot
		snapshotterErr       error
		expectReportCurrency *string
		expectReportFXRate   *string
	}{
		{
			name: "snapshot successful",
			snapshotterReturn: &domain.OrderFXSnapshot{
				ReportCurrency: "INR",
				ReportFXRate:   func() *string { s := "83.12000000"; return &s }(),
			},
			snapshotterErr:       nil,
			expectReportCurrency: func() *string { s := "INR"; return &s }(),
			expectReportFXRate:   func() *string { s := "83.12000000"; return &s }(),
		},
		{
			name:                 "snapshot returns nil (unusable/missing) without error",
			snapshotterReturn:    nil,
			snapshotterErr:       nil,
			expectReportCurrency: nil,
			expectReportFXRate:   nil,
		},
		{
			name:                 "snapshot returns error (provider failure) without blocking",
			snapshotterReturn:    nil,
			snapshotterErr:       errors.New("provider timeout"),
			expectReportCurrency: nil,
			expectReportFXRate:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, or, _, _, sf, _, _, _ := newPaymentSvc()
			sn := new(mockSnapshotter)
			s.SetReportingRateSnapshotter(sn)

			sf.On("FindWithLicenses", mock.Anything, specID).Return(spec, nil).Once()
			sn.On("Snapshot", mock.Anything, "USD").Return(tt.snapshotterReturn, tt.snapshotterErr).Once()

			var capturedOrder *domain.Order
			or.On("Create", mock.Anything, mock.MatchedBy(func(o *domain.Order) bool {
				capturedOrder = o
				return true
			})).Return(nil).Once()

			order, err := s.CreateOrder(context.Background(), userID, specID, licenseOptionID, "USD")
			require.NoError(t, err)
			require.NotNil(t, order)

			// Invariants strictly asserted:
			assert.Equal(t, 1999, order.Amount)
			assert.Equal(t, "USD", order.Currency)
			assert.Equal(t, tt.expectReportCurrency, order.ReportCurrency)
			assert.Equal(t, tt.expectReportFXRate, order.ReportFXRate)
			assert.Equal(t, 1999, capturedOrder.Amount)
			assert.Equal(t, "USD", capturedOrder.Currency)

			or.AssertExpectations(t)
			sn.AssertExpectations(t)
		})
	}
}
