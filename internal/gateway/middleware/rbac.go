package middleware

import (
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type Permission string

const (
	PermissionSuperAdmin Permission = "super_admin"
)

var rolePermissions = map[domain.SystemRole]map[Permission]bool{
	domain.SystemRoleSuperAdmin: {
		PermissionSuperAdmin: true,
	},
}

func HasPermission(systemRole string, permission Permission) bool {
	permissions, ok := rolePermissions[domain.SystemRole(systemRole)]
	if !ok {
		return false
	}
	return permissions[permission]
}

// This function `RequirePermission` is a method of the `AuthMiddleWare` struct. It takes a
// `Permission` type parameter called `permission` and an `http.Handler` called `next` as input
// parameters.
func (m *AuthMiddleWare) RequirePermission(permission Permission, next http.Handler) http.Handler {
	return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		systemRole, _ := r.Context().Value(ContextKeySystemRole).(string)
		if !HasPermission(systemRole, permission) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// This function `RequireSystemRole` is a method of the `AuthMiddleWare` struct. It takes in a slice of
// strings `roles` and an `http.Handler` called `next` as parameters.
func (m *AuthMiddleWare) RequireSystemRole(roles []string, next http.Handler) http.Handler {
	return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		systemRole, _ := r.Context().Value(ContextKeySystemRole).(string)
		for _, role := range roles {
			if systemRole == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}))
}

// RequireUserRole authenticates the request and permits only the requested
// application roles (for example, producer rather than artist).
func (m *AuthMiddleWare) RequireUserRole(roles []domain.UserRole, next http.Handler) http.Handler {
	return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(ContextKeyRole).(string)
		for _, allowed := range roles {
			if role == string(allowed) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}))
}
