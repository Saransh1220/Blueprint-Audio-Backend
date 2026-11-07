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
