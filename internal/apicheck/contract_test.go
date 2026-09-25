package apicheck

import (
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func TestGoRoutesMatchOpenAPI(t *testing.T) {
	root := repoRoot(t)
	goRoutes := GoRoutes(nil)

	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}

	if missing, extra := diffRoutes(goRoutes, openapi.Routes()); len(missing)+len(extra) > 0 {
		t.Error(formatRouteDiff("OpenAPI vs Go", missing, extra))
	}
}

func TestOpenAPISchemasMatchGoTypes(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}

	for _, bind := range schemaBindings() {
		t.Run(bind.name, func(t *testing.T) {
			openProps, err := openapi.SchemaPropertyNames(bind.name)
			if err != nil {
				t.Fatalf("openapi schema: %v", err)
			}
			goProps := jsonFieldNames(bind.typ)
			if err := comparePropertySets(bind.name, openProps, goProps); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenAPIEnumsMatchGoConstants(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}

	for _, bind := range enumBindings() {
		t.Run(bind.name, func(t *testing.T) {
			openVals, err := openapi.EnumValues(bind.name)
			if err != nil {
				t.Fatalf("openapi enum: %v", err)
			}
			if err := compareEnumSets(bind.name, openVals, bind.values); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Every JSON request body schema must be bound to a Go type, otherwise
// TestOpenAPISchemasMatchGoTypes never checks it.
func TestRequestBodySchemasHaveGoBinding(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}

	bound := make(map[string]struct{})
	for _, bind := range schemaBindings() {
		bound[bind.name] = struct{}{}
	}
	for _, route := range GoRoutes(nil) {
		schemaName, ok, err := openapi.RequestBodySchemaName(route.Method, route.Path)
		if err != nil {
			t.Fatalf("%s: %v", route, err)
		}
		if !ok {
			continue
		}
		if _, ok := bound[schemaName]; !ok {
			t.Errorf("%s: request schema %s has no entry in schemaBindings", route, schemaName)
		}
	}
}
