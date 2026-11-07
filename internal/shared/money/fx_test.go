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
