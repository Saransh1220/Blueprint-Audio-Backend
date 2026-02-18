package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SpecAnalytics represents analytics data for a spec
type SpecAnalytics struct {
	SpecID             uuid.UUID `json:"spec_id" db:"spec_id"`
	PlayCount          int       `json:"play_count" db:"play_count"`
	FavoriteCount      int       `json:"favorite_count" db:"favorite_count"`
	FreeDownloadCount  int       `json:"free_download_count" db:"free_download_count"`
	TotalPurchaseCount int       `json:"total_purchase_count" db:"total_purchase_count"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
	// License-specific purchases calculated on-the-fly, not stored in DB
	LicensePurchases map[string]int `json:"license_purchases,omitempty" db:"-"`
	IsFavorited      bool           `json:"is_favorited" db:"-"`
}

// UserFavorite represents a user's favorite spec
type UserFavorite struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	SpecID    uuid.UUID `json:"spec_id" db:"spec_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type DailyStat struct {
	Date  string `json:"date" db:"date"`
	Count int    `json:"count" db:"count"`
}

type DailyRevenueStat struct {
	Date    string  `json:"date" db:"date"`
	Revenue float64 `json:"revenue" db:"revenue"`
}

type TopSpecStat struct {
	SpecID    uuid.UUID `json:"spec_id" db:"spec_id"`
	Title     string    `json:"title" db:"title"`
	Plays     int       `json:"plays" db:"plays"`
	Downloads int       `json:"downloads" db:"downloads"`
	Revenue   float64   `json:"revenue" db:"revenue"`
}

type PublicAnalytics struct {
	PlayCount          int  `json:"play_count"`
	FavoriteCount      int  `json:"favorite_count"`
	TotalDownloadCount int  `json:"total_download_count"`
	IsFavorited        bool `json:"is_favorited"`
}

type ProducerAnalytics struct {
	PlayCount          int            `json:"play_count"`
	FavoriteCount      int            `json:"favorite_count"`
	FreeDownloadCount  int            `json:"free_download_count"`
	TotalPurchaseCount int            `json:"total_purchase_count"`
	LicensePurchases   map[string]int `json:"license_purchases"`
}

type AnalyticsOverviewResponse struct {
	TotalPlays       int                `json:"total_plays"`
	TotalFavorites   int                `json:"total_favorites"`
	TotalRevenue     float64            `json:"total_revenue"`
	TotalDownloads   int                `json:"total_downloads"`
	PlaysByDay       []DailyStat        `json:"plays_by_day"`
	DownloadsByDay   []DailyStat        `json:"downloads_by_day"`
	RevenueByDay     []DailyRevenueStat `json:"revenue_by_day"`
	TopSpecs         []TopSpecStat      `json:"top_specs"`
	RevenueByLicense map[string]float64 `json:"revenue_by_license"`
}

// AnalyticsRepository defines the contract for analytics data access
type AnalyticsRepository interface {
	GetSpecAnalytics(ctx context.Context, specID uuid.UUID) (*SpecAnalytics, error)
	IncrementPlayCount(ctx context.Context, specID uuid.UUID) error
	IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error

	AddFavorite(ctx context.Context, userID, specID uuid.UUID) error
	RemoveFavorite(ctx context.Context, userID, specID uuid.UUID) error
	IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error)

	GetLicensePurchaseCounts(ctx context.Context, specID uuid.UUID) (map[string]int, error)

	// Overview Analytics
	GetTotalPlays(ctx context.Context, producerID uuid.UUID, days int) (int, error)
	GetTotalFavorites(ctx context.Context, producerID uuid.UUID, days int) (int, error)
	GetTotalDownloads(ctx context.Context, producerID uuid.UUID, days int) (int, error)
	GetTotalRevenue(ctx context.Context, producerID uuid.UUID, days int) (float64, error)
	GetRevenueByLicenseGlobal(ctx context.Context, producerID uuid.UUID, days int) (map[string]float64, error)
	GetPlaysByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyStat, error)
	GetDownloadsByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyStat, error)
	GetRevenueByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyRevenueStat, error)
	GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]TopSpecStat, error)
}
