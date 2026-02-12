package filestorage

import (
	"context"
	"fmt"

	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/infrastructure/local"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage/infrastructure/s3"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
)

// Module represents the FileStorage module
type Module struct {
	service *application.FileService
	storage domain.FileStorage
}

// NewModule creates and initializes the FileStorage module
func NewModule(ctx context.Context, cfg config.FileStorageConfig) (*Module, error) {
	var storage domain.FileStorage
	var err error

	if cfg.UseS3 {
		// Initialize S3 storage
		s3Cfg := s3.S3Config{
			BucketName:     cfg.S3BucketName,
			Region:         cfg.S3Region,
			Endpoint:       cfg.S3Endpoint,
			PublicEndpoint: cfg.S3PublicEndpoint,
			AccessKey:      cfg.S3AccessKey,
			SecretKey:      cfg.S3SecretKey,
			UseSSL:         cfg.S3UseSSL,
		}
		storage, err = s3.NewS3Storage(ctx, s3Cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize S3 storage: %w", err)
		}
	} else {
		// Initialize local storage
		storage, err = local.NewLocalStorage(cfg.LocalPath, "http://localhost:8080/uploads")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize local storage: %w", err)
		}
	}

	service := application.NewFileService(storage)

	return &Module{
		service: service,
		storage: storage,
	}, nil
}

// Service returns the file service for use by other modules
func (m *Module) Service() *application.FileService {
	return m.service
}
