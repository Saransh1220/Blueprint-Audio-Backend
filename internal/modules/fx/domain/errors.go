package domain

import "errors"

var (
	ErrNoRate          = errors.New("exchange rate not found")
	ErrStaleRate       = errors.New("exchange rate is stale")
	ErrUnusableRate    = errors.New("exchange rate exceeds maximum age and is unusable")
	ErrRateRejected    = errors.New("exchange rate was rejected")
	ErrRateQuarantined = errors.New("exchange rate was quarantined")
	ErrLockNotAcquired = errors.New("fx refresh lock could not be acquired")
	ErrInvalidPair     = errors.New("invalid currency pair")
	ErrCooldownActive  = errors.New("refresh cooldown active: please wait before refreshing again")
)
