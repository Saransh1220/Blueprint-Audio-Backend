package money

import (
	"math"
	"strings"
)

const (
	CurrencyINR = "INR"
	CurrencyUSD = "USD"
)

type Money struct {
	AmountMinor int     `json:"amount_minor"`
	AmountMajor float64 `json:"amount_major"`
	Currency    string  `json:"currency"`
}

type CurrencySource string

const (
	SourceProfile CurrencySource = "profile"
	SourceCookie  CurrencySource = "cookie"
	SourceCountry CurrencySource = "country"
	SourceDefault CurrencySource = "default"
)

type ResolveInput struct {
	ProfileCurrency string
	CookieCurrency  string
	CountryCode     string
}

type ResolutionResult struct {
	Currency string         `json:"currency"`
	Source   CurrencySource `json:"source"`
}

// ResolveDisplayCurrency resolves buyer display currency and origin source using D2 precedence:
// 1. Profile preference (if non-empty and valid supported currency)
// 2. Cookie preference (if non-empty and valid supported currency)
// 3. Country code ("IN" -> INR, any other non-empty country -> USD)
// 4. Fallback -> USD (default source)
func ResolveDisplayCurrency(in ResolveInput) ResolutionResult {
	if def, ok := LookupCurrency(in.ProfileCurrency); ok {
		return ResolutionResult{Currency: def.Code, Source: SourceProfile}
	}
	if def, ok := LookupCurrency(in.CookieCurrency); ok {
		return ResolutionResult{Currency: def.Code, Source: SourceCookie}
	}
	country := strings.ToUpper(strings.TrimSpace(in.CountryCode))
	if country == "IN" {
		return ResolutionResult{Currency: CurrencyINR, Source: SourceCountry}
	}
	if country != "" {
		return ResolutionResult{Currency: CurrencyUSD, Source: SourceCountry}
	}
	return ResolutionResult{Currency: CurrencyUSD, Source: SourceDefault}
}

func INRFromMajor(amount float64) Money {
	minor := int(math.Round(amount * 100))
	return Money{AmountMinor: minor, AmountMajor: float64(minor) / 100, Currency: CurrencyINR}
}

func USDFromMajor(amount float64) Money {
	minor := int(math.Round(amount * 100))
	return Money{AmountMinor: minor, AmountMajor: float64(minor) / 100, Currency: CurrencyUSD}
}

func FromMajor(amount float64, currency string) Money {
	if strings.ToUpper(strings.TrimSpace(currency)) == CurrencyUSD {
		return USDFromMajor(amount)
	}
	return INRFromMajor(amount)
}
