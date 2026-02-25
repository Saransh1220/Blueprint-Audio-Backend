package auth

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	fileapp "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/require"
)

type noopStorage struct{}

func (noopStorage) UploadFile(_ context.Context, _ string, _ io.Reader, _ string) (string, error) {
	return "", nil
}
func (noopStorage) DeleteFile(_ context.Context, _ string) error { return nil }
func (noopStorage) GetPresignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (noopStorage) GetPresignedDownloadURL(_ context.Context, _ string, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (noopStorage) GetKeyFromURL(_ string) (string, error) { return "", nil }

var _ domain.FileStorage = noopStorage{}

func TestNewModuleAndAccessors(t *testing.T) {
	fs := fileapp.NewFileService(noopStorage{})
	m, err := NewModule(&sqlx.DB{}, "secret", time.Hour, fs, "test-client-id")
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Service())
	require.NotNil(t, m.UserFinder())
	require.NotNil(t, m.UserRepository())
	require.NotNil(t, m.HTTPHandler())
	_ = uuid.New()
}
