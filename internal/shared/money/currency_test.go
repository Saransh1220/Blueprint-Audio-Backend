package money

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCurrencyDefinitions(t *testing.T) {
	first := SupportedCurrencies()
	first[0].Code = "CHANGED"
	currency, ok := LookupCurrency(" inr ")
	require.True(t, ok)
	require.Equal(t, "INR", currency.Code)
	require.Equal(t, 100, currency.MinorUnitScale)
	_, ok = LookupCurrency("EUR")
	require.False(t, ok)
}
