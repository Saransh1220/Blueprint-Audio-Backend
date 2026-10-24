package main

import (
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Property represents an OpenAPI 3.0 schema property.
type Property struct {
	Type                 string               `yaml:"type,omitempty" json:"type,omitempty"`
	Format               string               `yaml:"format,omitempty" json:"format,omitempty"`
	Description          string               `yaml:"description,omitempty" json:"description,omitempty"`
	Nullable             bool                 `yaml:"nullable,omitempty" json:"nullable,omitempty"`
	Ref                  string               `yaml:"$ref,omitempty" json:"$ref,omitempty"`
	Items                *Property            `yaml:"items,omitempty" json:"items,omitempty"`
	Properties           map[string]*Property `yaml:"properties,omitempty" json:"properties,omitempty"`
	Required             []string             `yaml:"required,omitempty" json:"required,omitempty"`
	AdditionalProperties any                  `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
	Enum                 []any                `yaml:"enum,omitempty" json:"enum,omitempty"`
	Example              any                  `yaml:"example,omitempty" json:"example,omitempty"`
}

// Schema represents an OpenAPI 3.0 Schema Object.
type Schema struct {
	Type                 string               `yaml:"type,omitempty" json:"type,omitempty"`
	Format               string               `yaml:"format,omitempty" json:"format,omitempty"`
	Description          string               `yaml:"description,omitempty" json:"description,omitempty"`
	Nullable             bool                 `yaml:"nullable,omitempty" json:"nullable,omitempty"`
	Ref                  string               `yaml:"$ref,omitempty" json:"$ref,omitempty"`
	Required             []string             `yaml:"required,omitempty" json:"required,omitempty"`
	Properties           map[string]*Property `yaml:"properties,omitempty" json:"properties,omitempty"`
	Items                *Property            `yaml:"items,omitempty" json:"items,omitempty"`
	AdditionalProperties any                  `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
	Enum                 []any                `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// Reflector turns Go types into OpenAPI 3.0 schemas and collects definitions into a component pool.
type Reflector struct {
	Schemas map[string]*Schema
}

// NewReflector creates a new Reflector.
func NewReflector() *Reflector {
	return &Reflector{
		Schemas: make(map[string]*Schema),
	}
}

// RegisterStruct registers a struct type into the components schemas.
// Returns the schema reference name (e.g. "#/components/schemas/Name").
func (r *Reflector) RegisterStruct(name string, t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return ""
	}

	if _, exists := r.Schemas[name]; exists {
		return "#/components/schemas/" + name
	}

	// Placeholder to handle recursive definitions gracefully
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Property),
		Required:   []string{},
	}
	r.Schemas[name] = schema

	r.fillStructProperties(t, schema)
	return "#/components/schemas/" + name
}

func (r *Reflector) fillStructProperties(t reflect.Type, schema *Schema) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle embedded anonymous struct
		if field.Anonymous {
			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				r.fillStructProperties(ft, schema)
				continue
			}
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		propName := field.Name
		omitempty := false
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omitempty = true
				}
			}
		}

		prop := r.typeToProperty(field.Type, field.Tag)
		schema.Properties[propName] = prop

		// In OpenAPI standard, if omitempty or pointer or nullable is NOT present, it's required
		if !omitempty && field.Type.Kind() != reflect.Pointer && !prop.Nullable {
			schema.Required = append(schema.Required, propName)
		}
	}
}

var (
	timeType = reflect.TypeOf(time.Time{})
	uuidType = reflect.TypeOf(uuid.UUID{})
)

func (r *Reflector) typeToProperty(t reflect.Type, tag reflect.StructTag) *Property {
	prop := &Property{}

	// Check pointer
	if t.Kind() == reflect.Pointer {
		prop.Nullable = true
		t = t.Elem()
	}

	// Known standard types
	if t == timeType {
		prop.Type = "string"
		prop.Format = "date-time"
		return prop
	}
	if t == uuidType {
		prop.Type = "string"
		prop.Format = "uuid"
		return prop
	}

	// Special check for money.Money struct
	if t.PkgPath() != "" && strings.HasSuffix(t.PkgPath(), "money") && t.Name() == "Money" {
		refName := "Money"
		if _, exists := r.Schemas[refName]; !exists {
			r.Schemas[refName] = &Schema{
				Type:     "object",
				Required: []string{"amount", "currency", "formatted"},
				Properties: map[string]*Property{
					"amount":    {Type: "integer", Format: "int64", Description: "Amount in smallest currency unit (cents/paise)"},
					"currency":  {Type: "string", Description: "ISO 4217 3-letter currency code"},
					"formatted": {Type: "string", Description: "Human readable formatted string"},
				},
			}
		}
		return &Property{Ref: "#/components/schemas/" + refName}
	}

	switch t.Kind() {
	case reflect.Bool:
		prop.Type = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		prop.Type = "integer"
	case reflect.Int64:
		prop.Type = "integer"
		prop.Format = "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		prop.Type = "integer"
	case reflect.Float32, reflect.Float64:
		prop.Type = "number"
	case reflect.String:
		prop.Type = "string"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte
			prop.Type = "string"
			prop.Format = "byte"
		} else {
			prop.Type = "array"
			prop.Items = r.typeToProperty(t.Elem(), "")
		}
	case reflect.Map:
		prop.Type = "object"
		prop.AdditionalProperties = r.typeToProperty(t.Elem(), "")
	case reflect.Struct:
		// Check if it's an inline anonymous struct or a named type
		name := t.Name()
		if name == "" {
			// Anonymous inline struct
			prop.Type = "object"
			prop.Properties = make(map[string]*Property)
			subSchema := &Schema{Type: "object", Properties: prop.Properties}
			r.fillStructProperties(t, subSchema)
			prop.Required = subSchema.Required
		} else {
			ref := r.RegisterStruct(name, t)
			prop.Ref = ref
		}
	case reflect.Interface:
		prop.Type = "object"
		prop.AdditionalProperties = true
	default:
		prop.Type = "string"
	}

	return prop
}

// GenerateSchema generates an OpenAPI Property for any value or reflect.Type.
func (r *Reflector) GenerateProperty(v any) *Property {
	if v == nil {
		return nil
	}
	var t reflect.Type
	if rt, ok := v.(reflect.Type); ok {
		t = rt
	} else {
		t = reflect.TypeOf(v)
	}
	return r.typeToProperty(t, "")
}

// RegisterNamedType registers a type with a custom schema name.
func (r *Reflector) RegisterNamedType(name string, v any) string {
	var t reflect.Type
	if rt, ok := v.(reflect.Type); ok {
		t = rt
	} else {
		t = reflect.TypeOf(v)
	}
	return r.RegisterStruct(name, t)
}

// SetRawSchema allows setting a known custom schema directly.
func (r *Reflector) SetRawSchema(name string, s *Schema) {
	r.Schemas[name] = s
}
