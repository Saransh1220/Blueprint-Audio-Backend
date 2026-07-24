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
	fileID, err := uuid.NewV7()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate uuid: %w", err)
	}
	filename := fmt.Sprintf("%s%s", fileID.String(), ext)
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

// CreatePresignedUpload creates a temporary PUT request for a direct upload.
func (s *FileService) CreatePresignedUpload(
	ctx context.Context,
	key, contentType string,
	expectedSize int64,
	expiration time.Duration,
) (domain.PresignedUpload, error) {
	storage, err := s.directUploadStorage()
	if err != nil {
		return domain.PresignedUpload{}, err
	}
	return storage.CreatePresignedUpload(ctx, key, contentType, expectedSize, expiration)
}

// StatObject returns object metadata verified by the storage backend.
func (s *FileService) StatObject(ctx context.Context, key string) (domain.ObjectInfo, error) {
	storage, err := s.directUploadStorage()
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	return storage.StatObject(ctx, key)
}

// OpenObject opens an object as a stream.
func (s *FileService) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	storage, err := s.directUploadStorage()
	if err != nil {
		return nil, err
	}
	return storage.OpenObject(ctx, key)
}

// CopyObject copies an object inside the storage backend and returns the
// destination's stable canonical URL.
func (s *FileService) CopyObject(ctx context.Context, sourceKey, destinationKey, expectedETag string) (string, error) {
	storage, err := s.directUploadStorage()
	if err != nil {
		return "", err
	}
	return storage.CopyObject(ctx, sourceKey, destinationKey, expectedETag)
}

// ObjectURL returns the stable canonical URL for an object key.
func (s *FileService) ObjectURL(key string) (string, error) {
	storage, err := s.directUploadStorage()
	if err != nil {
		return "", err
	}
	return storage.ObjectURL(key)
}

func (s *FileService) directUploadStorage() (domain.DirectUploadStorage, error) {
	storage, ok := s.storage.(domain.DirectUploadStorage)
	if !ok {
		return nil, domain.ErrDirectUploadUnsupported
	}
	return storage, nil
}
