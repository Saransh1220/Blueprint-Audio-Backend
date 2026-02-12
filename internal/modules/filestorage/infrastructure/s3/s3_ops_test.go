package s3

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestS3Storage_UploadDeleteAndPresign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	st, err := NewS3Storage(context.Background(), S3Config{
		BucketName:     "bucket",
		Region:         "ap-south-1",
		Endpoint:       ts.URL,
		PublicEndpoint: "cdn.local",
		AccessKey:      "x",
		SecretKey:      "y",
		UseSSL:         false,
	})
	require.NoError(t, err)

	url, err := st.UploadFile(context.Background(), "a/file.txt", bytes.NewReader([]byte("hello")), "text/plain")
	require.NoError(t, err)
	require.True(t, strings.Contains(url, "cdn.local/bucket/a/file.txt"))

	err = st.DeleteFile(context.Background(), "a/file.txt")
	require.NoError(t, err)

	p, err := st.GetPresignedURL(context.Background(), "a/file.txt", time.Minute)
	require.NoError(t, err)
	require.Contains(t, p, "/a/file.txt")

	d, err := st.GetPresignedDownloadURL(context.Background(), "a/file.txt", "name.mp3", time.Minute)
	require.NoError(t, err)
	require.Contains(t, d, "response-content-disposition")
}

func TestS3Storage_UploadAndDelete_Error(t *testing.T) {
	st, err := NewS3Storage(context.Background(), S3Config{
		BucketName: "bucket", Region: "ap-south-1", Endpoint: "http://127.0.0.1:1", AccessKey: "x", SecretKey: "y",
	})
	require.NoError(t, err)

	_, err = st.UploadFile(context.Background(), "k", bytes.NewBufferString("x"), "text/plain")
	require.Error(t, err)

	err = st.DeleteFile(context.Background(), "k")
	require.Error(t, err)
}
