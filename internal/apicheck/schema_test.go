package apicheck

import (
	"reflect"
	"strings"
	"testing"
)

func TestComparePropertySets_match(t *testing.T) {
	openapi := map[string]struct{}{"id": {}, "name": {}}
	goFields := map[string]struct{}{"id": {}, "name": {}}
	if err := comparePropertySets("Example", openapi, goFields); err != nil {
		t.Fatalf("comparePropertySets() error = %v", err)
	}
}

func TestComparePropertySets_mismatch(t *testing.T) {
	openapi := map[string]struct{}{"id": {}}
	goFields := map[string]struct{}{"name": {}}
	err := comparePropertySets("Example", openapi, goFields)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "Example") {
		t.Fatalf("error = %v", err)
	}
}

type sampleJSONStruct struct {
	ID     string `json:"id"`
	Skip   int    `json:"-"`
	hidden string
	Name   string `json:"name,omitempty"`
}

func TestCollectJSONFields(t *testing.T) {
	_ = sampleJSONStruct{hidden: "unexported"} // fixture: must not appear in json tags
	fields := jsonFieldNames(reflect.TypeOf(sampleJSONStruct{}))
	if _, ok := fields["id"]; !ok {
		t.Fatal("missing id field")
	}
	if _, ok := fields["name"]; !ok {
		t.Fatal("missing name field")
	}
	if _, ok := fields["skip"]; ok {
		t.Fatal("unexpected skip field")
	}
}
