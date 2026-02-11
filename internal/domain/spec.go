package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Category string
type LicenseType string

const (
	CategoryBeat   Category = "beat"
	CategorySample Category = "sample"
)

const (
	LicenseBasic     LicenseType = "Basic"
	LicensePremium   LicenseType = "Premium"
	LicenseTrackout  LicenseType = "Trackout"
	LicenseUnlimited LicenseType = "Unlimited"
)

// Spec represents a beat or sample package.
type Spec struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ProducerID     uuid.UUID  `json:"producer_id" db:"producer_id"`
	ProducerName   string     `json:"producer_name" db:"producer_name"`
	Title          string     `json:"title" db:"title"`
	Category       Category   `json:"category" db:"category"`
	Type           string     `json:"type" db:"type"` // e.g., WAV, STEMS, PACK
	BPM            int        `json:"bpm" db:"bpm"`
	Key            string     `json:"key" db:"key"`
	ImageUrl       string     `json:"image_url" db:"image_url"`
	PreviewUrl     string     `json:"preview_url" db:"preview_url"`
	WavUrl         *string    `json:"wav_url,omitempty" db:"wav_url"`
	StemsUrl       *string    `json:"stems_url,omitempty" db:"stems_url"`
	BasePrice      float64    `json:"price" db:"base_price"`
	Description    string     `json:"description" db:"description"`
	Duration       int        `json:"duration" db:"duration"`                 // Audio duration in seconds
	FreeMp3Enabled bool       `json:"free_mp3_enabled" db:"free_mp3_enabled"` // Enable free MP3 downloads
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	IsDeleted      bool       `json:"is_deleted" db:"is_deleted"`

	// Relations
	Licenses  []LicenseOption `json:"licenses,omitempty"`
	Genres    []Genre         `json:"genres,omitempty"`
	Tags      pq.StringArray  `json:"tags,omitempty" db:"tags"`
	Analytics *SpecAnalytics  `json:"analytics,omitempty"` // Optional, loaded when needed
}

// LicenseOption defines the pricing and features for a specific spec.
type LicenseOption struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	SpecID      uuid.UUID      `json:"spec_id" db:"spec_id"`
	LicenseType LicenseType    `json:"type" db:"license_type"`
	Name        string         `json:"name" db:"name"`
	Price       float64        `json:"price" db:"price"`
	Features    pq.StringArray `json:"features" db:"features"`
	FileTypes   pq.StringArray `json:"file_types" db:"file_types"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
	IsDeleted   bool           `json:"is_deleted" db:"is_deleted"`
}

// Genre represents a musical genre.
type Genre struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// SpecFilter contains all possible filters for listing specs
type SpecFilter struct {
	Category Category
	Genres   []string
	Tags     []string
	Search   string
	MinBPM   int
	MaxBPM   int
	MinPrice float64
	MaxPrice float64
	Key      string
	Limit    int
	Offset   int
	Sort     string
}

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
	SpecID    string  `json:"spec_id" db:"spec_id"`
	Title     string  `json:"title" db:"title"`
	Plays     int     `json:"plays" db:"plays"`
	Downloads int     `json:"downloads" db:"downloads"`
	Revenue   float64 `json:"revenue" db:"revenue"`
}

type SpecRepository interface {
	Create(ctx context.Context, spec *Spec) error
	GetByID(ctx context.Context, id uuid.UUID) (*Spec, error)
	GetByIDSystem(ctx context.Context, id uuid.UUID) (*Spec, error)
	List(ctx context.Context, filter SpecFilter) ([]Spec, int, error)
	Update(ctx context.Context, spec *Spec) error
	Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error
	ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]Spec, int, error)
}

type AnalyticsRepository interface {
	GetSpecAnalytics(ctx context.Context, specID uuid.UUID) (*SpecAnalytics, error)
	IncrementPlayCount(ctx context.Context, specID uuid.UUID) error
	IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error

	AddFavorite(ctx context.Context, userID, specID uuid.UUID) error
	RemoveFavorite(ctx context.Context, userID, specID uuid.UUID) error
	IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error)

	GetLicensePurchaseCounts(ctx context.Context, specID uuid.UUID) (map[string]int, error)

	// Overview Analytics
	GetTotalPlays(ctx context.Context, producerID uuid.UUID) (int, error)
	GetTotalFavorites(ctx context.Context, producerID uuid.UUID) (int, error)
	GetTotalDownloads(ctx context.Context, producerID uuid.UUID) (int, error)
	GetTotalRevenue(ctx context.Context, producerID uuid.UUID) (float64, error)
	GetRevenueByLicenseGlobal(ctx context.Context, producerID uuid.UUID) (map[string]float64, error)
	GetPlaysByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyStat, error)
	GetDownloadsByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyStat, error)
	GetRevenueByDay(ctx context.Context, producerID uuid.UUID, days int) ([]DailyRevenueStat, error)
	GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]TopSpecStat, error)
}
