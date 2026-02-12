package http

import (
	"context"
	"io"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
)

// FileService defines the interface for file operations
type FileService interface {
	Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error)
	UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error)
	GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error)
	GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error)
	GetKeyFromUrl(fileUrl string) (string, error)
	Delete(ctx context.Context, key string) error
}

// AnalyticsService defines the dependencies on the analytics module
type AnalyticsService interface {
	GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*domain.PublicAnalytics, error)
	TrackFreeDownload(ctx context.Context, specID uuid.UUID) error
}
