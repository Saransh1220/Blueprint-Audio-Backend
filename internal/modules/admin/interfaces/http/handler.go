package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type AdminHandler struct {
	db       *sqlx.DB
	userRepo authDomain.UserRepository
}

type adminUser struct {
	ID            uuid.UUID             `json:"id" db:"id"`
	Email         string                `json:"email" db:"email"`
	Name          string                `json:"name" db:"name"`
	DisplayName   *string               `json:"display_name" db:"display_name"`
	Role          authDomain.UserRole   `json:"role" db:"role"`
	SystemRole    authDomain.SystemRole `json:"system_role" db:"system_role"`
	Status        authDomain.UserStatus `json:"status" db:"status"`
	EmailVerified bool                  `json:"email_verified" db:"email_verified"`
	CreatedAt     time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at" db:"updated_at"`
}

type auditLog struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	ActorID      *uuid.UUID      `json:"actor_id" db:"actor_id"`
	ActorEmail   *string         `json:"actor_email" db:"actor_email"`
	Action       string          `json:"action" db:"action"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   *uuid.UUID      `json:"resource_id" db:"resource_id"`
	BeforeState  json.RawMessage `json:"before_state" db:"before_state"`
	AfterState   json.RawMessage `json:"after_state" db:"after_state"`
	IPAddress    *string         `json:"ip_address" db:"ip_address"`
	UserAgent    *string         `json:"user_agent" db:"user_agent"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

func NewAdminHandler(db *sqlx.DB, userRepo authDomain.UserRepository) *AdminHandler {
	return &AdminHandler{db: db, userRepo: userRepo}
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r, 50, 100)
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	query := `SELECT id, email, name, display_name, role, system_role, status, email_verified, created_at, updated_at FROM users`
	countQuery := `SELECT COUNT(*) FROM users`
	args := []any{}
	if search != "" {
		query += ` WHERE email ILIKE $1 OR name ILIKE $1 OR display_name ILIKE $1`
		countQuery += ` WHERE email ILIKE $1 OR name ILIKE $1 OR display_name ILIKE $1`
		args = append(args, "%"+search+"%")
	}

	query += ` ORDER BY created_at DESC`
	query += ` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	queryArgs := append(args, limit, offset)

	var users []adminUser
	if err := h.db.SelectContext(r.Context(), &users, query, queryArgs...); err != nil {
		http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
		return
	}

	var total int
	if err := h.db.GetContext(r.Context(), &total, countQuery, args...); err != nil {
		http.Error(w, `{"error":"failed to count users"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(users, total, limit, offset))
}

func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var user adminUser
	err = h.db.GetContext(r.Context(), &user, `SELECT id, email, name, display_name, role, system_role, status, email_verified, created_at, updated_at FROM users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"failed to fetch user"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AdminHandler) UpdateUserSystemRole(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		SystemRole authDomain.SystemRole `json:"system_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.SystemRole != authDomain.SystemRoleUser && req.SystemRole != authDomain.SystemRoleSuperAdmin {
		http.Error(w, `{"error":"invalid system role"}`, http.StatusBadRequest)
		return
	}

	before, err := h.getUser(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	if before.SystemRole == authDomain.SystemRoleSuperAdmin && req.SystemRole != authDomain.SystemRoleSuperAdmin {
		if err := h.ensureNotLastSuperAdmin(r.Context(), id); err != nil {
			http.Error(w, `{"error":"cannot remove the last super admin"}`, http.StatusBadRequest)
			return
		}
	}

	if err := h.userRepo.UpdateSystemRole(r.Context(), id, req.SystemRole); err != nil {
		http.Error(w, `{"error":"failed to update system role"}`, http.StatusInternalServerError)
		return
	}
	after, _ := h.getUser(r.Context(), id)
	h.audit(r, "users.set_system_role", "user", &id, before, after)
	writeJSON(w, http.StatusOK, after)
}

func (h *AdminHandler) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Status authDomain.UserStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Status != authDomain.UserStatusActive && req.Status != authDomain.UserStatusSuspended {
		http.Error(w, `{"error":"invalid status"}`, http.StatusBadRequest)
		return
	}

	before, err := h.getUser(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	if before.SystemRole == authDomain.SystemRoleSuperAdmin && req.Status == authDomain.UserStatusSuspended {
		if err := h.ensureNotLastSuperAdmin(r.Context(), id); err != nil {
			http.Error(w, `{"error":"cannot suspend the last super admin"}`, http.StatusBadRequest)
			return
		}
	}
	if err := h.userRepo.UpdateStatus(r.Context(), id, req.Status); err != nil {
		http.Error(w, `{"error":"failed to update status"}`, http.StatusInternalServerError)
		return
	}
	after, _ := h.getUser(r.Context(), id)
	h.audit(r, "users.set_status", "user", &id, before, after)
	writeJSON(w, http.StatusOK, after)
}

