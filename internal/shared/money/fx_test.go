package money_test

import (
	"math/big"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRateDecimal(t *testing.T) {
	tests := []struct {
		name     string
		rate     *big.Rat
		expected string
	}{
		{"nil rate", nil, "0.00000000"},
		{"zero rate", new(big.Rat), "0.00000000"},
		{"negative rate", big.NewRat(-1, 2), "0.00000000"},
		{"one", big.NewRat(1, 1), "1.00000000"},
		{"standard rate", big.NewRat(8312, 100), "83.12000000"},
		{"8 decimals exact", big.NewRat(8312345678, 100000000), "83.12345678"},
		{"rounding half up (rounds up)", big.NewRat(83123456785, 1000000000), "83.12345679"},
		{"rounding half up (rounds down)", big.NewRat(83123456784, 1000000000), "83.12345678"},
		{"small decimal less than 1", big.NewRat(12, 1000), "0.01200000"},
		{"very small decimal", big.NewRat(1, 100000000), "0.00000001"},
		{"smaller than 8 decimals rounds to 0", big.NewRat(1, 1000000000), "0.00000000"},
		{"large integer rate", big.NewRat(12345678, 1), "12345678.00000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := money.FormatRateDecimal(tt.rate)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseRateDecimal(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
		expected  *big.Rat
	}{
		{"valid rate", "83.12000000", false, big.NewRat(8312, 100)},
		{"valid integer", "83", false, big.NewRat(83, 1)},
		{"valid fraction", "0.012", false, big.NewRat(12, 1000)},
		{"empty string", "", true, nil},
		{"whitespace string", "   ", true, nil},
		{"invalid string", "not-a-number", true, nil},
		{"negative number", "-5.25", true, nil},
		{"zero", "0.0", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rat, err := money.ParseRateDecimal(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, rat)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rat)
				assert.Equal(t, 0, rat.Cmp(tt.expected))
			}
		})
	}
}

