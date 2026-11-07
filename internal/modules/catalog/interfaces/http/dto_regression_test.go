package http_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalog "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	dto "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

func TestDTORegression_AbsentCanonicalPriceNotFabricated(t *testing.T) {
	specID := uuid.New()
	licID := uuid.New()
	spec := &catalog.Spec{
		ID:            specID,
		PriceCurrency: "INR",
		BasePrice:     1000,
		Licenses: []catalog.LicenseOption{
			{
				ID:            licID,
				SpecID:        specID,
				Price:         1000,
				PriceCurrency: "INR",
				Prices:        nil, // No canonical price rows
			},
		},
	}

	resp := dto.ToSpecResponseForCurrency(spec, "USD")
	require.Len(t, resp.Licenses, 1)

	sel := resp.Licenses[0].SelectedPrice
	require.NotNil(t, sel)
	assert.False(t, sel.IsPurchasable, "Option without stored canonical price must NOT be purchasable")
	assert.Nil(t, sel.PriceID, "Fabricated price ID must not equal license ID")
	assert.Equal(t, money.SelectionUnavailable, sel.Selection)
}

func TestDTORegression_NativeScalarPricePreservedForOwnerEdit(t *testing.T) {
	specID := uuid.New()
	licID := uuid.New()
	spec := &catalog.Spec{
		ID:            specID,
		PriceCurrency: "INR",
		BasePrice:     1000,
		Licenses: []catalog.LicenseOption{
			{
				ID:            licID,
				SpecID:        specID,
				Price:         1000,
				PriceCurrency: "INR",
				Prices: []money.StoredPrice{
					{
						ID:              uuid.New(),
						LicenseOptionID: licID,
						Currency:        "USD",
						AmountMinor:     1499,
						AmountMajor:     14.99,
						Revision:        1,
						IsActive:        true,
					},
				},
			},
		},
	}

	resp := dto.ToSpecResponseForCurrency(spec, "USD")
	require.Len(t, resp.Licenses, 1)

	lic := resp.Licenses[0]
	assert.Equal(t, 1000.0, lic.Price, "License.Price must remain the native scalar price to protect owner edits from corruption")
	assert.Equal(t, 1000.0, lic.PriceMoney.AmountMajor)
	assert.Equal(t, 14.99, lic.SelectedPrice.AmountMajor)
	assert.Equal(t, "USD", lic.SelectedPrice.Currency)
}

func TestDTORegression_StartingPricePrioritizesRequestedCurrencyOverPaise(t *testing.T) {
	specID := uuid.New()
	spec := &catalog.Spec{
		ID:            specID,
		PriceCurrency: "INR",
		BasePrice:     1000,
		Licenses: []catalog.LicenseOption{
			{
				ID:            uuid.New(),
				Price:         1000,
				PriceCurrency: "INR",
				Prices: []money.StoredPrice{
					{
						ID:          uuid.New(),
						Currency:    "INR",
						AmountMinor: 100000, // 100,000 paise
						AmountMajor: 1000.0,
						Revision:    1,
						IsActive:    true,
					},
				},
			},
			{
				ID:            uuid.New(),
				Price:         5000,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{
						ID:          uuid.New(),
						Currency:    "USD",
						AmountMinor: 500000, // 500,000 cents ($5,000.00) > 100,000 paise
						AmountMajor: 5000.0,
						Revision:    1,
						IsActive:    true,
					},
				},
			},
		},
	}

	resp := dto.ToSpecResponseForCurrency(spec, "USD")
	require.NotNil(t, resp.SelectedPrice)
	assert.Equal(t, "USD", resp.SelectedPrice.Currency, "Starting price for USD request must prioritize exact USD offers over INR fallback")
	assert.Equal(t, 500000, resp.SelectedPrice.AmountMinor)
}
