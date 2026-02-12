package domain

import (
	"context"
	"io"
	"time"
)

// FileStorage defines the interface for file storage operations
// This can be implemented by S3, MinIO, local filesystem, etc.
type FileStorage interface {
	// UploadFile uploads a file with the given key and returns the public URL
	UploadFile(ctx context.Context, key string, file io.Reader, contentType string) (string, error)

	// DeleteFile deletes a file by its key
	DeleteFile(ctx context.Context, key string) error

	// GetPresignedURL generates a temporary presigned URL for viewing a file
	GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// GetPresignedDownloadURL generates a temporary presigned URL for downloading a file
	GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error)

	// GetKeyFromURL extracts the storage key from a public URL
	GetKeyFromURL(url string) (string, error)
}
