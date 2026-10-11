package trainerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type fakeTrainerLister struct {
	items  []ClientTrainer
	err    error
	called int
	userID uuid.UUID
}

func (f *fakeTrainerLister) ListClientTrainers(_ context.Context, clientUserID uuid.UUID) ([]ClientTrainer, error) {
	f.called++
	f.userID = clientUserID
	return f.items, f.err
}

func clientTrainersEcho(f *fakeTrainerLister) *echo.Echo {
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.trainers = f
	e := echo.New()
	h.Mount(e)
	return e
}

func getClientTrainersAt(e *echo.Echo, target, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if authz != "" {
		req.Header.Set(echo.HeaderAuthorization, authz)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestClientListTrainers_success(t *testing.T) {
	userID := uuid.New()
	first := ClientTrainer{
		TrainerID:          uuid.New(),
		TrainerDisplayName: "Anna",
		LinkedAt:           time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC),
	}
	second := ClientTrainer{
		TrainerID:          uuid.New(),
		TrainerDisplayName: "Boris",
		LinkedAt:           time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC),
	}
	f := &fakeTrainerLister{items: []ClientTrainer{first, second}}

	rec := getClientTrainersAt(clientTrainersEcho(f), "/client/trainers", bearerFor(t, userID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.called != 1 || f.userID != userID {
		t.Fatalf("service called %d times for %v, want once for %v", f.called, f.userID, userID)
	}
	var raw map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string][]map[string]any{"items": {
		{"trainer_id": first.TrainerID.String(), "trainer_display_name": "Anna", "linked_at": "2026-10-11T09:30:00Z"},
		{"trainer_id": second.TrainerID.String(), "trainer_display_name": "Boris", "linked_at": "2026-09-01T18:00:00Z"},
	}}
	if !reflect.DeepEqual(raw, want) {
		t.Fatalf("body = %v, want exactly %v", raw, want)
	}
}

// The never-nil guarantee lives in the store and is checked by the integration tests.
func TestClientListTrainers_emptySliceIsWrittenAsAnArray(t *testing.T) {
	f := &fakeTrainerLister{items: []ClientTrainer{}}

	rec := getClientTrainersAt(clientTrainersEcho(f), "/client/trainers", bearerFor(t, uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[]}` {
		t.Fatalf("body = %s, want exactly {\"items\":[]}", got)
	}
}

func TestClientListTrainers_requiresToken(t *testing.T) {
	f := &fakeTrainerLister{}
	e := clientTrainersEcho(f)
	for name, authz := range map[string]string{"missing": "", "malformed": "Bearer nope"} {
		rec := getClientTrainersAt(e, "/client/trainers", authz)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
	if f.called != 0 {
		t.Fatalf("service called %d times without a valid token", f.called)
	}
}

func TestClientListTrainers_storeFailureHidesTheCause(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.5:5432: connection refused")
	f := &fakeTrainerLister{err: fmt.Errorf("list client trainers: %w", cause)}

	rec := getClientTrainersAt(clientTrainersEcho(f), "/client/trainers", bearerFor(t, uuid.New()))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"message":"internal"}` {
		t.Fatalf("body = %s, want exactly {\"message\":\"internal\"}", got)
	}
}

func TestClientListTrainers_userComesOnlyFromTheToken(t *testing.T) {
	userID, other := uuid.New(), uuid.New()
	f := &fakeTrainerLister{items: []ClientTrainer{}}
	target := "/client/trainers?client_user_id=" + other.String() + "&user_id=" + other.String()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set(echo.HeaderAuthorization, bearerFor(t, userID))
	req.Header.Set("X-User-ID", other.String())

	rec := httptest.NewRecorder()
	clientTrainersEcho(f).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.userID != userID {
		t.Fatalf("service got %v, want the token user %v", f.userID, userID)
	}
}
