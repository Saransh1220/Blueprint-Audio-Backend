package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	routesPath := flag.String("routes", "internal/gateway/routes.go", "Path to routes.go")
	outPath := flag.String("out", "internal/gateway/apidocs/openapi.yaml", "Output path for openapi.yaml")
	baseSpecPath := flag.String("base-spec", "internal/gateway/apidocs/openapi.yaml", "Path to existing spec to preserve custom definitions")
	checkOnly := flag.Bool("check", false, "Validate whether current spec is up to date without modifying")

	flag.Parse()

	log.Printf("[swaggergen] Parsing routes from %s...", *routesPath)
	parsedRoutes, err := ParseRoutesFile(*routesPath)
	if err != nil {
		log.Fatalf("Failed to parse routes.go: %v", err)
	}
	log.Printf("[swaggergen] Found %d route declarations in routes.go", len(parsedRoutes))

	// Load existing base spec if present
	var baseDoc *OpenAPIDoc
	if *baseSpecPath != "" {
		if content, err := os.ReadFile(*baseSpecPath); err == nil {
			var doc OpenAPIDoc
			if err := yaml.Unmarshal(content, &doc); err == nil {
				baseDoc = &doc
				log.Printf("[swaggergen] Loaded base spec with %d paths and %d schemas", len(baseDoc.Paths), len(baseDoc.Components.Schemas))
			}
		}
	}

	reflector := NewReflector()
	RegisterKnownTypes(reflector)

	endpoints := GetEndpointsRegistry()
	log.Printf("[swaggergen] Loaded %d endpoints from type-safe registry", len(endpoints))

	// Verify route coverage against routes.go
	registeredMap := make(map[string]bool)
	for _, ep := range endpoints {
		key := fmt.Sprintf("%s %s", strings.ToUpper(ep.Method), ep.Path)
		registeredMap[key] = true
	}

	missingInRegistry := 0
	for _, r := range parsedRoutes {
		key := fmt.Sprintf("%s %s", strings.ToUpper(r.Method), r.Path)
		if !registeredMap[key] {
			log.Printf("[swaggergen][WARNING] Route declared in routes.go is missing from swagger registry: %s", key)
			missingInRegistry++
		}
	}

	if missingInRegistry > 0 {
		log.Printf("[swaggergen][WARNING] Total %d routes declared in routes.go are not yet in swagger registry", missingInRegistry)
	} else {
		log.Printf("[swaggergen] 100%% of routes in routes.go are covered by registry!")
	}

	// Build OpenAPI document
	doc, err := BuildOpenAPIDoc(endpoints, reflector, baseDoc)
	if err != nil {
		log.Fatalf("Failed to build OpenAPI doc: %v", err)
	}

	// Serialize to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		log.Fatalf("Failed to encode YAML: %v", err)
	}

	generatedYAML := buf.Bytes()

	if *checkOnly {
		existing, err := os.ReadFile(*outPath)
		if err != nil {
			log.Fatalf("Check failed: unable to read %s: %v", *outPath, err)
		}

		// Normalize line endings for comparison
		normGen := strings.ReplaceAll(string(generatedYAML), "\r\n", "\n")
		normExist := strings.ReplaceAll(string(existing), "\r\n", "\n")

		if normGen != normExist {
			fmt.Fprintf(os.Stderr, "Error: %s is out of date! Run 'make swagger' to regenerate.\n", *outPath)
			os.Exit(1)
		}
		fmt.Printf("Success: %s is up to date.\n", *outPath)
		return
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		log.Fatalf("Failed to create dir %s: %v", filepath.Dir(*outPath), err)
	}

	if err := os.WriteFile(*outPath, generatedYAML, 0644); err != nil {
		log.Fatalf("Failed to write to %s: %v", *outPath, err)
	}

	log.Printf("[swaggergen] Successfully generated OpenAPI 3.0 specification at %s (%d bytes)", *outPath, len(generatedYAML))
}
