package notification

import (
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/websocket"
	notification_http "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
)

type Module struct {
	service *application.NotificationService
	handler *notification_http.NotificationHandler
	hub     *websocket.Hub
}

func NewModule(db *sqlx.DB) *Module {
	repo := postgres.NewPgNotificationRepository(db)
	hub := websocket.NewHub()
	go hub.Run()

	service := application.NewNotificationService(repo, hub)
	handler := notification_http.NewNotificationHandler(service, hub)

	return &Module{
		service: service,
		handler: handler,
		hub:     hub,
	}
}

func (m *Module) HTTPHandler() *notification_http.NotificationHandler {
	return m.handler
}

func (m *Module) Service() *application.NotificationService {
	return m.service
}
