package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
)

// S3Config holds configuration for S3/MinIO storage
type S3Config struct {
	BucketName      string
	Region          string
	Endpoint        string // Internal endpoint used by server-side operations.
	PresignEndpoint string // Browser-reachable S3 API endpoint used for signing.
	PublicEndpoint  string // Base endpoint used by stable canonical object URLs.
	AccessKey       string
	SecretKey       string
	UseSSL          bool
}

// S3Storage implements FileStorage interface using AWS S3 or MinIO
type S3Storage struct {
	client        *s3.Client
	presignClient *s3.Client
	config        S3Config
}

var _ domain.FileStorage = (*S3Storage)(nil)
var _ domain.DirectUploadStorage = (*S3Storage)(nil)

// NewS3Storage creates a new S3 storage implementation
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	if cfg.BucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	var awsCfg aws.Config
	var err error

	if cfg.Endpoint != "" {
		// MinIO / LocalStack Configuration
		endpoint := cfg.Endpoint
		if !cfg.UseSSL && !hasHTTPPrefix(endpoint) {
			endpoint = "http://" + endpoint
		}

		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
		)
	} else {
		// Standard AWS S3 Configuration
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			endpoint := cfg.Endpoint
			if !cfg.UseSSL && !hasHTTPPrefix(endpoint) {
				endpoint = "http://" + endpoint
			}
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Required for MinIO
		}
	})

	// Presigned URLs must use a browser-reachable S3 API endpoint. This is
	// intentionally separate from a public CDN/custom domain, which may not
	// support S3 signatures.
	presignEndpoint := cfg.PresignEndpoint
	if presignEndpoint == "" {
		presignEndpoint = cfg.Endpoint
		cfg.PresignEndpoint = presignEndpoint
	}

	var presignClient *s3.Client
	if presignEndpoint != "" {
		presignEndpointURL := presignEndpoint
		if !cfg.UseSSL && !hasHTTPPrefix(presignEndpointURL) {
			presignEndpointURL = "http://" + presignEndpointURL
		}

		presignClient = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(presignEndpointURL)
			o.UsePathStyle = true
		})
	} else {
		// For AWS S3, use the same client
		presignClient = client
	}

	return &S3Storage{
		client:        client,
		presignClient: presignClient,
		config:        cfg,
	}, nil
}

// UploadFile uploads a file to S3 and returns the public URL
func (s *S3Storage) UploadFile(ctx context.Context, key string, file io.Reader, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.config.BucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to s3: %w", err)
	}

	return s.ObjectURL(key)
}

// CreatePresignedUpload creates a presigned PUT request for a direct upload.
func (s *S3Storage) CreatePresignedUpload(
	ctx context.Context,
	key, contentType string,
	expectedSize int64,
	expiration time.Duration,
) (domain.PresignedUpload, error) {
	if expectedSize <= 0 {
		return domain.PresignedUpload{}, fmt.Errorf("expected upload size must be positive")
	}
	startedAt := time.Now().UTC()
	presignClient := s3.NewPresignClient(s.presignClient)
	request, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.config.BucketName),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(expectedSize),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})
	if err != nil {
		return domain.PresignedUpload{}, fmt.Errorf("failed to generate presigned upload: %w", err)
	}

	headers := make(map[string]string, len(request.SignedHeader)+1)
	if contentType != "" {
		// AWS SDK v2 removes Content-Type from a bodyless presign request.
		// Return it explicitly so the browser stores the intended metadata;
		// completion still verifies the resulting object with HeadObject.
		headers["Content-Type"] = contentType
	}
	for name, values := range request.SignedHeader {
		// The browser derives Host from the signed URL and does not permit
		// callers to set it explicitly.
		if strings.EqualFold(name, "Host") ||
			strings.EqualFold(name, "Content-Length") ||
			len(values) == 0 {
			continue
		}
		headers[http.CanonicalHeaderKey(name)] = strings.Join(values, ",")
	}

	return domain.PresignedUpload{
		URL:       request.URL,
		Method:    http.MethodPut,
		Headers:   headers,
		ExpiresAt: startedAt.Add(expiration),
	}, nil
}

