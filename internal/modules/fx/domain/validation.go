package domain

import (
	"math/big"
)

// ValidateRate applies the sanity check against the previous accepted rate.
// If the relative move exceeds maxDailyMove (e.g. 0.20 for 20%), the rate is quarantined.
func ValidateRate(rate *big.Rat, prevRate *big.Rat, maxDailyMove float64) (RateStatus, error) {
	if rate == nil || rate.Sign() <= 0 {
		return "", ErrRateRejected
	}

	if prevRate == nil || prevRate.Sign() <= 0 || maxDailyMove <= 0 {
		return RateStatusAccepted, nil
	}

	diff := new(big.Rat).Sub(rate, prevRate)
	diff.Abs(diff)

	relMove := new(big.Rat).Quo(diff, prevRate)
	threshold := new(big.Rat).SetFloat64(maxDailyMove)
	if threshold == nil {
		threshold = big.NewRat(2, 10) // default 20%
	}

	if relMove.Cmp(threshold) > 0 {
		return RateStatusQuarantined, nil
	}
	return RateStatusAccepted, nil
}
