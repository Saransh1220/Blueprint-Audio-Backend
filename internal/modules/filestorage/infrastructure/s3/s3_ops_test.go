package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestS3Storage_UploadDeleteAndPresign(t *testing.T) {
	var copySource string
	var copySourceIfMatch string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", "5")
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("ETag", `"etag-value"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("hello"))
		case http.MethodPut:
			if source := r.Header.Get("X-Amz-Copy-Source"); source != "" {
				copySource = source
				copySourceIfMatch = r.Header.Get("X-Amz-Copy-Source-If-Match")
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<CopyObjectResult><ETag>"copied"</ETag><LastModified>2026-01-01T00:00:00Z</LastModified></CopyObjectResult>`))
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	st, err := NewS3Storage(context.Background(), S3Config{
		BucketName:      "bucket",
		Region:          "ap-south-1",
		Endpoint:        ts.URL,
		PresignEndpoint: ts.URL,
		PublicEndpoint:  "cdn.local",
		AccessKey:       "x",
		SecretKey:       "y",
		UseSSL:          false,
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
	pURL, err := neturl.Parse(p)
	require.NoError(t, err)
	require.Equal(t, strings.TrimPrefix(ts.URL, "http://"), pURL.Host)

	d, err := st.GetPresignedDownloadURL(context.Background(), "a/file.txt", "name.mp3", time.Minute)
	require.NoError(t, err)
	require.Contains(t, d, "response-content-disposition")

	beforeExpiry := time.Now().UTC().Add(time.Minute)
	upload, err := st.CreatePresignedUpload(
		context.Background(), "incoming/file.mp3", "audio/mpeg", 5, time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, http.MethodPut, upload.Method)
	require.Equal(t, "audio/mpeg", upload.Headers["Content-Type"])
	require.NotContains(t, upload.Headers, "Content-Length")
	require.False(t, upload.ExpiresAt.Before(beforeExpiry))
	uploadURL, err := neturl.Parse(upload.URL)
	require.NoError(t, err)
	require.Equal(t, strings.TrimPrefix(ts.URL, "http://"), uploadURL.Host)
	require.Contains(t, uploadURL.Query().Get("X-Amz-SignedHeaders"), "content-length")

	info, err := st.StatObject(context.Background(), "a/file.txt")
	require.NoError(t, err)
	require.Equal(t, "a/file.txt", info.Key)
	require.Equal(t, int64(5), info.Size)
	require.Equal(t, "audio/mpeg", info.ContentType)
	require.Equal(t, "etag-value", info.ETag)

	reader, err := st.OpenObject(context.Background(), "a/file.txt")
	require.NoError(t, err)
	opened, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "hello", string(opened))

	copiedURL, err := st.CopyObject(
		context.Background(), "incoming/file.mp3", "audio/previews/file.mp3", "etag-value",
	)
	require.NoError(t, err)
	require.Equal(t, "http://cdn.local/bucket/audio/previews/file.mp3", copiedURL)
	require.NotEmpty(t, copySource)
	require.Equal(t, `"etag-value"`, copySourceIfMatch)
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

	_, err = st.StatObject(context.Background(), "k")
	require.Error(t, err)
	_, err = st.OpenObject(context.Background(), "k")
	require.Error(t, err)
	_, err = st.CopyObject(context.Background(), "k", "other", "etag")
	require.Error(t, err)
}
