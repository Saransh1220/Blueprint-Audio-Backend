package gateway

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/gateway/apidocs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOpenAPICoversGatewayRoutes(t *testing.T) {
	var document struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(apidocs.YAML(), &document))

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	routesSource, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "routes.go"))
	require.NoError(t, err)

	routePattern := regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE) ([^" ]+)"`)
	matches := routePattern.FindAllStringSubmatch(string(routesSource), -1)
	require.NotEmpty(t, matches)

	for _, match := range matches {
		method, path := strings.ToLower(match[1]), match[2]
		operations, exists := document.Paths[path]
		require.Truef(t, exists, "%s %s is registered but missing from OpenAPI", strings.ToUpper(method), path)
		_, exists = operations[method]
		require.Truef(t, exists, "%s %s is registered but missing from OpenAPI", strings.ToUpper(method), path)
	}

	for _, operation := range []struct{ method, path string }{
		{method: "get", path: "/health"},
		{method: "get", path: "/metrics"},
	} {
		operations, exists := document.Paths[operation.path]
		require.True(t, exists, operation.path)
		_, exists = operations[operation.method]
		require.True(t, exists, strings.ToUpper(operation.method)+" "+operation.path)
	}
}
