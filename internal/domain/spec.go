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
	ID         uuid.UUID `json:"id" db:"id"`
	ProducerID uuid.UUID `json:"producer_id" db:"producer_id"`
	Title      string    `json:"title" db:"title"`
	Category   Category  `json:"category" db:"category"`
	Type       string    `json:"type" db:"type"` // e.g., WAV, STEMS, PACK
	BPM        int       `json:"bpm" db:"bpm"`
	Key        string    `json:"key" db:"key"`
	ImageUrl   string    `json:"image_url" db:"image_url"`
	PreviewUrl string    `json:"preview_url" db:"preview_url"`
	WavUrl     *string   `json:"wav_url,omitempty" db:"wav_url"`
	StemsUrl   *string   `json:"stems_url,omitempty" db:"stems_url"`
	BasePrice  float64   `json:"price" db:"base_price"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`

	// Relations
	Licenses []LicenseOption `json:"licenses,omitempty"`
	Genres   []Genre         `json:"genres,omitempty"`
	Tags     pq.StringArray  `json:"tags,omitempty" db:"tags"`
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
}

// Genre represents a musical genre.
type Genre struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type SpecRepository interface {
	Create(ctx context.Context, spec *Spec) error
	GetByID(ctx context.Context, id uuid.UUID) (*Spec, error)
	List(ctx context.Context, category Category, genres []string, tags []string, limit, offset int) ([]Spec, int, error)
	Update(ctx context.Context, spec *Spec) error
	Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error
	ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]Spec, int, error)
}
