package analytics

import (
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type Module struct {
	AnalyticsService application.AnalyticsService
	AnalyticsHandler *http.AnalyticsHandler
}

func NewModule(db *sqlx.DB, specRepo catalogDomain.SpecRepository, fileService http.FileService) *Module {
	repo := postgres.NewAnalyticsRepository(db)
	service := application.NewAnalyticsService(repo, specRepo)
	handler := http.NewAnalyticsHandler(service, specRepo, fileService)

	return &Module{
		AnalyticsService: service,
		AnalyticsHandler: handler,
	}
}
