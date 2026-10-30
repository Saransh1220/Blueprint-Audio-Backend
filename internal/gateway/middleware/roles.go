package middleware

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// RoleKind separates the two role axes a user has: a business role (artist,
// producer) and a system role (user, super_admin).
type RoleKind string

const (
	RoleKindBusiness RoleKind = "business"
	RoleKindSystem   RoleKind = "system"
)

// RoleDef is one row of the `roles` table.
type RoleDef struct {
	ID          int16        `json:"id"`
	Name        string       `json:"name"`
	Kind        RoleKind     `json:"kind"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

// RoleLoader reads the current role definitions from their source of truth.
type RoleLoader func(ctx context.Context) ([]RoleDef, error)

// RoleStore is an in-memory, periodically refreshed copy of the roles table so
// that authorization never queries the database per request.
type RoleStore struct {
	loader RoleLoader

	mu      sync.RWMutex
	byName  map[string]map[Permission]struct{}
	kinds   map[string]RoleKind
	ordered []RoleDef
}

func NewRoleStore(loader RoleLoader) *RoleStore {
	return &RoleStore{loader: loader}
}

// NewStaticRoleStore builds a store from fixed definitions. Intended for tests.
func NewStaticRoleStore(defs ...RoleDef) *RoleStore {
	s := NewRoleStore(func(context.Context) ([]RoleDef, error) { return defs, nil })
	_ = s.Reload(context.Background())
	return s
}

// NewPgRoleLoader loads role definitions from the `roles` table.
func NewPgRoleLoader(db *sqlx.DB) RoleLoader {
	return func(ctx context.Context) ([]RoleDef, error) {
		rows, err := db.QueryxContext(ctx, `SELECT id, name, kind, description, permissions FROM roles ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var defs []RoleDef
		for rows.Next() {
			var (
				def   RoleDef
				kind  string
				perms pq.StringArray
			)
			if err := rows.Scan(&def.ID, &def.Name, &kind, &def.Description, &perms); err != nil {
				return nil, err
			}
			def.Kind = RoleKind(kind)
			for _, p := range perms {
				def.Permissions = append(def.Permissions, Permission(p))
			}
			defs = append(defs, def)
		}
		return defs, rows.Err()
	}
}

// Reload replaces the cached definitions. On error the previous definitions
// stay in effect.
func (s *RoleStore) Reload(ctx context.Context) error {
	defs, err := s.loader(ctx)
	if err != nil {
		return err
	}
	if len(defs) == 0 {
		return fmt.Errorf("roles table is empty; refusing to drop all permissions")
	}

	byName := make(map[string]map[Permission]struct{}, len(defs))
	kinds := make(map[string]RoleKind, len(defs))
	for _, def := range defs {
		set := make(map[Permission]struct{}, len(def.Permissions))
		for _, p := range def.Permissions {
			if !isKnownPermission(p) {
				log.Printf("RoleStore: role %q holds permission %q that no code requires (typo or stale?)", def.Name, p)
			}
			set[p] = struct{}{}
		}
		byName[def.Name] = set
		kinds[def.Name] = def.Kind
	}

	s.mu.Lock()
	s.byName, s.kinds, s.ordered = byName, kinds, defs
	s.mu.Unlock()
	return nil
}

// Start reloads the definitions every interval until ctx is cancelled, so a
// permission granted by migration takes effect without restarting the server.
func (s *RoleStore) Start(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.Reload(ctx); err != nil {
					log.Printf("RoleStore: reload failed, keeping previous roles: %v", err)
				}
			}
		}
	}()
}

// Can reports whether any of the named roles holds the permission. Unknown
// roles hold nothing.
func (s *RoleStore) Can(p Permission, roleNames ...string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, name := range roleNames {
		if _, ok := s.byName[name][p]; ok {
			return true
		}
	}
	return false
}

// HasRole reports whether a role with that name and kind exists.
func (s *RoleStore) HasRole(name string, kind RoleKind) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.kinds[name]
	return ok && k == kind
}

// List returns a copy of every role definition, ordered by id.
func (s *RoleStore) List() []RoleDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RoleDef, len(s.ordered))
	for i, def := range s.ordered {
		// Non-nil so a role with no permissions serialises as [] rather than null.
		def.Permissions = append(make([]Permission, 0, len(def.Permissions)), def.Permissions...)
		sort.Slice(def.Permissions, func(a, b int) bool { return def.Permissions[a] < def.Permissions[b] })
		out[i] = def
	}
	return out
}
