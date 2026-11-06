package legal

import (
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/infrastructure/persistence/postgres"
	legal_http "github.com/saransh1220/blueprint-audio/internal/modules/legal/interfaces/http"
)

type Module struct {
	service application.LegalService
	handler *legal_http.LegalHandler
}

func NewModule(db *sqlx.DB) *Module {
	docRepo := postgres.NewDocumentRepository(db)
	acceptanceRepo := postgres.NewAcceptanceRepository(db)
	service := application.NewLegalService(docRepo, acceptanceRepo)
	handler := legal_http.NewLegalHandler(service)

	return &Module{
		service: service,
		handler: handler,
	}
}

func (m *Module) Service() application.LegalService {
	return m.service
}

func (m *Module) HTTPHandler() *legal_http.LegalHandler {
	return m.handler
}
