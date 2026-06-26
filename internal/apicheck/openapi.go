package apicheck

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type openAPIDoc struct {
	Paths      map[string]map[string]any `yaml:"paths"`
	Components struct {
		Schemas map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

var httpMethods = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "patch": {}, "delete": {},
}

// LoadOpenAPI reads api/openapi.yaml.
func LoadOpenAPI(path string) (*openAPIDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read openapi: %w", err)
	}
	var doc openAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}
	if doc.Paths == nil {
		return nil, fmt.Errorf("openapi: paths missing")
	}
	if doc.Components.Schemas == nil {
		return nil, fmt.Errorf("openapi: components.schemas missing")
	}
	return &doc, nil
}

// Routes returns normalized routes declared in OpenAPI paths.
func (d *openAPIDoc) Routes() []Route {
	var routes []Route
	for path, item := range d.Paths {
		echoPath := OpenAPIPathToEcho(path)
		for method := range item {
			m := strings.ToUpper(method)
			if _, ok := httpMethods[strings.ToLower(method)]; !ok {
				continue
			}
			routes = append(routes, Route{Method: m, Path: echoPath})
		}
	}
	return routes
}

func (d *openAPIDoc) resolveRef(ref string) (map[string]any, error) {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported ref %q", ref)
	}
	name := strings.TrimPrefix(ref, prefix)
	raw, ok := d.Components.Schemas[name]
	if !ok {
		return nil, fmt.Errorf("schema %q not found", name)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema %q is not an object", name)
	}
	return m, nil
}

func (d *openAPIDoc) schemaPropertyNames(schema any) (map[string]struct{}, error) {
	switch node := schema.(type) {
	case nil:
		return map[string]struct{}{}, nil
	case map[string]any:
		if ref, ok := node["$ref"].(string); ok {
			resolved, err := d.resolveRef(ref)
			if err != nil {
				return nil, err
			}
			return d.schemaPropertyNames(resolved)
		}
		if allOf, ok := node["allOf"].([]any); ok {
			props := make(map[string]struct{})
			for _, part := range allOf {
				partProps, err := d.schemaPropertyNames(part)
				if err != nil {
					return nil, err
				}
				for k := range partProps {
					props[k] = struct{}{}
				}
			}
			if extra, ok := node["properties"].(map[string]any); ok {
				for k := range extra {
					props[k] = struct{}{}
				}
			}
			return props, nil
		}
		props := make(map[string]struct{})
		rawProps, ok := node["properties"].(map[string]any)
		if !ok {
			return props, nil
		}
		for k := range rawProps {
			props[k] = struct{}{}
		}
		return props, nil
	default:
		return nil, fmt.Errorf("unexpected schema node type %T", schema)
	}
}

// SchemaPropertyNames returns JSON property keys for a components/schemas entry.
func (d *openAPIDoc) SchemaPropertyNames(name string) (map[string]struct{}, error) {
	schema, err := d.resolveRef("#/components/schemas/" + name)
	if err != nil {
		return nil, err
	}
	return d.schemaPropertyNames(schema)
}

// EnumValues returns enum values for string enum schemas.
func (d *openAPIDoc) EnumValues(name string) ([]string, error) {
	schema, err := d.resolveRef("#/components/schemas/" + name)
	if err != nil {
		return nil, err
	}
	raw, ok := schema["enum"].([]any)
	if !ok {
		return nil, fmt.Errorf("schema %q has no enum", name)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("schema %q enum value is not a string", name)
		}
		out = append(out, s)
	}
	return out, nil
}

// RequestBodySchemaName returns the components schema name for JSON request bodies.
func (d *openAPIDoc) RequestBodySchemaName(method, echoPath string) (string, bool, error) {
	openPath := EchoPathToOpenAPI(echoPath)
	item, ok := d.Paths[openPath]
	if !ok {
		return "", false, nil
	}
	op, ok := item[strings.ToLower(method)].(map[string]any)
	if !ok {
		return "", false, nil
	}
	body, ok := op["requestBody"].(map[string]any)
	if !ok {
		return "", false, nil
	}
	content, ok := body["content"].(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("%s %s: requestBody.content missing", method, echoPath)
	}
	jsonCT, ok := content["application/json"].(map[string]any)
	if !ok {
		return "", false, nil
	}
	schema, ok := jsonCT["schema"].(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("%s %s: request schema missing", method, echoPath)
	}
	ref, ok := schema["$ref"].(string)
	if !ok {
		return "", false, fmt.Errorf("%s %s: request schema is not a $ref", method, echoPath)
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false, fmt.Errorf("%s %s: unsupported request schema ref %q", method, echoPath, ref)
	}
	return strings.TrimPrefix(ref, prefix), true, nil
}
