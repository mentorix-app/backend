package apicheck

import "testing"

func TestRoute_String(t *testing.T) {
	r := Route{Method: "GET", Path: "/health"}
	if got := r.String(); got != "GET /health" {
		t.Errorf("String() = %q", got)
	}
}

func TestOpenAPIPathToEcho_malformed(t *testing.T) {
	if got := OpenAPIPathToEcho("/items/{id"); got != "/items/{id" {
		t.Errorf("OpenAPIPathToEcho() = %q", got)
	}
}

func TestDiffRoutes_andFormatRouteDiff(t *testing.T) {
	want := []Route{{Method: "GET", Path: "/a"}}
	got := []Route{{Method: "POST", Path: "/b"}}
	missing, extra := diffRoutes(want, got)
	if len(missing) != 1 || len(extra) != 1 {
		t.Fatalf("missing=%v extra=%v", missing, extra)
	}
	diff := formatRouteDiff("test", missing, extra)
	if diff == "" || diff == formatRouteDiff("test", nil, nil) {
		t.Fatalf("formatRouteDiff = %q", diff)
	}
	if formatRouteDiff("ok", nil, nil) != "" {
		t.Fatal("expected empty diff")
	}
}