// StatObject returns storage-verified object metadata.
func (s *S3Storage) StatObject(ctx context.Context, key string) (domain.ObjectInfo, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("failed to stat s3 object: %w", err)
	}

	return domain.ObjectInfo{
		Key:         key,
		Size:        aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType),
		ETag:        strings.Trim(aws.ToString(result.ETag), `"`),
	}, nil
}

// OpenObject opens an S3 object as a streaming reader.
func (s *S3Storage) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open s3 object: %w", err)
	}
	return result.Body, nil
}

// CopyObject copies an object inside the bucket and returns the destination's
// stable canonical URL.
func (s *S3Storage) CopyObject(
	ctx context.Context,
	sourceKey, destinationKey, expectedETag string,
) (string, error) {
	copySource := url.PathEscape(s.config.BucketName + "/" + sourceKey)
	if expectedETag == "" {
		return "", fmt.Errorf("expected source ETag is required")
	}
	quotedETag := `"` + strings.Trim(expectedETag, `"`) + `"`
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(s.config.BucketName),
		CopySource:        aws.String(copySource),
		CopySourceIfMatch: aws.String(quotedETag),
		Key:               aws.String(destinationKey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to copy s3 object: %w", err)
	}
	return s.ObjectURL(destinationKey)
}

// ObjectURL returns the stable canonical URL used by existing database rows.
func (s *S3Storage) ObjectURL(key string) (string, error) {
	if s.config.PublicEndpoint != "" {
		prefix := ""
		if !hasHTTPPrefix(s.config.PublicEndpoint) {
			prefix = "http://"
		}
		return fmt.Sprintf("%s%s/%s/%s", prefix, s.config.PublicEndpoint, s.config.BucketName, key), nil
	}

	if s.config.Endpoint != "" {
		return fmt.Sprintf("%s/%s/%s", s.config.Endpoint, s.config.BucketName, key), nil
	}

	// S3: https://bucket.s3.region.amazonaws.com/folder/file.ext
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.config.BucketName, s.config.Region, key), nil
}

// DeleteFile deletes a file from S3
func (s *S3Storage) DeleteFile(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	})
	return err
}

// GetPresignedURL generates a presigned URL for viewing a file
func (s *S3Storage) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.presignClient)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// GetPresignedDownloadURL generates a presigned URL for downloading a file
func (s *S3Storage) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.presignClient)

	// Clean filename for safety
	if filename == "" || filename == "." {
		filename = "download.mp3"
	}
	contentDisposition := fmt.Sprintf("attachment; filename=\"%s\"", filename)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.config.BucketName),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(contentDisposition),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	return request.URL, nil
}

// GetKeyFromURL extracts the storage key from a public URL
func (s *S3Storage) GetKeyFromURL(fileUrl string) (string, error) {
	checkPrefix := func(endpoint string) (string, bool) {
		if endpoint == "" {
			return "", false
		}
		prefixBase := endpoint
		if !hasHTTPPrefix(prefixBase) {
			prefixBase = "http://" + prefixBase
		}

		prefix := fmt.Sprintf("%s/%s/", prefixBase, s.config.BucketName)
		if strings.HasPrefix(fileUrl, prefix) {
			return strings.TrimPrefix(fileUrl, prefix), true
		}
		return "", false
	}

	// Check Public Endpoint
	if key, ok := checkPrefix(s.config.PublicEndpoint); ok {
		return key, nil
	}

	// Check Internal Endpoint
	if key, ok := checkPrefix(s.config.Endpoint); ok {
		return key, nil
	}

	// Check Standard S3 Format
	if s.config.Endpoint == "" {
		prefix := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/", s.config.BucketName, s.config.Region)
		if strings.HasPrefix(fileUrl, prefix) {
			return strings.TrimPrefix(fileUrl, prefix), nil
		}
	}

	return "", fmt.Errorf("url does not match expected format: %s", fileUrl)
}

// hasHTTPPrefix checks if a string has http:// or https:// prefix
func hasHTTPPrefix(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
