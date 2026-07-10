package trainerclient_test

import (
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	h.Mount(e)

	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	want := []string{
		"POST /trainer/invites",
		"GET /trainer/clients",
		"GET /trainer/clients/:client_user_id/program-assignment",
		"PUT /trainer/clients/:client_user_id/program-assignment",
	}
	for _, rt := range want {
		if !found[rt] {
			t.Fatalf("route %s missing; got %v", rt, found)
		}
	}
}
