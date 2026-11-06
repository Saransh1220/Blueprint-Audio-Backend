package application

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	sharedmoney "github.com/saransh1220/blueprint-audio/internal/shared/money"
)

func TestPaymentServiceRegression_AbsentPricesRejected(t *testing.T) {
	ctx := context.Background()
	s, _, _, _, sf, _, _, _ := newPaymentSvc()
	specID := uuid.New()
	licID := uuid.New()
	spec := &catalogDomain.Spec{
		ID:               specID,
		Title:            "Beat 1",
		PriceCurrency:    "INR",
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
		Licenses: []catalogDomain.LicenseOption{
			{
				ID:            licID,
				SpecID:        specID,
				Price:         1000,
				PriceCurrency: "INR",
				Prices:        nil, // Empty prices: unreviewed legacy row
			},
		},
	}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()

	priceID := uuid.New()
	rev := int64(1)
	minor := 100000
	curr := "INR"

	_, err := s.CreateOrderWithOffer(ctx, CreateOrderInput{
		UserID:              uuid.New(),
		SpecID:              specID,
		LicenseOptionID:     licID,
		Currency:            "INR",
		ExpectedPriceID:     &priceID,
		ExpectedRevision:    &rev,
		ExpectedAmountMinor: &minor,
		ExpectedCurrency:    &curr,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sharedmoney.ErrPriceUnavailable, "Absent canonical prices must be rejected with ErrPriceUnavailable, not fabricated into purchasable order")
}

func TestPaymentServiceRegression_MissingExpectedEvidenceRejected(t *testing.T) {
	ctx := context.Background()
	s, _, _, _, sf, _, _, _ := newPaymentSvc()
	specID := uuid.New()
	licID := uuid.New()
	priceID := uuid.New()
	spec := &catalogDomain.Spec{
		ID:               specID,
		Title:            "Beat 1",
		PriceCurrency:    "INR",
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
		Licenses: []catalogDomain.LicenseOption{
			{
				ID:            licID,
				SpecID:        specID,
				Price:         1000,
				PriceCurrency: "INR",
				Prices: []sharedmoney.StoredPrice{
					{
						ID:              priceID,
						LicenseOptionID: licID,
						Currency:        "INR",
						AmountMinor:     100000,
						AmountMajor:     1000.0,
						Revision:        1,
						IsActive:        true,
					},
				},
			},
		},
	}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()

	// Calling with nil expected evidence must be rejected, not allowed to bypass validation
	_, err := s.CreateOrderWithOffer(ctx, CreateOrderInput{
		UserID:              uuid.New(),
		SpecID:              specID,
		LicenseOptionID:     licID,
		Currency:            "INR",
		ExpectedPriceID:     nil, // Missing evidence
		ExpectedRevision:    nil,
		ExpectedAmountMinor: nil,
		ExpectedCurrency:    nil,
	})

	require.Error(t, err, "Missing expected evidence must be rejected")
}

func TestPaymentServiceRegression_UnconfirmedNativeFallbackRejected(t *testing.T) {
	ctx := context.Background()
	s, _, _, _, sf, _, _, _ := newPaymentSvc()
	specID := uuid.New()
	licID := uuid.New()
	priceID := uuid.New()
	spec := &catalogDomain.Spec{
		ID:               specID,
		Title:            "Beat 1",
		PriceCurrency:    "INR",
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
		Licenses: []catalogDomain.LicenseOption{
			{
				ID:            licID,
				SpecID:        specID,
				Price:         1000,
				PriceCurrency: "INR",
				Prices: []sharedmoney.StoredPrice{
					{
						ID:              priceID,
						LicenseOptionID: licID,
						Currency:        "INR",
						AmountMinor:     100000,
						AmountMajor:     1000.0,
						Revision:        1,
						IsActive:        true,
					},
				},
			},
		},
	}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()

	rev := int64(1)
	minor := 100000
	usdCurr := "USD" // Client expected USD, but track only has INR!

	_, err := s.CreateOrderWithOffer(ctx, CreateOrderInput{
		UserID:              uuid.New(),
		SpecID:              specID,
		LicenseOptionID:     licID,
		Currency:            "USD", // Requested USD
		ExpectedPriceID:     &priceID,
		ExpectedRevision:    &rev,
		ExpectedAmountMinor: &minor,
		ExpectedCurrency:    &usdCurr, // Expected USD, but current offer is INR fallback!
	})

	require.Error(t, err, "Unconfirmed fallback currency must be rejected")
	var mismatch *PriceMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, "INR", mismatch.CurrentOffer.Currency)
}
