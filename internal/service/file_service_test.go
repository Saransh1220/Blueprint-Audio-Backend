package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3FileService_GetKeyFromUrl(t *testing.T) {
	s := &s3FileService{
		bucketName:     "bucket",
		publicEndpoint: "localhost:9000",
		endpoint:       "minio:9000",
		region:         "ap-south-1",
	}

	key, err := s.GetKeyFromUrl("http://localhost:9000/bucket/path/file.mp3")
	assert.NoError(t, err)
	assert.Equal(t, "path/file.mp3", key)

	key, err = s.GetKeyFromUrl("http://minio:9000/bucket/path/file.wav")
	assert.NoError(t, err)
	assert.Equal(t, "path/file.wav", key)

	s.endpoint = ""
	key, err = s.GetKeyFromUrl("https://bucket.s3.ap-south-1.amazonaws.com/path/file.jpg")
	assert.NoError(t, err)
	assert.Equal(t, "path/file.jpg", key)

	_, err = s.GetKeyFromUrl("http://unknown/other/file")
	assert.Error(t, err)
}

func TestNewFileService_AndPresign(t *testing.T) {
	t.Run("missing bucket", func(t *testing.T) {
		t.Setenv("S3_BUCKET", "")
		_, err := NewFileService(context.Background())
		require.Error(t, err)
		assert.EqualError(t, err, "S3_BUCKET is required")
	})

	t.Run("minio config and presign", func(t *testing.T) {
		t.Setenv("S3_BUCKET", "bucket")
		t.Setenv("S3_REGION", "ap-south-1")
		t.Setenv("S3_ENDPOINT", "localhost:9000")
		t.Setenv("S3_PUBLIC_ENDPOINT", "localhost:9000")
		t.Setenv("S3_ACCESS_KEY", "minio")
		t.Setenv("S3_SECRET_KEY", "miniosecret")
		t.Setenv("S3_USE_SSL", "false")

		fs, err := NewFileService(context.Background())
		require.NoError(t, err)

		s3fs, ok := fs.(*s3FileService)
		require.True(t, ok)
		assert.Equal(t, "http://localhost:9000", s3fs.endpoint)

		url, err := s3fs.GetPresignedURL(context.Background(), "audio/file.mp3", time.Minute)
		require.NoError(t, err)
		assert.Contains(t, url, "X-Amz-Signature")

		downloadURL, err := s3fs.GetPresignedDownloadURL(context.Background(), "audio/file.mp3", "test.mp3", time.Minute)
		require.NoError(t, err)
		assert.Contains(t, downloadURL, "response-content-disposition")
	})
}