func (h *AdminHandler) ListSpecs(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r, 50, 100)
	var specs []map[string]any
	query := `SELECT s.id, s.producer_id, u.email AS producer_email, COALESCE(u.display_name, u.name) AS producer_name, s.title, s.category, s.base_price, s.processing_status, s.is_deleted, s.created_at, s.updated_at FROM specs s JOIN users u ON s.producer_id = u.id ORDER BY s.created_at DESC LIMIT $1 OFFSET $2`
	rows, err := h.db.QueryxContext(r.Context(), query, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"failed to list specs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		row := map[string]any{}
		if err := rows.MapScan(row); err != nil {
			http.Error(w, `{"error":"failed to scan specs"}`, http.StatusInternalServerError)
			return
		}
		normalizeMap(row)
		specs = append(specs, row)
	}
	var total int
	_ = h.db.GetContext(r.Context(), &total, `SELECT COUNT(*) FROM specs`)
	writeJSON(w, http.StatusOK, pageResponse(specs, total, limit, offset))
}

func (h *AdminHandler) UpdateSpec(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"invalid spec id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Title     *string  `json:"title"`
		BasePrice *float64 `json:"base_price"`
		IsDeleted *bool    `json:"is_deleted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	before := h.getSpecState(r.Context(), id)
	if before == nil {
		http.Error(w, `{"error":"spec not found"}`, http.StatusNotFound)
		return
	}

	var title any
	if req.Title != nil {
		title = *req.Title
	}
	var basePrice any
	if req.BasePrice != nil {
		basePrice = *req.BasePrice
	}
	var isDeleted any
	if req.IsDeleted != nil {
		isDeleted = *req.IsDeleted
	}

	result, err := h.db.ExecContext(
		r.Context(),
		`UPDATE specs
		 SET title = COALESCE($1, title),
		     base_price = COALESCE($2, base_price),
		     is_deleted = COALESCE($3, is_deleted),
		     deleted_at = CASE WHEN COALESCE($3, is_deleted) THEN COALESCE(deleted_at, NOW()) ELSE NULL END,
		     updated_at = NOW()
		 WHERE id = $4`,
		title,
		basePrice,
		isDeleted,
		id,
	)
	if err != nil {
		http.Error(w, `{"error":"failed to update spec"}`, http.StatusInternalServerError)
		return
	}
	if rows, err := result.RowsAffected(); err != nil {
		http.Error(w, `{"error":"failed to confirm spec update"}`, http.StatusInternalServerError)
		return
	} else if rows == 0 {
		http.Error(w, `{"error":"spec not found"}`, http.StatusNotFound)
		return
	}
	after := h.getSpecState(r.Context(), id)
	h.audit(r, "specs.update", "spec", &id, before, after)
	writeJSON(w, http.StatusOK, after)
}

func (h *AdminHandler) DeleteSpec(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"invalid spec id"}`, http.StatusBadRequest)
		return
	}
	before := h.getSpecState(r.Context(), id)
	result, err := h.db.ExecContext(r.Context(), `UPDATE specs SET is_deleted = TRUE, deleted_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		http.Error(w, `{"error":"failed to delete spec"}`, http.StatusInternalServerError)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		http.Error(w, `{"error":"spec not found"}`, http.StatusNotFound)
		return
	}
	after := h.getSpecState(r.Context(), id)
	h.audit(r, "specs.delete", "spec", &id, before, after)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r, 50, 100)
	rows := []map[string]any{}
	query := `SELECT o.id, o.user_id, u.email AS buyer_email, o.spec_id, s.title AS spec_title, o.license_type, o.amount, o.currency, o.status, o.created_at FROM orders o JOIN users u ON o.user_id = u.id JOIN specs s ON o.spec_id = s.id ORDER BY o.created_at DESC LIMIT $1 OFFSET $2`
	if err := h.selectMaps(r.Context(), &rows, query, limit, offset); err != nil {
		http.Error(w, `{"error":"failed to list orders"}`, http.StatusInternalServerError)
		return
	}
	var total int
	_ = h.db.GetContext(r.Context(), &total, `SELECT COUNT(*) FROM orders`)
	writeJSON(w, http.StatusOK, pageResponse(rows, total, limit, offset))
}

func (h *AdminHandler) ListLicenses(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r, 50, 100)
	rows := []map[string]any{}
	query := `SELECT l.id, l.user_id, u.email AS user_email, l.spec_id, s.title AS spec_title, l.license_type, l.purchase_price, l.is_active, l.is_revoked, l.downloads_count, l.issued_at FROM licenses l JOIN users u ON l.user_id = u.id JOIN specs s ON l.spec_id = s.id ORDER BY l.issued_at DESC LIMIT $1 OFFSET $2`
	if err := h.selectMaps(r.Context(), &rows, query, limit, offset); err != nil {
		http.Error(w, `{"error":"failed to list licenses"}`, http.StatusInternalServerError)
		return
	}
	var total int
	_ = h.db.GetContext(r.Context(), &total, `SELECT COUNT(*) FROM licenses`)
	writeJSON(w, http.StatusOK, pageResponse(rows, total, limit, offset))
}

func (h *AdminHandler) AnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	stats := map[string]any{}
	var users, producers, specs, orders, paidOrders int
	_ = h.db.GetContext(r.Context(), &users, `SELECT COUNT(*) FROM users`)
	_ = h.db.GetContext(r.Context(), &producers, `SELECT COUNT(*) FROM users WHERE role = 'producer'`)
	_ = h.db.GetContext(r.Context(), &specs, `SELECT COUNT(*) FROM specs`)
	_ = h.db.GetContext(r.Context(), &orders, `SELECT COUNT(*) FROM orders`)
	_ = h.db.GetContext(r.Context(), &paidOrders, `SELECT COUNT(*) FROM orders WHERE status = 'paid'`)
	var revenue int
	_ = h.db.GetContext(r.Context(), &revenue, `SELECT COALESCE(SUM(amount), 0) FROM orders WHERE status = 'paid'`)
	stats["users"] = users
	stats["producers"] = producers
	stats["specs"] = specs
	stats["orders"] = orders
	stats["paid_orders"] = paidOrders
	stats["revenue"] = revenue
	writeJSON(w, http.StatusOK, stats)
}

func (h *AdminHandler) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r, 50, 100)
	var logs []auditLog
	query := `SELECT a.id, a.actor_id, u.email AS actor_email, a.action, a.resource_type, a.resource_id, a.before_state, a.after_state, a.ip_address, a.user_agent, a.created_at FROM admin_audit_logs a LEFT JOIN users u ON a.actor_id = u.id ORDER BY a.created_at DESC LIMIT $1 OFFSET $2`
	if err := h.db.SelectContext(r.Context(), &logs, query, limit, offset); err != nil {
		http.Error(w, `{"error":"failed to list audit logs"}`, http.StatusInternalServerError)
		return
	}
	var total int
	_ = h.db.GetContext(r.Context(), &total, `SELECT COUNT(*) FROM admin_audit_logs`)
	writeJSON(w, http.StatusOK, pageResponse(logs, total, limit, offset))
}

func (h *AdminHandler) getUser(ctx context.Context, id uuid.UUID) (*adminUser, error) {
	var user adminUser
	err := h.db.GetContext(ctx, &user, `SELECT id, email, name, display_name, role, system_role, status, email_verified, created_at, updated_at FROM users WHERE id = $1`, id)
	return &user, err
}

func (h *AdminHandler) ensureNotLastSuperAdmin(ctx context.Context, id uuid.UUID) error {
	count, err := h.userRepo.CountBySystemRole(ctx, authDomain.SystemRoleSuperAdmin)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errors.New("last super admin")
	}
	return nil
}

func (h *AdminHandler) audit(r *http.Request, action, resourceType string, resourceID *uuid.UUID, before, after any) {
	actorID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	ip := clientIP(r)
	userAgent := r.UserAgent()
	_, _ = h.db.ExecContext(r.Context(), `INSERT INTO admin_audit_logs (actor_id, action, resource_type, resource_id, before_state, after_state, ip_address, user_agent) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, actorID, action, resourceType, resourceID, beforeJSON, afterJSON, ip, userAgent)
}

func (h *AdminHandler) getSpecState(ctx context.Context, id uuid.UUID) map[string]any {
	rows := []map[string]any{}
	_ = h.selectMaps(ctx, &rows, `SELECT id, producer_id, title, category, base_price, processing_status, is_deleted, deleted_at, updated_at FROM specs WHERE id = $1`, id)
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func (h *AdminHandler) selectMaps(ctx context.Context, dest *[]map[string]any, query string, args ...any) error {
	rows, err := h.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		row := map[string]any{}
		if err := rows.MapScan(row); err != nil {
			return err
		}
		normalizeMap(row)
		*dest = append(*dest, row)
	}
	return rows.Err()
}

func normalizeMap(row map[string]any) {
	for key, value := range row {
		if bytes, ok := value.([]byte); ok {
			row[key] = string(bytes)
		}
	}
}

func pagination(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	return limit, (page - 1) * limit
}

func pageResponse(data any, total, limit, offset int) map[string]any {
	page := offset/limit + 1
	return map[string]any{
		"data": data,
		"metadata": map[string]any{
			"total":    total,
			"page":     page,
			"per_page": limit,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return r.RemoteAddr
}
