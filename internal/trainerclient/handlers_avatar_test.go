package trainerclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/trainerclient"
)

func TestHandlers_GetClientAvatar_invalidClientID(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/trainer/clients/not-a-uuid/avatar?exp=1&sig=x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("client_user_id")
	c.SetParamValues("not-a-uuid")

	err := h.GetClientAvatar(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestHandlers_GetClientAvatar_invalidSig(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	clientID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/trainer/clients/"+clientID.String()+"/avatar?exp=1&sig=bad", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("client_user_id")
	c.SetParamValues(clientID.String())

	err := h.GetClientAvatar(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestBuildAvatarURL_inListShape(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-chars-long"
	clientID := uuid.New()
	url := trainerclient.BuildAvatarURL(clientID, "photos/a.jpg", secret)
	if url == "" {
		t.Fatal("expected url")
	}
	if url[:len("/trainer/clients/")] != "/trainer/clients/" {
		t.Fatalf("url = %q", url)
	}
}
