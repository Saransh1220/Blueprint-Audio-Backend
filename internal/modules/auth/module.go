package auth

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	fileApp "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
)

// Module represents the Auth module
type Module struct {
	service    *application.AuthService
	repository *postgres.PgUserRepository
	handler    *auth_http.AuthHandler
}

// NewModule creates and initializes the Auth module
func NewModule(db *sqlx.DB, jwtSecret string, jwtExpiry time.Duration, fileService *fileApp.FileService, googleClientID string) (*Module, error) {
	repository := postgres.NewUserRepository(db)
	service := application.NewAuthService(repository, jwtSecret, jwtExpiry)
	handler := auth_http.NewAuthHandler(service, fileService, googleClientID)

	return &Module{
		service:    service,
		repository: repository,
		handler:    handler,
	}, nil
}

// Service returns the auth service for use by the gateway layer
func (m *Module) Service() *application.AuthService {
	return m.service
}

// UserFinder returns the user finder interface for use by other modules
func (m *Module) UserFinder() domain.UserFinder {
	return m.repository
}

// UserRepository returns the user repository (for handlers still using old interfaces)
func (m *Module) UserRepository() *postgres.PgUserRepository {
	return m.repository
}

// HTTPHandler returns the HTTP handler for the auth module
func (m *Module) HTTPHandler() *auth_http.AuthHandler {
	return m.handler
}
