package middleware

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Permission strings look like "spec:upload" or "admin:users:read".
var permissionToken = regexp.MustCompile(`[a-z_]+(?::[a-z_]+)+`)

// Matches the quoted array literal of a role seed, e.g. '{spec:upload,...}'.
var seededArray = regexp.MustCompile(`'\{([a-z_:,]*)\}'`)

// TestPermissionsMatchMigrations keeps the Permission constants and the
// permission strings written into role rows by migrations in lockstep. A typo
// on either side would otherwise silently grant or require nothing.
func TestPermissionsMatchMigrations(t *testing.T) {
	files, err := filepath.Glob("../../../db/migrations/*.up.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files, "migrations directory not found")

	seeded := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		sql := string(raw)
		// Only look at migrations that touch the roles table.
		if !regexp.MustCompile(`(?i)\broles\b`).MatchString(sql) {
			continue
		}
		for _, m := range seededArray.FindAllStringSubmatch(sql, -1) {
			for _, p := range permissionToken.FindAllString(m[1], -1) {
				seeded[p] = true
			}
		}
		// array_append(permissions, 'x:y') grants
		for _, m := range regexp.MustCompile(`array_append\(\s*permissions\s*,\s*'([a-z_:]+)'\s*\)`).FindAllStringSubmatch(sql, -1) {
			seeded[m[1]] = true
		}
	}

	known := map[string]bool{}
	for _, p := range AllPermissions {
		known[string(p)] = true
	}

	var unknownInSQL, unusedInSQL []string
	for p := range seeded {
		if !known[p] {
			unknownInSQL = append(unknownInSQL, p)
		}
	}
	for p := range known {
		if !seeded[p] {
			unusedInSQL = append(unusedInSQL, p)
		}
	}
	sort.Strings(unknownInSQL)
	sort.Strings(unusedInSQL)

	assert.Empty(t, unknownInSQL, "migrations grant permissions that have no Permission constant (typo?)")
	assert.Empty(t, unusedInSQL, "Permission constants that no migration grants to any role")
}

func TestAllPermissionsAreUnique(t *testing.T) {
	seen := map[Permission]bool{}
	for _, p := range AllPermissions {
		assert.False(t, seen[p], "duplicate permission %s", p)
		seen[p] = true
	}
}
