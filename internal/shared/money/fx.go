package money

import (
	"errors"
	"math/big"
	"strings"
)

var (
	ErrInvalidRate     = errors.New("invalid exchange rate: must be positive and non-zero")
	ErrInvalidAmount   = errors.New("invalid amount: cannot be negative")
	ErrInvalidScale    = errors.New("invalid scale: must be greater than zero")
	ErrOverflowAmount  = errors.New("converted amount exceeds integer bounds")
)

const RateDecimalPlaces = 8

// FormatRateDecimal formats a *big.Rat as a fixed-point decimal string with 8 decimal places.
func FormatRateDecimal(rate *big.Rat) string {
	if rate == nil || rate.Sign() <= 0 {
		return "0.00000000"
	}
	// Scale by 10^8
	scaleFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(RateDecimalPlaces)), nil)
	scaledRat := new(big.Rat).Mul(rate, new(big.Rat).SetInt(scaleFactor))

	num := new(big.Int).Set(scaledRat.Num())
	denom := new(big.Int).Set(scaledRat.Denom())

	q, rem := new(big.Int).QuoRem(num, denom, new(big.Int))
	twiceRem := new(big.Int).Mul(rem, big.NewInt(2))
	if twiceRem.Cmp(denom) >= 0 {
		q.Add(q, big.NewInt(1))
	}

	qStr := q.String()
	if len(qStr) <= RateDecimalPlaces {
		zerosNeeded := RateDecimalPlaces - len(qStr) + 1
		qStr = strings.Repeat("0", zerosNeeded) + qStr
	}
	splitIdx := len(qStr) - RateDecimalPlaces
	integerPart := qStr[:splitIdx]
	fractionalPart := qStr[splitIdx:]
	return integerPart + "." + fractionalPart
}

// ParseRateDecimal parses a decimal string (e.g. "83.12000000" or "0.012") into *big.Rat.
func ParseRateDecimal(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrInvalidRate
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, ErrInvalidRate
	}
	return r, nil
}

// ConvertMinor converts an amount in minor units from one currency to another using exact rational arithmetic.
// Rounding is half-up (away from zero for positive amounts).
func ConvertMinor(amountMinor int, fromScale, toScale int, rate *big.Rat) (int, error) {
	if amountMinor < 0 {
		return 0, ErrInvalidAmount
	}
	if fromScale <= 0 || toScale <= 0 {
		return 0, ErrInvalidScale
	}
	if rate == nil || rate.Sign() <= 0 {
		return 0, ErrInvalidRate
	}
	if amountMinor == 0 {
		return 0, nil
	}

	minorRat := new(big.Rat).SetInt64(int64(amountMinor))
	scaled := new(big.Rat).Mul(minorRat, rate)
	scaled.Mul(scaled, new(big.Rat).SetInt64(int64(toScale)))
	scaled.Quo(scaled, new(big.Rat).SetInt64(int64(fromScale)))

	num := new(big.Int).Set(scaled.Num())
	denom := new(big.Int).Set(scaled.Denom())

	q, rem := new(big.Int).QuoRem(num, denom, new(big.Int))
	twiceRem := new(big.Int).Mul(rem, big.NewInt(2))
	if twiceRem.Cmp(denom) >= 0 {
		q.Add(q, big.NewInt(1))
	}

	if !q.IsInt64() {
		return 0, ErrOverflowAmount
	}
	res := q.Int64()
	const maxInt = int64(^uint(0) >> 1)
	if res > maxInt {
		return 0, ErrOverflowAmount
	}
	return int(res), nil
}
