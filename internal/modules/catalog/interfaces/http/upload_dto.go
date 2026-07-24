package http

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type CreateSpecMetadataRequest struct {
	Title          string                 `json:"title"`
	Category       domain.Category        `json:"category"`
	Type           string                 `json:"type"`
	BPM            int                    `json:"bpm"`
	Key            string                 `json:"key"`
	Price          float64                `json:"price"`
	PriceCurrency  string                 `json:"price_currency"`
	Description    string                 `json:"description"`
	FreeMP3Enabled bool                   `json:"free_mp3_enabled"`
	Tags           []string               `json:"tags"`
	Moods          []string               `json:"moods"`
	Instruments    []string               `json:"instruments"`
	Genres         []CreateGenreRequest   `json:"genres"`
	Licenses       []CreateLicenseRequest `json:"licenses"`
}

type CreateGenreRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type CreateLicenseRequest struct {
	Type          domain.LicenseType `json:"type"`
	Name          string             `json:"name"`
	Price         float64            `json:"price"`
	PriceCurrency string             `json:"price_currency"`
	Features      []string           `json:"features"`
	FileTypes     []string           `json:"file_types"`
}

type CreateUploadFileRequest struct {
	Kind        domain.UploadAssetKind `json:"kind"`
	FileName    string                 `json:"file_name"`
	ContentType string                 `json:"content_type"`
	SizeBytes   int64                  `json:"size_bytes"`
}

type CreateSpecUploadResponse struct {
	UploadID  uuid.UUID `json:"upload_id"`
	SpecID    uuid.UUID `json:"spec_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PresignedUploadResponse struct {
	AssetID   uuid.UUID              `json:"asset_id"`
	Kind      domain.UploadAssetKind `json:"kind"`
	ObjectKey string                 `json:"object_key"`
	UploadURL string                 `json:"upload_url"`
	Method    string                 `json:"method"`
	Headers   map[string]string      `json:"headers"`
}

type ConfirmedUploadFileResponse struct {
	AssetID     uuid.UUID              `json:"asset_id"`
	Kind        domain.UploadAssetKind `json:"kind"`
	FileName    string                 `json:"file_name"`
	SizeBytes   int64                  `json:"size_bytes"`
	ContentType string                 `json:"content_type"`
}

type SpecUploadStatusResponse struct {
	UploadID         uuid.UUID               `json:"upload_id"`
	SpecID           uuid.UUID               `json:"spec_id"`
	Status           domain.UploadStatus     `json:"status"`
	ProcessingStatus domain.ProcessingStatus `json:"processing_status"`
	Error            *string                 `json:"error,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	ExpiresAt        time.Time               `json:"expires_at"`
}

func (r CreateSpecMetadataRequest) toSpec() domain.Spec {
	spec := domain.Spec{
		Title:          r.Title,
		Category:       r.Category,
		Type:           r.Type,
		BPM:            r.BPM,
		Key:            r.Key,
		BasePrice:      r.Price,
		PriceCurrency:  r.PriceCurrency,
		Description:    r.Description,
		FreeMp3Enabled: r.FreeMP3Enabled,
		Tags:           pq.StringArray(r.Tags),
		Moods:          pq.StringArray(r.Moods),
		Instruments:    pq.StringArray(r.Instruments),
		Genres:         make([]domain.Genre, len(r.Genres)),
		Licenses:       make([]domain.LicenseOption, len(r.Licenses)),
	}
	for i, genre := range r.Genres {
		spec.Genres[i] = domain.Genre{Name: genre.Name, Slug: genre.Slug}
	}
	for i, license := range r.Licenses {
		spec.Licenses[i] = domain.LicenseOption{
			LicenseType:   license.Type,
			Name:          license.Name,
			Price:         license.Price,
			PriceCurrency: license.PriceCurrency,
			Features:      pq.StringArray(license.Features),
			FileTypes:     pq.StringArray(license.FileTypes),
		}
	}
	return spec
}

func (r CreateUploadFileRequest) toCommand() application.UploadFileCommand {
	return application.UploadFileCommand{
		Kind:        r.Kind,
		FileName:    r.FileName,
		ContentType: r.ContentType,
		SizeBytes:   r.SizeBytes,
	}
}

func toCreateUploadResponse(result *application.InitiateSpecUploadResult) CreateSpecUploadResponse {
	return CreateSpecUploadResponse{
		UploadID:  result.UploadID,
		SpecID:    result.SpecID,
		ExpiresAt: result.ExpiresAt,
	}
}

func toPresignedUploadResponse(instruction *application.UploadInstruction) PresignedUploadResponse {
	return PresignedUploadResponse{
		AssetID:   instruction.AssetID,
		Kind:      instruction.Kind,
		ObjectKey: instruction.ObjectKey,
		UploadURL: instruction.Upload.URL,
		Method:    instruction.Upload.Method,
		Headers:   instruction.Upload.Headers,
	}
}

func toConfirmedUploadFileResponse(asset *domain.SpecUploadAsset) ConfirmedUploadFileResponse {
	contentType := asset.DeclaredContentType
	if asset.ActualContentType != nil {
		contentType = *asset.ActualContentType
	}
	size := asset.ExpectedSize
	if asset.ActualSize != nil {
		size = *asset.ActualSize
	}
	return ConfirmedUploadFileResponse{
		AssetID:     asset.ID,
		Kind:        asset.Kind,
		FileName:    asset.FileName,
		SizeBytes:   size,
		ContentType: contentType,
	}
}

func toUploadStatusResponse(status *domain.SpecUploadStatus) SpecUploadStatusResponse {
	return SpecUploadStatusResponse{
		UploadID:         status.UploadID,
		SpecID:           status.SpecID,
		Status:           status.Status,
		ProcessingStatus: status.ProcessingStatus,
		Error:            status.ErrorMessage,
		CreatedAt:        status.CreatedAt,
		UpdatedAt:        status.UpdatedAt,
		ExpiresAt:        status.ExpiresAt,
	}
}
