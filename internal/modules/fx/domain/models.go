package domain

import (
	"context"
	"math/big"
	"time"
)

type RateStatus string

const (
	RateStatusAccepted    RateStatus = "accepted"
	RateStatusQuarantined RateStatus = "quarantined"
)

type Rate struct {
	ID          int64      `json:"id" db:"id"`
	Base        string     `json:"base" db:"base"`
	Quote       string     `json:"quote" db:"quote"`
	Rate        *big.Rat   `json:"-" db:"-"`
	RateDecimal string     `json:"rate" db:"rate"`
	Source      string     `json:"source" db:"source"`
	AsOf        time.Time  `json:"as_of" db:"as_of"`
	FetchedAt   time.Time  `json:"fetched_at" db:"fetched_at"`
	Status      RateStatus `json:"status" db:"status"`
	Stale       bool       `json:"stale" db:"-"`
	Unusable    bool       `json:"unusable" db:"-"`
}

func (r *Rate) GetBigRat() *big.Rat {
	if r == nil {
		return nil
	}
	return r.Rate
}

type HistoryFilter struct {
	Base   string     `json:"base"`
	Quote  string     `json:"quote"`
	Status RateStatus `json:"status"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type ProviderRates struct {
	Base   string              `json:"base"`
	AsOf   time.Time           `json:"as_of"`
	Rates  map[string]*big.Rat `json:"rates"`
	Source string              `json:"source"`
}

type RateRepository interface {
	Append(ctx context.Context, rate *Rate) error
	AppendBatch(ctx context.Context, rates []*Rate) error
	Latest(ctx context.Context, base, quote string) (*Rate, error)
	History(ctx context.Context, filter HistoryFilter) ([]*Rate, int, error)
	LatestPerPair(ctx context.Context) ([]*Rate, error)
}

type LockManager interface {
	TryLock(ctx context.Context, key string) (unlock func() error, acquired bool, err error)
}

type RateProvider interface {
	Name() string
	Fetch(ctx context.Context, base string, quotes []string) (*ProviderRates, error)
}
