package application

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
)

// FileService provides high-level file operations
type FileService struct {
	storage domain.FileStorage
}

// NewFileService creates a new file service
func NewFileService(storage domain.FileStorage) *FileService {
	return &FileService{
		storage: storage,
	}
}

// Upload uploads a file with automatic key generation
func (s *FileService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	key := fmt.Sprintf("%s/%s", folder, filename)

	url, err := s.UploadWithKey(ctx, file, key, header.Header.Get("Content-Type"))
	if err != nil {
		return "", "", err
	}
	return url, key, nil
}

// UploadWithKey uploads a file with a specific key
func (s *FileService) UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error) {
	return s.storage.UploadFile(ctx, key, file, contentType)
}

// GetPresignedURL generates a presigned URL for viewing
func (s *FileService) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	return s.storage.GetPresignedURL(ctx, key, expiration)
}

// GetPresignedDownloadURL generates a presigned URL for downloading
func (s *FileService) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	return s.storage.GetPresignedDownloadURL(ctx, key, filename, expiration)
}

// Delete deletes a file
func (s *FileService) Delete(ctx context.Context, key string) error {
	return s.storage.DeleteFile(ctx, key)
}

// GetKeyFromUrl extracts the storage key from a URL
func (s *FileService) GetKeyFromUrl(fileUrl string) (string, error) {
	return s.storage.GetKeyFromURL(fileUrl)
}
