package money_test

import (
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/assert"
)

func TestMoneyBasicCreation(t *testing.T) {
	inr := money.INRFromMajor(99.50)
	assert.Equal(t, 9950, inr.AmountMinor)
	assert.Equal(t, 99.50, inr.AmountMajor)
	assert.Equal(t, money.CurrencyINR, inr.Currency)

	usd := money.USDFromMajor(19.99)
	assert.Equal(t, 1999, usd.AmountMinor)
	assert.Equal(t, 19.99, usd.AmountMajor)
	assert.Equal(t, money.CurrencyUSD, usd.Currency)

	fromUSD := money.FromMajor(15.00, "usd")
	assert.Equal(t, money.CurrencyUSD, fromUSD.Currency)
	assert.Equal(t, 1500, fromUSD.AmountMinor)

	fromINR := money.FromMajor(1000.00, "INR")
	assert.Equal(t, money.CurrencyINR, fromINR.Currency)
	assert.Equal(t, 100000, fromINR.AmountMinor)

	fromEmpty := money.FromMajor(50.00, "")
	assert.Equal(t, money.CurrencyINR, fromEmpty.Currency)
}

func TestResolveCurrencyFromRequest(t *testing.T) {
	reqIN := httptest.NewRequest("GET", "/", nil)
	reqIN.Header.Set("X-Country-Code", "IN")
	assert.Equal(t, money.CurrencyINR, money.ResolveCurrencyFromRequest(reqIN))

	reqCF := httptest.NewRequest("GET", "/", nil)
	reqCF.Header.Set("CF-IPCountry", "in")
	assert.Equal(t, money.CurrencyINR, money.ResolveCurrencyFromRequest(reqCF))

	reqUS := httptest.NewRequest("GET", "/", nil)
	reqUS.Header.Set("CF-IPCountry", "US")
	assert.Equal(t, money.CurrencyUSD, money.ResolveCurrencyFromRequest(reqUS))

	reqDefault := httptest.NewRequest("GET", "/", nil)
	assert.Equal(t, money.CurrencyUSD, money.ResolveCurrencyFromRequest(reqDefault))
}
