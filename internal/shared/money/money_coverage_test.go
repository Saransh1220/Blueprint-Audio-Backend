package money

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMoneyConversionBranches(t *testing.T) {
	oldGetenv := getenv
	defer func() { getenv = oldGetenv }()

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Country-Code", " in ")
	assert.Equal(t, CurrencyINR, ResolveCurrencyFromRequest(req))
	req.Header.Set("CF-IPCountry", "US")
	assert.Equal(t, CurrencyUSD, ResolveCurrencyFromRequest(req))

	assert.Equal(t, Money{AmountMinor: 123, AmountMajor: 1.23, Currency: CurrencyUSD}, USDFromMajor(1.234))
	assert.Equal(t, CurrencyINR, DisplayFromINRMajor(10, "inr").Currency)

	getenv = func(string) string { return "0.01" }
	assert.Equal(t, CurrencyUSD, DisplayFromINRMajor(10, "usd").Currency)
	assert.Equal(t, CurrencyINR, DisplayPrice(10, "", CurrencyINR).Currency)
	assert.Equal(t, CurrencyUSD, DisplayPrice(10, CurrencyUSD, CurrencyUSD).Currency)
	assert.Equal(t, CurrencyUSD, DisplayPrice(100, CurrencyINR, CurrencyUSD).Currency)
	assert.Equal(t, CurrencyINR, DisplayPrice(1, CurrencyUSD, CurrencyINR).Currency)

	assert.Equal(t, 0.0, roundMarketplaceUSD(0))
	assert.Equal(t, 0.99, roundMarketplaceUSD(0.5))
	assert.Equal(t, 3.99, roundMarketplaceUSD(3.2))
	assert.Equal(t, 0.0, roundMarketplaceINR(-1))
	assert.Equal(t, 3.0, roundMarketplaceINR(3.2))

	assert.Equal(t, 0.01, usdPerINR())
	getenv = func(string) string { return "" }
	assert.Equal(t, 0.012, usdPerINR())
}
