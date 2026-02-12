package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestNewS3Storage_ValidationAndConfig(t *testing.T) {
	_, err := NewS3Storage(context.Background(), S3Config{})
	require.Error(t, err)

	st, err := NewS3Storage(context.Background(), S3Config{
		BucketName:     "bucket",
		Region:         "ap-south-1",
		Endpoint:       "localhost:9000",
		PublicEndpoint: "localhost:9000",
		AccessKey:      "x",
		SecretKey:      "y",
		UseSSL:         false,
	})
	require.NoError(t, err)
	require.NotNil(t, st)
	require.NotNil(t, st.client)
	require.NotNil(t, st.presignClient)
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
