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

func TestGoRoutesMatchOpenAPIAndPostman(t *testing.T) {
	root := repoRoot(t)
	goRoutes := GoRoutes(nil)

	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}

	postman, err := LoadPostman(filepath.Join(root, "postman/mentorix-backend.postman_collection.json"))
	if err != nil {
		t.Fatalf("load postman: %v", err)
	}

	if missing, extra := diffRoutes(goRoutes, openapi.Routes()); len(missing)+len(extra) > 0 {
		t.Error(formatRouteDiff("OpenAPI vs Go", missing, extra))
	}
	if missing, extra := diffRoutes(goRoutes, postman.Routes()); len(missing)+len(extra) > 0 {
		t.Error(formatRouteDiff("Postman vs Go", missing, extra))
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

func TestPostmanRequestBodiesMatchOpenAPI(t *testing.T) {
	root := repoRoot(t)
	openapi, err := LoadOpenAPI(filepath.Join(root, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	postman, err := LoadPostman(filepath.Join(root, "postman/mentorix-backend.postman_collection.json"))
	if err != nil {
		t.Fatalf("load postman: %v", err)
	}

	for _, req := range postman.Requests() {
		if req.BodyRaw == "" {
			continue
		}
		schemaName, ok, err := openapi.RequestBodySchemaName(req.Route.Method, req.Route.Path)
		if err != nil {
			t.Fatalf("%s (%s): %v", req.Name, req.Route, err)
		}
		if !ok {
			t.Fatalf("%s (%s): postman has JSON body but OpenAPI has no request schema", req.Name, req.Route)
		}
		bodyKeys, err := jsonObjectKeys(req.BodyRaw)
		if err != nil {
			t.Fatalf("%s: parse body: %v", req.Name, err)
		}
		schemaKeys, err := openapi.SchemaPropertyNames(schemaName)
		if err != nil {
			t.Fatalf("%s: schema %s: %v", req.Name, schemaName, err)
		}
		for k := range bodyKeys {
			if _, ok := schemaKeys[k]; !ok {
				t.Errorf("%s: body field %q not in OpenAPI schema %s", req.Name, k, schemaName)
			}
		}
	}
}
