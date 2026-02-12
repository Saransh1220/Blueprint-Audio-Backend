package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
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
