package apidocs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openAPIYAML []byte

var (
	openAPIJSON     []byte
	openAPIJSONErr  error
	openAPIJSONOnce sync.Once
)

// Register exposes the interactive Swagger UI and the underlying OpenAPI document.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusPermanentRedirect)
	})
	mux.HandleFunc("GET /docs/", serveSwaggerUI)
	mux.HandleFunc("GET /openapi.yaml", serveYAML)
	mux.HandleFunc("GET /openapi.json", serveJSON)
}

// YAML returns a copy of the embedded OpenAPI source document.
func YAML() []byte {
	return append([]byte(nil), openAPIYAML...)
}

func serveYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(openAPIYAML)
}

func serveJSON(w http.ResponseWriter, _ *http.Request) {
	document, err := JSON()
	if err != nil {
		http.Error(w, "failed to render OpenAPI document", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(document)
}

// JSON converts the embedded YAML document to JSON once and returns a copy.
func JSON() ([]byte, error) {
	openAPIJSONOnce.Do(func() {
		var document any
		if err := yaml.Unmarshal(openAPIYAML, &document); err != nil {
			openAPIJSONErr = fmt.Errorf("parse embedded OpenAPI YAML: %w", err)
			return
		}
		openAPIJSON, openAPIJSONErr = json.Marshal(document)
		if openAPIJSONErr != nil {
			openAPIJSONErr = fmt.Errorf("encode OpenAPI JSON: %w", openAPIJSONErr)
		}
	})
	return append([]byte(nil), openAPIJSON...), openAPIJSONErr
}

func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'none'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https:; connect-src 'self'; font-src https://cdn.jsdelivr.net data:",
	)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

const swaggerUIHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="description" content="Waveyard backend API documentation">
  <title>Waveyard API Documentation</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>
    html { box-sizing: border-box; overflow-y: scroll; }
    *, *::before, *::after { box-sizing: inherit; }
    body { margin: 0; background: #0d0d0d; }
    .swagger-ui .topbar { background: #151515; border-bottom: 1px solid #ff4d68; }
    .swagger-ui .topbar .download-url-wrapper .select-label { color: #f0e9d6; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin="anonymous"></script>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js" crossorigin="anonymous"></script>
  <script>
    window.addEventListener('load', function () {
      window.ui = SwaggerUIBundle({
        url: '/openapi.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        displayRequestDuration: true,
        persistAuthorization: true,
        validatorUrl: null,
        withCredentials: true,
        filter: true,
        tryItOutEnabled: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        layout: 'StandaloneLayout'
      });
    });
  </script>
</body>
</html>`
