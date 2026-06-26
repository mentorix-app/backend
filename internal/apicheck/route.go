package apicheck

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// Route is a normalized HTTP method and Echo-style path (/resource/:id).
type Route struct {
	Method string
	Path   string
}

func (r Route) String() string {
	return r.Method + " " + r.Path
}

var httpMethodsSet = map[string]struct{}{
	http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodHead: {},
}

// GoRoutes returns routes registered on a test Echo instance.
func GoRoutes(pool *pgxpool.Pool) []Route {
	e := echo.New()
	MountRoutes(e, pool)

	seen := make(map[Route]struct{})
	for _, r := range e.Routes() {
		if _, ok := httpMethodsSet[r.Method]; !ok {
			continue
		}
		seen[Route{Method: r.Method, Path: r.Path}] = struct{}{}
	}

	out := make([]Route, 0, len(seen))
	for rt := range seen {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Method < out[j].Method
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// OpenAPIPathToEcho converts /items/{id} to /items/:id.
func OpenAPIPathToEcho(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			j := strings.IndexByte(path[i:], '}')
			if j < 0 {
				return path
			}
			b.WriteByte(':')
			b.WriteString(path[i+1 : i+j])
			i += j
			continue
		}
		b.WriteByte(path[i])
	}
	return b.String()
}

// EchoPathToOpenAPI converts /items/:id to /items/{id}.
func EchoPathToOpenAPI(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == ':' {
			j := i + 1
			for j < len(path) && path[j] != '/' {
				j++
			}
			b.WriteByte('{')
			b.WriteString(path[i+1 : j])
			b.WriteByte('}')
			i = j - 1
			continue
		}
		b.WriteByte(path[i])
	}
	return b.String()
}

func routeSet(routes []Route) map[Route]struct{} {
	set := make(map[Route]struct{}, len(routes))
	for _, r := range routes {
		set[r] = struct{}{}
	}
	return set
}

func diffRoutes(want, got []Route) (missing, extra []Route) {
	wantSet := routeSet(want)
	gotSet := routeSet(got)
	for r := range wantSet {
		if _, ok := gotSet[r]; !ok {
			missing = append(missing, r)
		}
	}
	for r := range gotSet {
		if _, ok := wantSet[r]; !ok {
			extra = append(extra, r)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].String() < missing[j].String() })
	sort.Slice(extra, func(i, j int) bool { return extra[i].String() < extra[j].String() })
	return missing, extra
}

func formatRouteDiff(label string, missing, extra []Route) string {
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n", label)
	for _, r := range missing {
		fmt.Fprintf(&b, "  missing %s\n", r)
	}
	for _, r := range extra {
		fmt.Fprintf(&b, "  extra %s\n", r)
	}
	return b.String()
}
