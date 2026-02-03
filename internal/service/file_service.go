package service

import (
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type FileService interface {
	Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error)
	Delete(ctx context.Context, key string) error
	GetKeyFromUrl(fileUrl string) (string, error)
} // Ref: FileService Interface Update

type s3FileService struct {
	client         *s3.Client
	bucketName     string
	endpoint       string
	publicEndpoint string
	region         string
}

func NewFileService(ctx context.Context) (FileService, error) {
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("S3_REGION")
	endpoint := os.Getenv("S3_ENDPOINT")
	publicEndpoint := os.Getenv("S3_PUBLIC_ENDPOINT")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	useSSL := os.Getenv("S3_USE_SSL") == "true"

	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required")
	}

	var cfg aws.Config
	var err error

	if endpoint != "" {
		// MinIO / LocalStack Configuration
		// Ensure endpoint has protocol if missing and useSSL is strictly checked
		if !useSSL && !func() bool {
			// simplified check, better to just prepend http:// if no protocol present
			return len(endpoint) > 7 && (endpoint[:7] == "http://" || endpoint[:8] == "https://")
		}() {
			endpoint = "http://" + endpoint
		}

		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else {
		// Standard AWS S3 Configuration
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}

	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Required for MinIO
		}
	})

	return &s3FileService{
		client:         client,
		bucketName:     bucket,
		endpoint:       endpoint,
		publicEndpoint: publicEndpoint,
		region:         region,
	}, nil
}

func (s *s3FileService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	// 1. Generate Unique Filename (UUID + Original Ext)
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	key := fmt.Sprintf("%s/%s", folder, filename)

	// 2. Upload to S3
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(header.Header.Get("Content-Type")),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload to s3: %w", err)
	}

	// 3. Return Public URL
	// Use Public Endpoint if configured (e.g. http://localhost:9000 for MinIO)
	if s.publicEndpoint != "" {
		// Ensure protocol (simple check)
		prefix := ""
		if !strings.HasPrefix(s.publicEndpoint, "http") {
			prefix = "http://"
		}
		return fmt.Sprintf("%s%s/%s/%s", prefix, s.publicEndpoint, s.bucketName, key), key, nil
	}

	if s.endpoint != "" {

		return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucketName, key), key, nil
	}

	// S3: https://bucket.s3.region.amazonaws.com/folder/file.ext
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucketName, s.region, key), key, nil
}

func (s *s3FileService) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	return err
}

func (s *s3FileService) GetKeyFromUrl(fileUrl string) (string, error) {
	if s.endpoint != "" {
		prefix := fmt.Sprintf("%s/%s/", s.endpoint, s.bucketName)
		if strings.HasPrefix(fileUrl, prefix) {
			return strings.TrimPrefix(fileUrl, prefix), nil
		}
	} else {
		// S3: https://bucket.s3.region.amazonaws.com/folder/file.ext
		prefix := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/", s.bucketName, s.region)
		if strings.HasPrefix(fileUrl, prefix) {
			return strings.TrimPrefix(fileUrl, prefix), nil
		}
	}
	return "", fmt.Errorf("url does not match expected format")
}
