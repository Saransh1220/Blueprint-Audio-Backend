package user

import (
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	fileApp "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/user/application"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
)

// Module represents the User module
type Module struct {
	service *application.UserService
	handler *user_http.UserHandler
}

// NewModule creates and initializes the User module
func NewModule(repo authDomain.UserRepository, fileService *fileApp.FileService) *Module {
	service := application.NewUserService(repo)
	handler := user_http.NewUserHandler(service, fileService)

	return &Module{
		service: service,
		handler: handler,
	}
}

// HTTPHandler returns the HTTP handler for the user module
func (m *Module) HTTPHandler() *user_http.UserHandler {
	return m.handler
}

// Service returns the user service
func (m *Module) Service() *application.UserService {
	return m.service
}
