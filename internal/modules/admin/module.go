package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	admin_http "github.com/saransh1220/blueprint-audio/internal/modules/admin/interfaces/http"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type AuditWriter struct {
	db *sqlx.DB
}

func NewAuditWriter(db *sqlx.DB) *AuditWriter {
	return &AuditWriter{db: db}
}

func (w *AuditWriter) Log(ctx context.Context, actorID *uuid.UUID, action, resourceType, resourceID string, beforeState, afterState any, ip, userAgent string) error {
	beforeJSON, _ := json.Marshal(beforeState)
	afterJSON, _ := json.Marshal(afterState)
	var resID *uuid.UUID
	if resourceID != "" {
		if parsed, err := uuid.Parse(resourceID); err == nil {
			resID = &parsed
		}
	}
	_, err := w.db.ExecContext(ctx, `INSERT INTO admin_audit_logs (actor_id, action, resource_type, resource_id, before_state, after_state, ip_address, user_agent) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, actorID, action, resourceType, resID, beforeJSON, afterJSON, ip, userAgent)
	return err
}

type Module struct {
	handler     *admin_http.AdminHandler
	auditWriter *AuditWriter
}

func NewModule(db *sqlx.DB, userRepo authDomain.UserRepository, roles *middleware.RoleStore) *Module {
	return &Module{
		handler:     admin_http.NewAdminHandler(db, userRepo).WithRoleStore(roles),
		auditWriter: NewAuditWriter(db),
	}
}

func (m *Module) HTTPHandler() *admin_http.AdminHandler {
	return m.handler
}

func (m *Module) AuditWriter() *AuditWriter {
	return m.auditWriter
}