func TestConvertMinor(t *testing.T) {
	rate83, _ := money.ParseRateDecimal("83.12")
	rateInverse, _ := money.ParseRateDecimal("0.01203080")

	tests := []struct {
		name        string
		amountMinor int
		fromScale   int
		toScale     int
		rate        *big.Rat
		expected    int
		expectErr   bool
	}{
		{
			name:        "100 USD cents to INR paise at 83.12 (100 * 83.12 * 100 / 100 = 8312)",
			amountMinor: 100,
			fromScale:   100,
			toScale:     100,
			rate:        rate83,
			expected:    8312,
			expectErr:   false,
		},
		{
			name:        "10000 INR paise to USD cents at 0.01203080 (10000 * 0.01203080 = 120.308 -> 120)",
			amountMinor: 10000,
			fromScale:   100,
			toScale:     100,
			rate:        rateInverse,
			expected:    120,
			expectErr:   false,
		},
		{
			name:        "half-up rounding exact midpoint: 1 unit at 1.5 rate with same scale -> 2",
			amountMinor: 1,
			fromScale:   1,
			toScale:     1,
			rate:        big.NewRat(3, 2),
			expected:    2,
			expectErr:   false,
		},
		{
			name:        "half-up rounding below midpoint: 1 unit at 1.49 rate with same scale -> 1",
			amountMinor: 1,
			fromScale:   1,
			toScale:     1,
			rate:        big.NewRat(149, 100),
			expected:    1,
			expectErr:   false,
		},
		{
			name:        "zero minor amount returns zero",
			amountMinor: 0,
			fromScale:   100,
			toScale:     100,
			rate:        rate83,
			expected:    0,
			expectErr:   false,
		},
		{
			name:        "different scales: 3-scale to 2-scale (1500 minor / 1000 * 80 * 100 = 12000 minor)",
			amountMinor: 1500,
			fromScale:   1000,
			toScale:     100,
			rate:        big.NewRat(80, 1),
			expected:    12000,
			expectErr:   false,
		},
		{
			name:        "different scales: 2-scale to 0-scale (JPY-style)",
			amountMinor: 2500, // 25.00 USD
			fromScale:   100,
			toScale:     1,
			rate:        big.NewRat(150, 1), // 1 USD = 150 JPY
			expected:    3750,               // 3750 JPY
			expectErr:   false,
		},
		{
			name:        "negative amount returns error",
			amountMinor: -100,
			fromScale:   100,
			toScale:     100,
			rate:        rate83,
			expected:    0,
			expectErr:   true,
		},
		{
			name:        "zero fromScale returns error",
			amountMinor: 100,
			fromScale:   0,
			toScale:     100,
			rate:        rate83,
			expected:    0,
			expectErr:   true,
		},
		{
			name:        "negative toScale returns error",
			amountMinor: 100,
			fromScale:   100,
			toScale:     -1,
			rate:        rate83,
			expected:    0,
			expectErr:   true,
		},
		{
			name:        "nil rate returns error",
			amountMinor: 100,
			fromScale:   100,
			toScale:     100,
			rate:        nil,
			expected:    0,
			expectErr:   true,
		},
		{
			name:        "negative rate returns error",
			amountMinor: 100,
			fromScale:   100,
			toScale:     100,
			rate:        big.NewRat(-1, 1),
			expected:    0,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := money.ConvertMinor(tt.amountMinor, tt.fromScale, tt.toScale, tt.rate)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestRoundingPolicyForCurrency(t *testing.T) {
	assert.Equal(t, money.RoundingPolicyMarketplace99, money.RoundingPolicyForCurrency("USD"))
	assert.Equal(t, money.RoundingPolicyMarketplace99, money.RoundingPolicyForCurrency("usd"))
	assert.Equal(t, money.RoundingPolicyWholeUnit, money.RoundingPolicyForCurrency("INR"))
	assert.Equal(t, money.RoundingPolicyWholeUnit, money.RoundingPolicyForCurrency("inr"))
	assert.Equal(t, money.RoundingPolicyNone, money.RoundingPolicyForCurrency("EUR"))
	assert.Equal(t, money.RoundingPolicyNone, money.RoundingPolicyForCurrency(""))
}

func TestSuggestSecondaryPrice(t *testing.T) {
	usdDef, ok := money.LookupCurrency("USD")
	require.True(t, ok)
	inrDef, ok := money.LookupCurrency("INR")
	require.True(t, ok)

	rateUsdToInr := big.NewRat(83, 1)      // 1 USD = 83 INR
	rateInrToUsd := big.NewRat(1, 83)      // 1 INR = 1/83 USD
	rateFractional := big.NewRat(8325, 100) // 1 USD = 83.25 INR

	tests := []struct {
		name           string
		amountMinor    int
		fromDef        money.CurrencyDefinition
		toDef          money.CurrencyDefinition
		rate           *big.Rat
		expectedAmount int
		expectedPolicy money.RoundingPolicy
		expectErr      bool
	}{
		{
			name:           "USD to INR whole rupee rounding",
			amountMinor:    1999, // $19.99 * 83 = 1659.17 -> 165900 (₹1659.00)
			fromDef:        usdDef,
			toDef:          inrDef,
			rate:           rateUsdToInr,
			expectedAmount: 165900,
			expectedPolicy: money.RoundingPolicyWholeUnit,
			expectErr:      false,
		},
		{
			name:           "INR to USD marketplace .99 rounding",
			amountMinor:    149900, // ₹1499.00 / 83 = $18.06 -> $17.99 (1799 cents)
			fromDef:        inrDef,
			toDef:          usdDef,
			rate:           rateInrToUsd,
			expectedAmount: 1799,
			expectedPolicy: money.RoundingPolicyMarketplace99,
			expectErr:      false,
		},
		{
			name:           "INR to USD rounds up to .99",
			amountMinor:    156500, // ₹1565 / 83 = $18.855 -> $18.99 (1899 cents)
			fromDef:        inrDef,
			toDef:          usdDef,
			rate:           rateInrToUsd,
			expectedAmount: 1899,
			expectedPolicy: money.RoundingPolicyMarketplace99,
			expectErr:      false,
		},
		{
			name:           "tiny INR to USD clamps to 99 cents",
			amountMinor:    100, // ₹1.00 / 83 = $0.012 -> clamps to $0.99 (99 cents)
			fromDef:        inrDef,
			toDef:          usdDef,
			rate:           rateInrToUsd,
			expectedAmount: 99,
			expectedPolicy: money.RoundingPolicyMarketplace99,
			expectErr:      false,
		},
		{
			name:           "tiny USD to INR clamps to 1 whole rupee",
			amountMinor:    1, // 1 cent * 83 = 0.83 paise -> rounds to 100 paise (₹1.00)
			fromDef:        usdDef,
			toDef:          inrDef,
			rate:           rateUsdToInr,
			expectedAmount: 100,
			expectedPolicy: money.RoundingPolicyWholeUnit,
			expectErr:      false,
		},
		{
			name:           "identity conversion returns exact same amount",
			amountMinor:    1999,
			fromDef:        usdDef,
			toDef:          usdDef,
			rate:           big.NewRat(1, 1),
			expectedAmount: 1999,
			expectedPolicy: money.RoundingPolicyNone,
			expectErr:      false,
		},
		{
			name:           "custom currency with scale 1 and none policy",
			amountMinor:    100,
			fromDef:        usdDef,
			toDef:          money.CurrencyDefinition{Code: "JPY", MinorUnitScale: 1},
			rate:           big.NewRat(150, 1),
			expectedAmount: 150,
			expectedPolicy: money.RoundingPolicyNone,
			expectErr:      false,
		},
		{
			name:           "fractional rate conversion",
			amountMinor:    1999,
			fromDef:        usdDef,
			toDef:          inrDef,
			rate:           rateFractional,
			expectedAmount: 166400, // 19.99 * 83.25 = 1664.1675 -> 166400
			expectedPolicy: money.RoundingPolicyWholeUnit,
			expectErr:      false,
		},
		{
			name:        "zero amount returns error",
			amountMinor: 0,
			fromDef:     usdDef,
			toDef:       inrDef,
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "negative amount returns error",
			amountMinor: -50,
			fromDef:     usdDef,
			toDef:       inrDef,
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "amount overflow returns error",
			amountMinor: money.MaxAmountMinor + 1,
			fromDef:     usdDef,
			toDef:       inrDef,
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "empty fromDef code returns error",
			amountMinor: 100,
			fromDef:     money.CurrencyDefinition{Code: ""},
			toDef:       inrDef,
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "empty toDef code returns error",
			amountMinor: 100,
			fromDef:     usdDef,
			toDef:       money.CurrencyDefinition{Code: ""},
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "invalid scale returns error",
			amountMinor: 100,
			fromDef:     money.CurrencyDefinition{Code: "USD", MinorUnitScale: 0},
			toDef:       inrDef,
			rate:        rateUsdToInr,
			expectErr:   true,
		},
		{
			name:        "nil rate returns error",
			amountMinor: 100,
			fromDef:     usdDef,
			toDef:       inrDef,
			rate:        nil,
			expectErr:   true,
		},
		{
			name:        "negative rate returns error",
			amountMinor: 100,
			fromDef:     usdDef,
			toDef:       inrDef,
			rate:        big.NewRat(-1, 1),
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, policy, err := money.SuggestSecondaryPrice(tt.amountMinor, tt.fromDef, tt.toDef, tt.rate)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedAmount, got)
				assert.Equal(t, tt.expectedPolicy, policy)
			}
		})
	}
}
