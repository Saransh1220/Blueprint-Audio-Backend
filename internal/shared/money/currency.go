package money

import "strings"

// CurrencyDefinition describes representation, not FX conversion or price policy.
// MinorUnitScale is the divisor from integer minor units to major units.
type CurrencyDefinition struct {
	Code           string  `json:"code"`
	Symbol         string  `json:"symbol"`
	MinorUnitScale int     `json:"minor_unit_scale"`
	Locale         string  `json:"locale"`
	Gateway        *string `json:"gateway"`
}

// SupportedCurrencies returns a copy so callers cannot mutate shared definitions.
func SupportedCurrencies() []CurrencyDefinition {
	return []CurrencyDefinition{
		{Code: CurrencyINR, Symbol: "₹", MinorUnitScale: 100, Locale: "en-IN"},
		{Code: CurrencyUSD, Symbol: "$", MinorUnitScale: 100, Locale: "en-US"},
	}
}

func LookupCurrency(code string) (CurrencyDefinition, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, currency := range SupportedCurrencies() {
		if currency.Code == code {
			return currency, true
		}
	}
	return CurrencyDefinition{}, false
}
