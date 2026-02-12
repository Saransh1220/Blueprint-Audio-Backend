package s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds configuration for S3/MinIO storage
type S3Config struct {
	BucketName     string
	Region         string
	Endpoint       string // Internal endpoint (e.g., minio:9000)
	PublicEndpoint string // Public endpoint (e.g., localhost:9000)
	AccessKey      string
	SecretKey      string
	UseSSL         bool
}

// S3Storage implements FileStorage interface using AWS S3 or MinIO
type S3Storage struct {
	client        *s3.Client
	presignClient *s3.Client // Separate client for presigning with public endpoint
	config        S3Config
}

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

	// Create separate client for presigning using public endpoint
	var presignClient *s3.Client
	if cfg.Endpoint != "" && cfg.PublicEndpoint != "" {
		publicEndpointURL := cfg.PublicEndpoint
		if !cfg.UseSSL && !hasHTTPPrefix(publicEndpointURL) {
			publicEndpointURL = "http://" + publicEndpointURL
		}

		presignClient = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(publicEndpointURL)
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

	// Return Public URL
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
