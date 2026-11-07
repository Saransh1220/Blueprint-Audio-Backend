package money_test

import (
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

func TestResolveDisplayCurrency(t *testing.T) {
	tests := []struct {
		name         string
		in           money.ResolveInput
		wantCurrency string
		wantSource   money.CurrencySource
	}{
		{
			name: "profile preference beats all",
			in: money.ResolveInput{
				ProfileCurrency: "INR",
				CookieCurrency:  "USD",
				CountryCode:     "US",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceProfile,
		},
		{
			name: "profile preference USD beats IN country",
			in: money.ResolveInput{
				ProfileCurrency: "usd",
				CookieCurrency:  "",
				CountryCode:     "IN",
			},
			wantCurrency: money.CurrencyUSD,
			wantSource:   money.SourceProfile,
		},
		{
			name: "invalid profile falls back to cookie",
			in: money.ResolveInput{
				ProfileCurrency: "EUR",
				CookieCurrency:  "INR",
				CountryCode:     "US",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceCookie,
		},
		{
			name: "cookie preference beats country",
			in: money.ResolveInput{
				ProfileCurrency: "",
				CookieCurrency:  "INR",
				CountryCode:     "US",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceCookie,
		},
		{
			name: "cookie USD beats IN country",
			in: money.ResolveInput{
				ProfileCurrency: "",
				CookieCurrency:  "USD",
				CountryCode:     "IN",
			},
			wantCurrency: money.CurrencyUSD,
			wantSource:   money.SourceCookie,
		},
		{
			name: "invalid cookie falls through to country IN",
			in: money.ResolveInput{
				ProfileCurrency: "",
				CookieCurrency:  "INVALID",
				CountryCode:     "IN",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceCountry,
		},
		{
			name: "country IN lower case resolves to INR",
			in: money.ResolveInput{
				CountryCode: "in",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceCountry,
		},
		{
			name: "country with whitespace resolves correctly",
			in: money.ResolveInput{
				CountryCode: "  IN  ",
			},
			wantCurrency: money.CurrencyINR,
			wantSource:   money.SourceCountry,
		},
		{
			name: "non-IN country resolves to USD",
			in: money.ResolveInput{
				CountryCode: "GB",
			},
			wantCurrency: money.CurrencyUSD,
			wantSource:   money.SourceCountry,
		},
		{
			name: "empty inputs resolve to default USD",
			in: money.ResolveInput{
				ProfileCurrency: "",
				CookieCurrency:  "",
				CountryCode:     "",
			},
			wantCurrency: money.CurrencyUSD,
			wantSource:   money.SourceDefault,
		},
		{
			name: "whitespace-only country resolves to default USD",
			in: money.ResolveInput{
				CountryCode: "   ",
			},
			wantCurrency: money.CurrencyUSD,
			wantSource:   money.SourceDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := money.ResolveDisplayCurrency(tt.in)
			assert.Equal(t, tt.wantCurrency, res.Currency)
			assert.Equal(t, tt.wantSource, res.Source)
		})
	}
}
