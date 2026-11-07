package money

import (
	"math"
	"net/http"
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

func ResolveCurrencyFromRequest(r *http.Request) string {
	country := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
	if country == "" {
		country = strings.ToUpper(strings.TrimSpace(r.Header.Get("X-Country-Code")))
	}
	if country == "IN" {
		return CurrencyINR
	}
	return CurrencyUSD
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
