package apicheck

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type postmanCollection struct {
	Item []postmanItem `json:"item"`
}

type postmanItem struct {
	Name    string        `json:"name"`
	Item    []postmanItem `json:"item"`
	Request *postmanReq   `json:"request"`
}

type postmanReq struct {
	Method string       `json:"method"`
	URL    string       `json:"url"`
	Body   *postmanBody `json:"body"`
}

type postmanBody struct {
	Mode string `json:"mode"`
	Raw  string `json:"raw"`
}

var postmanVarToEcho = []struct{ old, new string }{
	// Analytics routes use program_id as the param name; must precede the generic program_id rule.
	{"/trainer/programs/{{program_id}}/analytics", "/trainer/programs/:program_id/analytics"},
	{"{{program_week_day_block_exercise_id}}", ":item_id"},
	{"{{program_week_day_block_id}}", ":block_id"},
	{"{{program_week_day_id}}", ":day_id"},
	{"{{program_week_id}}", ":week_id"},
	{"{{program_version_id}}", ":version_id"},
	{"{{client_user_id}}", ":client_user_id"},
	{"{{completion_id}}", ":completion_id"},
	{"{{program_id}}", ":id"},
	{"{{exercise_id}}", ":id"},
	{"{{plan_trainer_user_id}}", ":user_id"},
}

// LoadPostman reads a Postman collection v2.1 file.
func LoadPostman(path string) (*postmanCollection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read postman: %w", err)
	}
	var c postmanCollection
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse postman: %w", err)
	}
	return &c, nil
}

func normalizePostmanURL(url string) string {
	path := strings.TrimPrefix(url, "{{base_url}}")
	for _, rep := range postmanVarToEcho {
		path = strings.ReplaceAll(path, rep.old, rep.new)
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return path
}

type PostmanRequest struct {
	Name    string
	Route   Route
	BodyRaw string
}

// Requests walks the collection and returns normalized requests.
func (c *postmanCollection) Requests() []PostmanRequest {
	var out []PostmanRequest
	var walk func(items []postmanItem)
	walk = func(items []postmanItem) {
		for _, it := range items {
			if len(it.Item) > 0 {
				walk(it.Item)
				continue
			}
			if it.Request == nil {
				continue
			}
			path := normalizePostmanURL(it.Request.URL)
			req := PostmanRequest{
				Name:  it.Name,
				Route: Route{Method: it.Request.Method, Path: path},
			}
			if it.Request.Body != nil && it.Request.Body.Mode == "raw" {
				req.BodyRaw = strings.TrimSpace(it.Request.Body.Raw)
			}
			out = append(out, req)
		}
	}
	walk(c.Item)
	return out
}

// Routes returns the set of routes covered by at least one Postman request.
func (c *postmanCollection) Routes() []Route {
	reqs := c.Requests()
	seen := make(map[Route]struct{}, len(reqs))
	for _, r := range reqs {
		seen[r.Route] = struct{}{}
	}
	out := make([]Route, 0, len(seen))
	for rt := range seen {
		out = append(out, rt)
	}
	return out
}

var postmanBodyVar = regexp.MustCompile(`\{\{[^}]+\}\}`)

func normalizePostmanBodyRaw(raw string) string {
	return postmanBodyVar.ReplaceAllString(raw, "[]")
}

func jsonObjectKeys(raw string) (map[string]struct{}, error) {
	raw = normalizePostmanBodyRaw(raw)
	if raw == "" {
		return map[string]struct{}{}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	keys := make(map[string]struct{}, len(obj))
	for k := range obj {
		keys[k] = struct{}{}
	}
	return keys, nil
}
