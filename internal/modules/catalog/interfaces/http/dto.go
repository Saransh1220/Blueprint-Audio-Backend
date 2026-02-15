package http

import (
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

// SpecResponse is the PUBLIC response
type SpecResponse struct {
	ID               uuid.UUID         `json:"id"`
	ProducerID       uuid.UUID         `json:"producer_id"`
	ProducerName     string            `json:"producer_name"`
	Title            string            `json:"title"`
	Category         string            `json:"category"`
	Type             string            `json:"type"`
	BPM              int               `json:"bpm"`
	Key              string            `json:"key"`
	ImageURL         string            `json:"image_url"`
	PreviewURL       string            `json:"preview_url"`
	Price            float64           `json:"price"`
	Duration         int               `json:"duration"`
	FreeMp3Enabled   bool              `json:"free_mp3_enabled"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Licenses         []LicenseResponse `json:"licenses,omitempty"`
	Genres           []GenreResponse   `json:"genres,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Analytics        *SpecAnalytics    `json:"analytics,omitempty"`
	ProcessingStatus string            `json:"processing_status"`
}

// SpecAnalytics contains publicly visible analytics
type SpecAnalytics struct {
	PlayCount          int  `json:"play_count"`
	FavoriteCount      int  `json:"favorite_count"`
	TotalDownloadCount int  `json:"total_download_count"`
	IsFavorited        bool `json:"is_favorited"`
}

// LicenseResponse for nested license data
type LicenseResponse struct {
	ID        uuid.UUID `json:"id"`
	SpecID    uuid.UUID `json:"spec_id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	Features  []string  `json:"features"`
	FileTypes []string  `json:"file_types"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GenreResponse for nested genre data
type GenreResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

func ToSpecResponse(spec *domain.Spec) *SpecResponse {
	response := &SpecResponse{
		ID:               spec.ID,
		ProducerID:       spec.ProducerID,
		ProducerName:     spec.ProducerName,
		Title:            spec.Title,
		Category:         string(spec.Category),
		Type:             spec.Type,
		BPM:              spec.BPM,
		Key:              spec.Key,
		ImageURL:         spec.ImageUrl,
		PreviewURL:       spec.PreviewUrl,
		Price:            spec.BasePrice,
		Duration:         spec.Duration,
		FreeMp3Enabled:   spec.FreeMp3Enabled,
		CreatedAt:        spec.CreatedAt,
		UpdatedAt:        spec.UpdatedAt,
		Tags:             spec.Tags,
		ProcessingStatus: string(spec.ProcessingStatus),
	}

	// Convert licenses
	if len(spec.Licenses) > 0 {
		response.Licenses = make([]LicenseResponse, len(spec.Licenses))
		for i, license := range spec.Licenses {
			response.Licenses[i] = LicenseResponse{
				ID:        license.ID,
				SpecID:    license.SpecID,
				Type:      string(license.LicenseType),
				Name:      license.Name,
				Price:     license.Price,
				Features:  license.Features,
				FileTypes: license.FileTypes,
				CreatedAt: license.CreatedAt,
				UpdatedAt: license.UpdatedAt,
			}
		}
	}

	// Convert genres
	if len(spec.Genres) > 0 {
		response.Genres = make([]GenreResponse, len(spec.Genres))
		for i, genre := range spec.Genres {
			response.Genres[i] = GenreResponse{
				ID:        genre.ID,
				Name:      genre.Name,
				Slug:      genre.Slug,
				CreatedAt: genre.CreatedAt,
			}
		}
	}

	return response
}
