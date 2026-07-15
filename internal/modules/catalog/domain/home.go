package domain

import (
	"encoding/json"
	"time"
)

const (
	HomeSectionFeatured    = "featured"
	HomeSectionTrending    = "trending"
	HomeSectionTopCharts   = "top_charts"
	HomeSectionNewReleases = "new_releases"

	HomePeriod24H = "24h"
	HomePeriod7D  = "7d"
	HomePeriod30D = "30d"
)

type HomepageParams struct {
	Limit    int
	Sections []string
	Period   string
}

type HomepageStats struct {
	TotalLiveBeats int `json:"total_live_beats" db:"total_live_beats"`
	NewReleases7D  int `json:"new_releases_7d" db:"new_releases_7d"`
	TotalProducers int `json:"total_producers" db:"total_producers"`
}

type BeatRankingMetrics struct {
	Plays           int     `json:"plays"`
	UniqueListeners int     `json:"unique_listeners,omitempty"`
	Favorites       int     `json:"favorites"`
	Downloads       int     `json:"downloads"`
	Purchases       int     `json:"purchases"`
	Revenue         float64 `json:"revenue"`
}

type RankedSpec struct {
	Spec         Spec
	Rank         int
	Score        *float64
	Movement     string
	Metrics      *BeatRankingMetrics
	CalculatedAt time.Time
}

type RankingFreshness struct {
	Count        int
	CalculatedAt time.Time
}

type RankingRow struct {
	Spec         Spec
	Rank         int
	Score        float64
	PreviousRank *int
	MetricsJSON  json.RawMessage
	CalculatedAt time.Time
}

type HomepageSection struct {
	Title  string
	Source string
	Period string
	Items  []RankedSpec
}

type HomepageData struct {
	GeneratedAt     time.Time
	CacheTTLSeconds int
	Stats           HomepageStats
	Sections        map[string]HomepageSection
}
