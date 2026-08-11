package apidocs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedOpenAPIDocument(t *testing.T) {
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(YAML(), &document))
	require.Equal(t, "3.0.3", document.OpenAPI)
	require.NotEmpty(t, document.Paths)
	require.Contains(t, document.Paths, "/specs")
	require.Contains(t, document.Paths, "/spec-uploads/{id}/files/{assetID}/complete")
	require.Contains(t, document.Paths, "/admin/audit-log")

	encoded, err := JSON()
	require.NoError(t, err)
	var jsonDocument map[string]any
	require.NoError(t, json.Unmarshal(encoded, &jsonDocument))
	require.Equal(t, "3.0.3", jsonDocument["openapi"])
}

func TestDocumentationHandlers(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	tests := []struct {
		path        string
		status      int
		contentType string
		body        string
	}{
		{path: "/docs/", status: http.StatusOK, contentType: "text/html", body: "SwaggerUIBundle"},
		{path: "/openapi.yaml", status: http.StatusOK, contentType: "application/yaml", body: "openapi: 3.0.3"},
		{path: "/openapi.json", status: http.StatusOK, contentType: "application/json", body: `"openapi":"3.0.3"`},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			require.Equal(t, test.status, response.Code)
			require.Contains(t, response.Header().Get("Content-Type"), test.contentType)
			require.Contains(t, response.Body.String(), test.body)
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/docs", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	require.Equal(t, http.StatusPermanentRedirect, response.Code)
	require.Equal(t, "/docs/", response.Header().Get("Location"))
}

func TestOperationIDsAreUnique(t *testing.T) {
	var document struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(YAML(), &document))

	seen := map[string]string{}
	for path, pathItem := range document.Paths {
		for method, value := range pathItem {
			if !isHTTPMethod(method) {
				continue
			}
			operation, ok := value.(map[string]any)
			require.True(t, ok, strings.ToUpper(method)+" "+path)
			operationID, _ := operation["operationId"].(string)
			require.NotEmpty(t, operationID, strings.ToUpper(method)+" "+path)
			if previous, exists := seen[operationID]; exists {
				t.Fatalf("operationId %q is shared by %s and %s %s", operationID, previous, strings.ToUpper(method), path)
			}
			seen[operationID] = strings.ToUpper(method) + " " + path
		}
	}
}

func TestOpenAPIIsGenerationReady(t *testing.T) {
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(YAML(), &document))

	components := requireMap(t, document, "components")
	schemas := requireMap(t, components, "schemas")
	parameters := requireMap(t, components, "parameters")

	for _, removedPlaceholder := range []string{"GenericPage", "MediaURL"} {
		require.NotContains(t, schemas, removedPlaceholder, "generic schemas hide the wire contract and produce unusable generated DTOs")
	}
	require.NotContains(t, parameters, "DisplayCurrency", "the backend derives currency from country headers")

	for _, exactSchema := range []string{
		"Money", "Spec", "PublicUser", "ProducerOrder", "AdminSpec", "AdminOrder",
		"AdminLicense", "AdminAuditLog", "DailyRevenueStat", "BeatRankingMetrics",
	} {
		require.Contains(t, schemas, exactSchema)
	}

	walkOpenAPI(t, document, func(key string, value any) {
		if key != "$ref" {
			return
		}
		reference, ok := value.(string)
		require.True(t, ok)
		const schemaPrefix = "#/components/schemas/"
		if strings.HasPrefix(reference, schemaPrefix) {
			require.Contains(t, schemas, strings.TrimPrefix(reference, schemaPrefix), "unresolved schema reference %s", reference)
		}
	})
}

func requireMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	require.True(t, ok, "%s must be an object", key)
	return value
}

func walkOpenAPI(t *testing.T, value any, visit func(string, any)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			visit(key, child)
			walkOpenAPI(t, child, visit)
		}
	case []any:
		for _, child := range typed {
			walkOpenAPI(t, child, visit)
		}
	}
}

func isHTTPMethod(value string) bool {
	switch strings.ToLower(value) {
	case "get", "post", "put", "patch", "delete", "head", "options", "trace":
		return true
	default:
		return false
	}
}
