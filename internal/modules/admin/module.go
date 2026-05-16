package admin

import (
	"github.com/jmoiron/sqlx"
	admin_http "github.com/saransh1220/blueprint-audio/internal/modules/admin/interfaces/http"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type Module struct {
	handler *admin_http.AdminHandler
}

func NewModule(db *sqlx.DB, userRepo authDomain.UserRepository) *Module {
	return &Module{
		handler: admin_http.NewAdminHandler(db, userRepo),
	}
}

func (m *Module) HTTPHandler() *admin_http.AdminHandler {
	return m.handler
}
