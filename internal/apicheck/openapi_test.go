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
