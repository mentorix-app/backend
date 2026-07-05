package apicheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOpenAPI_invalidFile(t *testing.T) {
	_, err := LoadOpenAPI(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadOpenAPI_invalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOpenAPI(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadPostman_invalidFile(t *testing.T) {
	_, err := LoadPostman(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		return
	}
	t.Fatal("expected error")
}

func TestSchemaPropertyNames_unknownSchema(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	_, err = openapi.SchemaPropertyNames("NotARealSchema")
	if err == nil {
		t.Fatal("expected error for unknown schema")
	}
}

func TestRequestBodySchemaName_knownRoute(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	name, ok, err := openapi.RequestBodySchemaName("POST", "/auth/login")
	if err != nil {
		t.Fatalf("RequestBodySchemaName() error = %v", err)
	}
	if !ok || name == "" {
		t.Fatalf("name = %q, ok = %v", name, ok)
	}
}

func TestRequestBodySchemaName_missingPath(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	name, ok, err := openapi.RequestBodySchemaName("POST", "/v1/no-such-route")
	if err != nil {
		t.Fatalf("RequestBodySchemaName() error = %v", err)
	}
	if ok || name != "" {
		t.Fatalf("name = %q, ok = %v, want false", name, ok)
	}
}

func TestRequestBodySchemaName_invalidRequestBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	doc := `openapi: 3.0.3
info:
  title: t
  version: "1"
paths:
  /v1/bad:
    post:
      requestBody: {}
components:
  schemas: {}
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	openapi, err := LoadOpenAPI(path)
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	_, _, err = openapi.RequestBodySchemaName("POST", "/v1/bad")
	if err == nil {
		t.Fatal("expected error for missing requestBody.content")
	}
}

func TestRequestBodySchemaName_schemaNotRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	doc := `openapi: 3.0.3
info:
  title: t
  version: "1"
paths:
  /v1/bad:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
components:
  schemas: {}
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	openapi, err := LoadOpenAPI(path)
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	_, _, err = openapi.RequestBodySchemaName("POST", "/v1/bad")
	if err == nil {
		t.Fatal("expected error for non-$ref schema")
	}
}

func TestRequestBodySchemaName_unsupportedRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	doc := `openapi: 3.0.3
info:
  title: t
  version: "1"
paths:
  /v1/bad:
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: "http://example.com/Foo"
components:
  schemas: {}
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	openapi, err := LoadOpenAPI(path)
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	_, _, err = openapi.RequestBodySchemaName("POST", "/v1/bad")
	if err == nil {
		t.Fatal("expected error for unsupported schema ref")
	}
}
