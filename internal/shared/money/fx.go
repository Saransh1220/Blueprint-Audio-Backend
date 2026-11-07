package money

import (
	"errors"
	"math/big"
	"strings"
)

var (
	ErrInvalidRate    = errors.New("invalid exchange rate: must be positive and non-zero")
	ErrInvalidAmount  = errors.New("invalid amount: cannot be negative")
	ErrInvalidScale   = errors.New("invalid scale: must be greater than zero")
	ErrOverflowAmount = errors.New("converted amount exceeds integer bounds")
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

type RoundingPolicy string

const (
	RoundingPolicyMarketplace99 RoundingPolicy = "marketplace_99"
	RoundingPolicyWholeUnit     RoundingPolicy = "whole_unit"
	RoundingPolicyNone          RoundingPolicy = "none"
)

var defaultRoundingPolicies = map[string]RoundingPolicy{
	CurrencyUSD: RoundingPolicyMarketplace99,
	CurrencyINR: RoundingPolicyWholeUnit,
}

// RoundingPolicyForCurrency returns the configured rounding policy for a currency code.
func RoundingPolicyForCurrency(code string) RoundingPolicy {
	if p, ok := defaultRoundingPolicies[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return p
	}
	return RoundingPolicyNone
}

// SuggestSecondaryPrice calculates a suggested price in toDef for an amount in fromDef,
// applying the target currency's rounding policy (e.g. .99 for USD, nearest whole rupee for INR).
func SuggestSecondaryPrice(
	amountMinor int,
	fromDef, toDef CurrencyDefinition,
	rate *big.Rat,
) (int, RoundingPolicy, error) {
	if amountMinor <= 0 {
		return 0, RoundingPolicyNone, ErrInvalidAmount
	}
	if amountMinor > MaxAmountMinor {
		return 0, RoundingPolicyNone, ErrAmountOverflow
	}
	if fromDef.Code == "" || toDef.Code == "" {
		return 0, RoundingPolicyNone, ErrInvalidCurrency
	}
	if fromDef.MinorUnitScale <= 0 || toDef.MinorUnitScale <= 0 {
		return 0, RoundingPolicyNone, ErrInvalidScale
	}
	if rate == nil || rate.Sign() <= 0 {
		return 0, RoundingPolicyNone, ErrInvalidRate
	}

	// Identity conversion if currencies are the same
	if strings.EqualFold(fromDef.Code, toDef.Code) {
		return amountMinor, RoundingPolicyNone, nil
	}

	policy := RoundingPolicyForCurrency(toDef.Code)

	// Calculate converted rational in toDef minor units:
	// ratToMinor = (amountMinor / fromScale) * rate * toScale
	minorRat := new(big.Rat).SetInt64(int64(amountMinor))
	scaled := new(big.Rat).Mul(minorRat, rate)
	scaled.Mul(scaled, new(big.Rat).SetInt64(int64(toDef.MinorUnitScale)))
	scaled.Quo(scaled, new(big.Rat).SetInt64(int64(fromDef.MinorUnitScale)))

	var suggested int
	switch policy {
	case RoundingPolicyMarketplace99:
		scaleInt64 := int64(toDef.MinorUnitScale)
		if scaleInt64 <= 1 {
			suggested = roundRatHalfUp(scaled)
			break
		}
		// Target is k*scale + (scale - 1)
		ratPlusOne := new(big.Rat).Add(scaled, new(big.Rat).SetInt64(1))
		ratUnits := new(big.Rat).Quo(ratPlusOne, new(big.Rat).SetInt64(scaleInt64))
		units := roundRatHalfUp(ratUnits)
		if units < 1 {
			units = 1
		}
		res := int64(units)*scaleInt64 - 1
		if res > int64(MaxAmountMinor) {
			return 0, policy, ErrAmountOverflow
		}
		suggested = int(res)

	case RoundingPolicyWholeUnit:
		scaleInt64 := int64(toDef.MinorUnitScale)
		if scaleInt64 <= 1 {
			suggested = roundRatHalfUp(scaled)
			break
		}
		// Target is k*scale (nearest whole unit)
		ratUnits := new(big.Rat).Quo(scaled, new(big.Rat).SetInt64(scaleInt64))
		units := roundRatHalfUp(ratUnits)
		if units < 1 {
			units = 1
		}
		res := int64(units) * scaleInt64
		if res > int64(MaxAmountMinor) {
			return 0, policy, ErrAmountOverflow
		}
		suggested = int(res)

	default: // RoundingPolicyNone
		res := roundRatHalfUp(scaled)
		if res < 1 {
			res = 1
		}
		if res > MaxAmountMinor {
			return 0, policy, ErrAmountOverflow
		}
		suggested = res
	}

	return suggested, policy, nil
}

func roundRatHalfUp(r *big.Rat) int {
	num := new(big.Int).Set(r.Num())
	denom := new(big.Int).Set(r.Denom())
	q, rem := new(big.Int).QuoRem(num, denom, new(big.Int))
	twiceRem := new(big.Int).Mul(rem, big.NewInt(2))
	if twiceRem.Cmp(denom) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return MaxAmountMinor + 1
	}
	res := q.Int64()
	if res > int64(MaxAmountMinor) {
		return MaxAmountMinor + 1
	}
	return int(res)
}
