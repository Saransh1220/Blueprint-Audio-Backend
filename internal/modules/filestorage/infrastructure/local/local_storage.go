package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalStorage implements FileStorage interface using local filesystem
type LocalStorage struct {
	basePath string
	baseURL  string
}

// NewLocalStorage creates a new local filesystem storage
func NewLocalStorage(basePath, baseURL string) (*LocalStorage, error) {
	// Ensure directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{
		basePath: basePath,
		baseURL:  baseURL,
	}, nil
}

// UploadFile uploads a file to local filesystem
func (l *LocalStorage) UploadFile(ctx context.Context, key string, file io.Reader, contentType string) (string, error) {
	fullPath := filepath.Join(l.basePath, key)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	outFile, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	// Copy content
	if _, err := io.Copy(outFile, file); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Return public URL
	return fmt.Sprintf("%s/%s", l.baseURL, key), nil
}

// DeleteFile deletes a file from local filesystem
func (l *LocalStorage) DeleteFile(ctx context.Context, key string) error {
	fullPath := filepath.Join(l.basePath, key)
	return os.Remove(fullPath)
}

// GetPresignedURL for local storage just returns the public URL (no presigning needed)
func (l *LocalStorage) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	return fmt.Sprintf("%s/%s", l.baseURL, key), nil
}

// GetPresignedDownloadURL for local storage just returns the public URL
func (l *LocalStorage) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	return fmt.Sprintf("%s/%s", l.baseURL, key), nil
}

// GetKeyFromURL extracts the key from a public URL
func (l *LocalStorage) GetKeyFromURL(url string) (string, error) {
	prefix := l.baseURL + "/"
	if len(url) > len(prefix) && url[:len(prefix)] == prefix {
		return url[len(prefix):], nil
	}
	return "", fmt.Errorf("url does not match expected format: %s", url)
}
