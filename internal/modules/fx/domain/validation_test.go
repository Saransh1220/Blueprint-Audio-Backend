package domain_test

import (
	"math/big"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRate(t *testing.T) {
	tests := []struct {
		name         string
		rate         *big.Rat
		prevRate     *big.Rat
		maxDailyMove float64
		expected     domain.RateStatus
		expectErr    bool
	}{
		{
			name:         "nil rate rejected",
			rate:         nil,
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     "",
			expectErr:    true,
		},
		{
			name:         "zero rate rejected",
			rate:         new(big.Rat),
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     "",
			expectErr:    true,
		},
		{
			name:         "negative rate rejected",
			rate:         big.NewRat(-5, 1),
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     "",
			expectErr:    true,
		},
		{
			name:         "first rate with no previous accepted rate accepted",
			rate:         big.NewRat(80, 1),
			prevRate:     nil,
			maxDailyMove: 0.20,
			expected:     domain.RateStatusAccepted,
			expectErr:    false,
		},
		{
			name:         "first rate with invalid previous rate accepted",
			rate:         big.NewRat(80, 1),
			prevRate:     big.NewRat(-10, 1),
			maxDailyMove: 0.20,
			expected:     domain.RateStatusAccepted,
			expectErr:    false,
		},
		{
			name:         "rate move within threshold (10% move on 20% limit) accepted",
			rate:         big.NewRat(88, 1), // 80 -> 88 is 10%
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     domain.RateStatusAccepted,
			expectErr:    false,
		},
		{
			name:         "rate move exactly at threshold (20% move on 20% limit) accepted",
			rate:         big.NewRat(96, 1), // 80 -> 96 is 20%
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     domain.RateStatusAccepted,
			expectErr:    false,
		},
		{
			name:         "rate jump exceeding threshold (25% increase) quarantined",
			rate:         big.NewRat(100, 1), // 80 -> 100 is 25%
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     domain.RateStatusQuarantined,
			expectErr:    false,
		},
		{
			name:         "rate drop exceeding threshold (30% drop) quarantined",
			rate:         big.NewRat(56, 1), // 80 -> 56 is 30% drop
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0.20,
			expected:     domain.RateStatusQuarantined,
			expectErr:    false,
		},
		{
			name:         "zero maxDailyMove disables quarantine",
			rate:         big.NewRat(200, 1),
			prevRate:     big.NewRat(80, 1),
			maxDailyMove: 0,
			expected:     domain.RateStatusAccepted,
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := domain.ValidateRate(tt.rate, tt.prevRate, tt.maxDailyMove)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, status)
			}
		})
	}
}
