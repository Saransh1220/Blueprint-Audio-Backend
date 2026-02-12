package filestorage

import (
	"context"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/stretchr/testify/require"
)

func TestNewModule_LocalAndS3Error(t *testing.T) {
	m, err := NewModule(context.Background(), config.FileStorageConfig{UseS3: false, LocalPath: t.TempDir()})
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Service())

	_, err = NewModule(context.Background(), config.FileStorageConfig{UseS3: true, S3BucketName: ""})
	require.Error(t, err)
}
