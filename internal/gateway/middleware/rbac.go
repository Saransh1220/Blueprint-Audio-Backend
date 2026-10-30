package middleware

import (
	"net/http"
)

// WithRoleStore gives the middleware the role definitions Authorize checks
// against. Without one, Authorize denies every request (fail closed).
func (m *AuthMiddleWare) WithRoleStore(roles *RoleStore) *AuthMiddleWare {
	m.roles = roles
	return m
}

// Authorize authenticates the request and then requires the caller to hold
// every listed permission through their business role or system role.
//
// Authentication is RequireAuth; authorization is the permission check. When an
// account lookup is configured both roles come from the database, so a role
// change applies within the lookup cache TTL rather than at token expiry.
//
//	mux.Handle("GET /admin/users", authz(PermAdminUsersRead)(handler))
func (m *AuthMiddleWare) Authorize(perms ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m.roles == nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			role, _ := r.Context().Value(ContextKeyRole).(string)
			systemRole, _ := r.Context().Value(ContextKeySystemRole).(string)
			for _, p := range perms {
				if !m.roles.Can(p, role, systemRole) {
					http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		}))
	}
}
