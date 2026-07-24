package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStorage struct {
	uploadFn          func(context.Context, string, io.Reader, string) (string, error)
	deleteFn          func(context.Context, string) error
	presignFn         func(context.Context, string, time.Duration) (string, error)
	presignDownloadFn func(context.Context, string, string, time.Duration) (string, error)
	getKeyFn          func(string) (string, error)
}

func (m mockStorage) UploadFile(ctx context.Context, key string, file io.Reader, ct string) (string, error) {
	return m.uploadFn(ctx, key, file, ct)
}
func (m mockStorage) DeleteFile(ctx context.Context, key string) error { return m.deleteFn(ctx, key) }
func (m mockStorage) GetPresignedURL(ctx context.Context, key string, d time.Duration) (string, error) {
	return m.presignFn(ctx, key, d)
}
func (m mockStorage) GetPresignedDownloadURL(ctx context.Context, key, filename string, d time.Duration) (string, error) {
	return m.presignDownloadFn(ctx, key, filename, d)
}
func (m mockStorage) GetKeyFromURL(u string) (string, error) { return m.getKeyFn(u) }

type mockDirectStorage struct {
	mockStorage
}

func (m mockDirectStorage) CreatePresignedUpload(ctx context.Context, key, contentType string, expectedSize int64, expiration time.Duration) (domain.PresignedUpload, error) {
	return domain.PresignedUpload{
		URL:       "https://storage.example/" + key,
		Method:    "PUT",
		Headers:   map[string]string{"Content-Type": contentType},
		ExpiresAt: time.Now().Add(expiration),
	}, nil
}

func (m mockDirectStorage) StatObject(ctx context.Context, key string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Key: key, Size: 6, ContentType: "audio/mpeg", ETag: "etag"}, nil
}

func (m mockDirectStorage) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("object")), nil
}

func (m mockDirectStorage) CopyObject(ctx context.Context, sourceKey, destinationKey, expectedETag string) (string, error) {
	return "https://storage.example/" + destinationKey, nil
}

func (m mockDirectStorage) ObjectURL(key string) (string, error) {
	return "https://storage.example/" + key, nil
}

func TestFileService_Methods(t *testing.T) {
	ms := mockStorage{
		uploadFn:          func(context.Context, string, io.Reader, string) (string, error) { return "url", nil },
		deleteFn:          func(context.Context, string) error { return nil },
		presignFn:         func(context.Context, string, time.Duration) (string, error) { return "p", nil },
		presignDownloadFn: func(context.Context, string, string, time.Duration) (string, error) { return "pd", nil },
		getKeyFn:          func(string) (string, error) { return "k", nil },
	}
	svc := application.NewFileService(ms)

	tf, err := os.CreateTemp(t.TempDir(), "upload-*.mp3")
	require.NoError(t, err)
	_, err = tf.WriteString("abc")
	require.NoError(t, err)
	require.NoError(t, tf.Close())

	f, err := os.Open(tf.Name())
	require.NoError(t, err)
	defer f.Close()

	h := &multipart.FileHeader{Filename: "x.mp3", Header: map[string][]string{"Content-Type": {"audio/mpeg"}}}
	url, key, err := svc.Upload(context.Background(), f, h, "folder")
	require.NoError(t, err)
	assert.Equal(t, "url", url)
	assert.Contains(t, key, "folder/")

	u, err := svc.UploadWithKey(context.Background(), bytes.NewBufferString("x"), "k", "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "url", u)

	_, err = svc.GetPresignedURL(context.Background(), "k", time.Minute)
	require.NoError(t, err)
	_, err = svc.GetPresignedDownloadURL(context.Background(), "k", "f", time.Minute)
	require.NoError(t, err)
	require.NoError(t, svc.Delete(context.Background(), "k"))
	_, err = svc.GetKeyFromUrl("u")
	require.NoError(t, err)
}

func TestFileService_UploadError(t *testing.T) {
	svc := application.NewFileService(mockStorage{
		uploadFn:          func(context.Context, string, io.Reader, string) (string, error) { return "", errors.New("x") },
		deleteFn:          func(context.Context, string) error { return nil },
		presignFn:         func(context.Context, string, time.Duration) (string, error) { return "", nil },
		presignDownloadFn: func(context.Context, string, string, time.Duration) (string, error) { return "", nil },
		getKeyFn:          func(string) (string, error) { return "", nil },
	})

	tf, err := os.CreateTemp(t.TempDir(), "upload-*.wav")
	require.NoError(t, err)
	_, _ = tf.WriteString("abc")
	require.NoError(t, tf.Close())
	f, err := os.Open(tf.Name())
	require.NoError(t, err)
	defer f.Close()

	h := &multipart.FileHeader{Filename: "x.wav", Header: map[string][]string{"Content-Type": {"audio/wav"}}}
	_, _, err = svc.Upload(context.Background(), f, h, "folder")
	require.Error(t, err)
}

func TestFileService_DirectUploadMethods(t *testing.T) {
	svc := application.NewFileService(mockDirectStorage{})
	ctx := context.Background()

	upload, err := svc.CreatePresignedUpload(ctx, "incoming/preview.mp3", "audio/mpeg", 6, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "PUT", upload.Method)
	assert.Equal(t, "audio/mpeg", upload.Headers["Content-Type"])

	info, err := svc.StatObject(ctx, "incoming/preview.mp3")
	require.NoError(t, err)
	assert.Equal(t, int64(6), info.Size)
	assert.Equal(t, "etag", info.ETag)

	reader, err := svc.OpenObject(ctx, "incoming/preview.mp3")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, "object", string(data))

	copiedURL, err := svc.CopyObject(ctx, "incoming/preview.mp3", "audio/previews/preview.mp3", "etag")
	require.NoError(t, err)
	assert.Equal(t, "https://storage.example/audio/previews/preview.mp3", copiedURL)

	objectURL, err := svc.ObjectURL("audio/previews/preview.mp3")
	require.NoError(t, err)
	assert.Equal(t, copiedURL, objectURL)
}

func TestFileService_DirectUploadUnsupported(t *testing.T) {
	svc := application.NewFileService(mockStorage{})
	ctx := context.Background()

	_, err := svc.CreatePresignedUpload(ctx, "key", "audio/mpeg", 6, time.Hour)
	assert.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = svc.StatObject(ctx, "key")
	assert.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = svc.OpenObject(ctx, "key")
	assert.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = svc.CopyObject(ctx, "source", "destination", "etag")
	assert.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
	_, err = svc.ObjectURL("key")
	assert.ErrorIs(t, err, domain.ErrDirectUploadUnsupported)
}
