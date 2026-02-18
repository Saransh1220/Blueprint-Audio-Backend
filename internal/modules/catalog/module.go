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
	repository *persistence.PgSpecRepository
	service    application.SpecService
	handler    *catalogHttp.SpecHandler
}

// NewModule creates and initializes the Catalog module
// NewModule creates and initializes the Catalog module
func NewModule(
	db *sqlx.DB,
	repository *persistence.PgSpecRepository, // Accept repository here
	fileService catalogHttp.FileService,
	analyticsService catalogHttp.AnalyticsService,
	notificationService *notificationApp.NotificationService,
	redisClient *redis.Client,
) *Module {
	service := application.NewSpecService(repository)
	handler := catalogHttp.NewSpecHandler(service, fileService, analyticsService, notificationService, redisClient)

	return &Module{
		repository: repository,
		service:    service,
		handler:    handler,
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
