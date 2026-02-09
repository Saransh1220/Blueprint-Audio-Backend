package mocks

import (
	"context"
	"io"
	"mime/multipart"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockFileService is a mock implementation of service.FileService for testing
type MockFileService struct {
	mock.Mock
}

func (m *MockFileService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	args := m.Called(ctx, file, header, folder)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockFileService) UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error) {
	args := m.Called(ctx, file, key, contentType)
	return args.String(0), args.Error(1)
}

func (m *MockFileService) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}

func (m *MockFileService) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, filename, expiration)
	return args.String(0), args.Error(1)
}

func (m *MockFileService) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockFileService) GetKeyFromUrl(url string) (string, error) {
	args := m.Called(url)
	return args.String(0), args.Error(1)
}
