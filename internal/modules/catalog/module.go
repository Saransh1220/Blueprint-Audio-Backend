package catalog

import (
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	persistence "github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	catalogHttp "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	notificationApp "github.com/saransh1220/blueprint-audio/internal/modules/notification/application"
)

// Module represents the Catalog module
type Module struct {
	repository       *persistence.PgSpecRepository
	uploadRepository *persistence.PgSpecUploadRepository
	service          application.SpecService
	uploadService    application.SpecUploadService
	handler          *catalogHttp.SpecHandler
	uploadHandler    *catalogHttp.SpecUploadHandler
}

type FileService interface {
	catalogHttp.FileService
	application.SpecObjectStore
}

// NewModule creates and initializes the Catalog module
func NewModule(
	db *sqlx.DB,
	repository *persistence.PgSpecRepository, // Accept repository here
	fileService FileService,
	analyticsService catalogHttp.AnalyticsService,
	notificationService *notificationApp.NotificationService,
	redisClient *redis.Client,
) *Module {
	service := application.NewSpecService(repository)
	uploadRepository := persistence.NewSpecUploadRepository(db)
	uploadService := application.NewSpecUploadService(uploadRepository, repository, fileService)
	handler := catalogHttp.NewSpecHandler(service, fileService, analyticsService, notificationService, redisClient)
	uploadHandler := catalogHttp.NewSpecUploadHandler(uploadService)

	return &Module{
		repository:       repository,
		uploadRepository: uploadRepository,
		service:          service,
		uploadService:    uploadService,
		handler:          handler,
		uploadHandler:    uploadHandler,
	}
}

// Repository returns the spec repository for use by the gateway layer
func (m *Module) Repository() *persistence.PgSpecRepository {
	return m.repository
}

// SpecFinder returns the spec finder interface for use by other modules (Payment, Analytics)
func (m *Module) SpecFinder() domain.SpecFinder {
	return m.repository
}

// Service returns the spec service
func (m *Module) Service() application.SpecService {
	return m.service
}

// HTTPHandler returns the HTTP handler
func (m *Module) HTTPHandler() *catalogHttp.SpecHandler {
	return m.handler
}

func (m *Module) UploadRepository() *persistence.PgSpecUploadRepository {
	return m.uploadRepository
}

func (m *Module) UploadService() application.SpecUploadService {
	return m.uploadService
}

func (m *Module) UploadHTTPHandler() *catalogHttp.SpecUploadHandler {
	return m.uploadHandler
}
