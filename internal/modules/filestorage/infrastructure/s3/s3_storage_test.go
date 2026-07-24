package s3

import (
	"context"
	neturl "net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestNewS3Storage_ValidationAndConfig(t *testing.T) {
	_, err := NewS3Storage(context.Background(), S3Config{})
	require.Error(t, err)

	st, err := NewS3Storage(context.Background(), S3Config{
		BucketName:      "bucket",
		Region:          "ap-south-1",
		Endpoint:        "internal:9000",
		PresignEndpoint: "browser:9000",
		PublicEndpoint:  "cdn.local",
		AccessKey:       "x",
		SecretKey:       "y",
		UseSSL:          false,
	})
	require.NoError(t, err)
	require.NotNil(t, st)
	require.NotNil(t, st.client)
	require.NotNil(t, st.presignClient)
	require.Equal(t, "browser:9000", st.config.PresignEndpoint)
}

func TestS3Storage_GetKeyFromURL_AndHelpers(t *testing.T) {
	st := &S3Storage{client: &s3.Client{}, presignClient: &s3.Client{}, config: S3Config{BucketName: "b", Region: "ap-south-1", Endpoint: "localhost:9000", PublicEndpoint: "cdn.local"}}

	k, err := st.GetKeyFromURL("http://cdn.local/b/a/file.mp3")
	require.NoError(t, err)
	require.Equal(t, "a/file.mp3", k)

	k, err = st.GetKeyFromURL("http://localhost:9000/b/x.wav")
	require.NoError(t, err)
	require.Equal(t, "x.wav", k)

	st2 := &S3Storage{config: S3Config{BucketName: "b", Region: "ap-south-1"}}
	k, err = st2.GetKeyFromURL("https://b.s3.ap-south-1.amazonaws.com/f/g")
	require.NoError(t, err)
	require.Equal(t, "f/g", k)

	_, err = st2.GetKeyFromURL("https://example.com/x")
	require.Error(t, err)

	require.True(t, hasHTTPPrefix("http://x"))
	require.True(t, hasHTTPPrefix("https://x"))
	require.False(t, hasHTTPPrefix("x"))
}

func TestS3Storage_ObjectURLPreservesExistingFormats(t *testing.T) {
	withPublicEndpoint := &S3Storage{config: S3Config{
		BucketName:     "bucket",
		PublicEndpoint: "cdn.local",
	}}
	objectURL, err := withPublicEndpoint.ObjectURL("audio/file.mp3")
	require.NoError(t, err)
	require.Equal(t, "http://cdn.local/bucket/audio/file.mp3", objectURL)

	withStorageEndpoint := &S3Storage{config: S3Config{
		BucketName: "bucket",
		Endpoint:   "https://storage.example",
	}}
	objectURL, err = withStorageEndpoint.ObjectURL("audio/file.mp3")
	require.NoError(t, err)
	require.Equal(t, "https://storage.example/bucket/audio/file.mp3", objectURL)

	standardS3 := &S3Storage{config: S3Config{
		BucketName: "bucket",
		Region:     "ap-south-1",
	}}
	objectURL, err = standardS3.ObjectURL("audio/file.mp3")
	require.NoError(t, err)
	require.Equal(t, "https://bucket.s3.ap-south-1.amazonaws.com/audio/file.mp3", objectURL)
}

func TestS3Storage_PresigningUsesPresignEndpointNotPublicEndpoint(t *testing.T) {
	storage, err := NewS3Storage(context.Background(), S3Config{
		BucketName:      "bucket",
		Region:          "auto",
		Endpoint:        "internal.storage:9000",
		PresignEndpoint: "browser.storage:9000",
		PublicEndpoint:  "cdn.storage",
		AccessKey:       "x",
		SecretKey:       "y",
		UseSSL:          false,
	})
	require.NoError(t, err)

	readURL, err := storage.GetPresignedURL(context.Background(), "audio/file.mp3", time.Minute)
	require.NoError(t, err)
	parsedReadURL, err := neturl.Parse(readURL)
	require.NoError(t, err)
	require.Equal(t, "browser.storage:9000", parsedReadURL.Host)

	upload, err := storage.CreatePresignedUpload(
		context.Background(), "incoming/file.mp3", "audio/mpeg", 5, time.Minute,
	)
	require.NoError(t, err)
	parsedUploadURL, err := neturl.Parse(upload.URL)
	require.NoError(t, err)
	require.Equal(t, "browser.storage:9000", parsedUploadURL.Host)

	objectURL, err := storage.ObjectURL("audio/file.mp3")
	require.NoError(t, err)
	require.Equal(t, "http://cdn.storage/bucket/audio/file.mp3", objectURL)
}
