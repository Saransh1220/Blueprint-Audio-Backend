package money

import (
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
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

func USDFromINRMajor(amount float64) Money {
	rate := usdPerINR()
	raw := amount * rate
	rounded := roundMarketplaceUSD(raw)
	minor := int(math.Round(rounded * 100))
	return Money{AmountMinor: minor, AmountMajor: float64(minor) / 100, Currency: CurrencyUSD}
}

func INRFromUSDMajor(amount float64) Money {
	rate := usdPerINR()
	raw := amount / rate
	rounded := roundMarketplaceINR(raw)
	minor := int(math.Round(rounded * 100))
	return Money{AmountMinor: minor, AmountMajor: float64(minor) / 100, Currency: CurrencyINR}
}

// DisplayFromINRMajor is kept for backward compatibility.
// Prefer DisplayPrice for new code — it handles both stored currencies.
func DisplayFromINRMajor(amount float64, currency string) Money {
	if strings.ToUpper(currency) == CurrencyINR {
		return INRFromMajor(amount)
	}
	return USDFromINRMajor(amount)
}

// DisplayPrice converts a stored price to the desired display currency.
// storedCurrency is the currency the price was originally entered in (INR or USD).
// displayCurrency is what the user should see (resolved from their location).
func DisplayPrice(amount float64, storedCurrency, displayCurrency string) Money {
	stored := strings.ToUpper(strings.TrimSpace(storedCurrency))
	display := strings.ToUpper(strings.TrimSpace(displayCurrency))

	// Normalize empty stored currency to INR (legacy records)
	if stored == "" {
		stored = CurrencyINR
	}

	if stored == display {
		// No conversion needed — return as-is in the stored currency
		if stored == CurrencyINR {
			return INRFromMajor(amount)
		}
		return USDFromMajor(amount)
	}

	if stored == CurrencyINR && display == CurrencyUSD {
		return USDFromINRMajor(amount)
	}

	// stored == USD, display == INR
	return INRFromUSDMajor(amount)
}

func usdPerINR() float64 {
	if value := strings.TrimSpace(getenv("INR_USD_RATE")); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
		if err != nil {
			log.Printf("usdPerINR(): invalid INR_USD_RATE=%q parse_error=%v; using fallback 0.012", value, err)
		} else {
			log.Printf("usdPerINR(): invalid INR_USD_RATE=%q parsed_rate=%f; using fallback 0.012", value, parsed)
		}
	}
	return 0.012
}

func roundMarketplaceUSD(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value < 1 {
		return 0.99
	}
	whole := math.Floor(value)
	return whole + 0.99
}

func roundMarketplaceINR(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round(value)
}

var getenv = os.Getenv
