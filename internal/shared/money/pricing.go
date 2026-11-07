package money

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Constants for bounds and limits.
const (
	MaxAmountMinor = 2147483647 // Max 32-bit signed integer (PostgreSQL INTEGER limit)
)

var (
	ErrInvalidCurrency      = errors.New("invalid or unsupported currency")
	ErrNegativeAmount       = errors.New("price amount cannot be negative")
	ErrAmountOverflow       = errors.New("price amount exceeds maximum supported minor units")
	ErrPriceNotFound        = errors.New("price row not found")
	ErrPriceUnavailable     = errors.New("price unavailable for selected currency")
	ErrStalePrice           = errors.New("price has changed since offer was displayed")
	ErrLicenseOptionNotFound = errors.New("license option not found")
)

// StoredPrice represents a canonical price stored in license_option_prices.
type StoredPrice struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	LicenseOptionID uuid.UUID  `json:"license_option_id" db:"license_option_id"`
	Currency        string     `json:"currency" db:"currency"`
	AmountMinor     int        `json:"amount_minor" db:"amount_minor"`
	AmountMajor     float64    `json:"amount_major" db:"amount_major"`
	Source          string     `json:"source" db:"source"`
	FxRateSnapshot  *float64   `json:"fx_rate_snapshot,omitempty" db:"fx_rate_snapshot"`
	Revision        int64      `json:"revision" db:"revision"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// Validate ensures monetary invariants for a StoredPrice.
func (p *StoredPrice) Validate() error {
	curr := strings.ToUpper(strings.TrimSpace(p.Currency))
	if curr != CurrencyINR && curr != CurrencyUSD {
		return ErrInvalidCurrency
	}
	if p.AmountMinor < 0 {
		return ErrNegativeAmount
	}
	if p.AmountMinor > MaxAmountMinor {
		return ErrAmountOverflow
	}
	if p.Revision < 1 {
		p.Revision = 1
	}
	// Derive amount_major exactly from minor units
	p.AmountMajor = float64(p.AmountMinor) / 100.0
	p.Currency = curr
	return nil
}

type SelectionStatus string

const (
	SelectionExact          SelectionStatus = "exact"
	SelectionNativeFallback SelectionStatus = "native_fallback"
	SelectionUnavailable    SelectionStatus = "unavailable"
)

// SelectedPriceOffer contains the resolved offer for a requested currency.
type SelectedPriceOffer struct {
	PriceID             *uuid.UUID      `json:"price_id,omitempty"`
	Revision            int64           `json:"revision"`
	AmountMinor         int             `json:"amount_minor"`
	AmountMajor         float64         `json:"amount_major"`
	Currency            string          `json:"currency"`
	Selection           SelectionStatus `json:"selection"`
	FallbackReason      string          `json:"fallback_reason,omitempty"`
	IsPurchasable       bool            `json:"is_purchasable"`
	AvailableCurrencies []string        `json:"available_currencies"`
}

// SelectPriceOffer evaluates available active stored prices against requested and native currencies.
// It is a pure function: no network, no database, no FX math, no marketplace rounding.
func SelectPriceOffer(prices []StoredPrice, nativeCurrency string, requestedCurrency string) SelectedPriceOffer {
	reqCode := strings.ToUpper(strings.TrimSpace(requestedCurrency))
	natCode := strings.ToUpper(strings.TrimSpace(nativeCurrency))
	if natCode == "" {
		natCode = CurrencyINR
	}

	available := make([]string, 0, 2)
	activePrices := make(map[string]StoredPrice)
	for _, p := range prices {
		if !p.IsActive {
			continue
		}
		curr := strings.ToUpper(strings.TrimSpace(p.Currency))
		activePrices[curr] = p
		available = append(available, curr)
	}

	// If reqCode is not supported (only INR and USD are supported commerce currencies)
	if _, ok := LookupCurrency(reqCode); !ok {
		return SelectedPriceOffer{
			Currency:            reqCode,
			Selection:           SelectionUnavailable,
			FallbackReason:      fmt.Sprintf("Currency %q is not supported", reqCode),
			IsPurchasable:       false,
			AvailableCurrencies: available,
		}
	}

	// 1. Exact match for requested currency
	if price, found := activePrices[reqCode]; found {
		return SelectedPriceOffer{
			PriceID:             &price.ID,
			Revision:            price.Revision,
			AmountMinor:         price.AmountMinor,
			AmountMajor:         float64(price.AmountMinor) / 100.0,
			Currency:            price.Currency,
			Selection:           SelectionExact,
			IsPurchasable:       true,
			AvailableCurrencies: available,
		}
	}

	// 2. Fallback to native currency if available
	if nativePrice, found := activePrices[natCode]; found {
		return SelectedPriceOffer{
			PriceID:             &nativePrice.ID,
			Revision:            nativePrice.Revision,
			AmountMinor:         nativePrice.AmountMinor,
			AmountMajor:         float64(nativePrice.AmountMinor) / 100.0,
			Currency:            nativePrice.Currency,
			Selection:           SelectionNativeFallback,
			FallbackReason:      fmt.Sprintf("%s pricing unavailable. Available in %s", reqCode, nativePrice.Currency),
			IsPurchasable:       true,
			AvailableCurrencies: available,
		}
	}

	// 3. Unavailable
	return SelectedPriceOffer{
		Currency:            reqCode,
		Selection:           SelectionUnavailable,
		FallbackReason:      "Pricing unavailable for this item",
		IsPurchasable:       false,
		AvailableCurrencies: available,
	}
}

// MajorToMinor safely quantizes a major decimal amount to minor units with exact rounding.
func MajorToMinor(amount float64) (int, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, errors.New("invalid decimal amount")
	}
	if amount < 0 {
		return 0, ErrNegativeAmount
	}
	minorF := math.Round(amount * 100)
	if minorF > float64(MaxAmountMinor) {
		return 0, ErrAmountOverflow
	}
	return int(minorF), nil
}
