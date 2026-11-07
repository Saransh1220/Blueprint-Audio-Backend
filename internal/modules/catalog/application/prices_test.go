package application

import (
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPrepareLicensePricesPreservesInactiveAndProvenance(t *testing.T) {
	id := uuid.New()
	old := &domain.Spec{PriceCurrency: "USD", Licenses: []domain.LicenseOption{{ID: id, Price: 15, PriceCurrency: "USD", Prices: []money.StoredPrice{{Currency: "USD", AmountMinor: 1500, Revision: 3, IsActive: false, Source: "migration"}, {Currency: "INR", AmountMinor: 100000, Revision: 2, IsActive: true, Source: "owner"}}}}}
	edited := &domain.Spec{Licenses: []domain.LicenseOption{{ID: id, Price: 15}}}
	require.NoError(t, prepareLicensePrices(edited, old))
	require.Equal(t, "USD", edited.Licenses[0].PriceCurrency)
	require.False(t, edited.Licenses[0].Prices[0].IsActive)
	require.Equal(t, "migration", edited.Licenses[0].Prices[0].Source)
	require.Equal(t, 100000, edited.Licenses[0].Prices[1].AmountMinor)
}

func TestPrepareLicensePricesRejectsStaleExplicitPatch(t *testing.T) {
	id := uuid.New()
	old := &domain.Spec{PriceCurrency: "USD", Licenses: []domain.LicenseOption{{ID: id, Price: 15, PriceCurrency: "USD", Prices: []money.StoredPrice{{Currency: "USD", AmountMinor: 1500, Revision: 3, IsActive: true}}}}}
	edited := &domain.Spec{Licenses: []domain.LicenseOption{{ID: id, Price: 15, Prices: []money.StoredPrice{{Currency: "USD", AmountMinor: 1200, Revision: 2, IsActive: true}}}}}
	require.ErrorIs(t, prepareLicensePrices(edited, old), money.ErrStalePrice)
}

func TestPrepareLicensePrices_CreateWithMultiplePrices(t *testing.T) {
	spec := &domain.Spec{
		PriceCurrency: "USD",
		Licenses: []domain.LicenseOption{
			{
				Name:          "Basic",
				Price:         19.99,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{Currency: "USD", AmountMinor: 1999, IsActive: true},
					{Currency: "INR", AmountMinor: 149900, IsActive: true},
				},
			},
		},
	}
	require.NoError(t, prepareLicensePrices(spec, nil))
	require.Len(t, spec.Licenses[0].Prices, 2)
	assertPrice := func(cur string, minor int, source string, rev int64) {
		for _, p := range spec.Licenses[0].Prices {
			if p.Currency == cur {
				require.Equal(t, minor, p.AmountMinor)
				require.Equal(t, source, p.Source)
				require.Equal(t, rev, p.Revision)
				require.True(t, p.IsActive)
				return
			}
		}
		t.Fatalf("currency %s not found", cur)
	}
	assertPrice("USD", 1999, "owner", 1)
	assertPrice("INR", 149900, "owner", 1)
}

func TestPrepareLicensePrices_CreateRejectsConflictingScalarAndPrices(t *testing.T) {
	spec := &domain.Spec{
		PriceCurrency: "USD",
		Licenses: []domain.LicenseOption{
			{
				Name:          "Basic",
				Price:         19.99,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{Currency: "USD", AmountMinor: 2500, IsActive: true}, // 2500 != 1999
				},
			},
		},
	}
	err := prepareLicensePrices(spec, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflicting price for USD")
}

func TestPrepareLicensePrices_CreateRejectsNegativeAmount(t *testing.T) {
	spec := &domain.Spec{
		PriceCurrency: "USD",
		Licenses: []domain.LicenseOption{
			{
				Name:          "Basic",
				Price:         19.99,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{Currency: "INR", AmountMinor: -100, IsActive: true},
				},
			},
		},
	}
	err := prepareLicensePrices(spec, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, money.ErrNegativeAmount)
}

func TestPrepareLicensePrices_CreateRejectsDuplicateCurrencies(t *testing.T) {
	spec := &domain.Spec{
		PriceCurrency: "USD",
		Licenses: []domain.LicenseOption{
			{
				Name:          "Basic",
				Price:         19.99,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{Currency: "INR", AmountMinor: 100000, IsActive: true},
					{Currency: "INR", AmountMinor: 120000, IsActive: true},
				},
			},
		},
	}
	err := prepareLicensePrices(spec, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate price currency INR")
}

func TestPrepareLicensePrices_ServerForcesOwnerSource(t *testing.T) {
	spec := &domain.Spec{
		PriceCurrency: "USD",
		Licenses: []domain.LicenseOption{
			{
				Name:          "Basic",
				Price:         19.99,
				PriceCurrency: "USD",
				Prices: []money.StoredPrice{
					{Currency: "INR", AmountMinor: 149900, Source: "client_claimed", Revision: 999, IsActive: true},
				},
			},
		},
	}
	require.NoError(t, prepareLicensePrices(spec, nil))
	for _, p := range spec.Licenses[0].Prices {
		require.Equal(t, "owner", p.Source)
	}
}

