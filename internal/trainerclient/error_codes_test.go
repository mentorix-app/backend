package trainerclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/workoutcompletion"
)

func decodeJSONObject(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	return got
}

func assertCodedError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body)
	}
	want := map[string]any{"code": code, "message": message}
	if got := decodeJSONObject(t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}

// renderHTTPErrorFrom returns err from GET /x through HTTPErrorFrom behind the
// error handler that app.Mount installs.
func renderHTTPErrorFrom(err error) *httptest.ResponseRecorder {
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	e.GET("/x", func(echo.Context) error { return HTTPErrorFrom(err) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec
}

func TestHTTPErrorFrom_codes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"client limit reached", ErrClientLimitReached, 409, "client_limit_reached", "client limit reached"},
		{"self invite", ErrSelfInvite, 422, "self_invite", "trainer cannot accept own invite"},
		{"invite not found", ErrInviteNotFound, 404, "invite_not_found", "invite not found"},
		{"invite expired", ErrInviteExpired, 410, "invite_expired", "invite expired"},
		{"invite consumed", ErrInviteConsumed, 409, "invite_consumed", "invite already consumed"},
		{"invite not configured", ErrInviteNotConfigured, 503, "invite_not_configured", "telegram invite not configured"},
		{"client not linked", program.ErrClientNotLinked, 403, "client_not_linked", "client is not linked to trainer"},
		{"trainer not linked keeps the default", ErrTrainerNotLinked, 403, "forbidden", "trainer is not linked to client"},
		{"admin cannot accept", ErrAdminCannotAccept, 403, "admin_cannot_accept", "admin cannot accept an invite"},
		{"active trainer not set keeps the default", ErrActiveTrainerNotSet, 409, "conflict", "active trainer not set"},
		{"telegram user not found keeps the default", ErrTelegramUserNotFound, 404, "not_found", "telegram user not found"},
		{"user not found", ErrUserNotFound, 404, "user_not_found", "user not found"},
		{"client blocked", program.ErrClientBlocked, 422, "client_blocked", "client is blocked"},
		{"program not published", program.ErrProgramNotPublished, 422, "program_not_published", "program is not published"},
		{"client not found", program.ErrClientNotFound, 404, "client_not_found", "client not found"},
		{"program forbidden keeps the default", program.ErrForbidden, 403, "forbidden", "forbidden"},
		{"program not found keeps the default", program.ErrNotFound, 404, "not_found", "program not found"},
		{"day not found", ErrDayNotFound, 404, "day_not_found", "day not found"},
		{"program changed", ErrProgramChanged, 409, "program_changed", "program changed"},
		{"workout already completed", workoutcompletion.ErrAlreadyCompleted, 409, "workout_already_completed", "workout already completed"},
		{"workout validation keeps the default", fmt.Errorf("%w: result_text required", workoutcompletion.ErrValidation), 400, "bad_request", "validation failed: result_text required"},
		{"unexpected keeps the default", errors.New("boom"), 500, "internal_error", "internal"},
		{"wrapped sentinel keeps its code", fmt.Errorf("accept invite: %w", ErrInviteExpired), 410, "invite_expired", "accept invite: invite expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCodedError(t, renderHTTPErrorFrom(tt.err), tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestHTTPErrorFrom_quotaBodyHasAllSevenFields(t *testing.T) {
	qe := &subscription.QuotaError{Resource: subscription.ResourceClients, Plan: subscription.PlanFree, Limit: 3, Usage: 3}

	rec := renderHTTPErrorFrom(fmt.Errorf("create invite: %w", qe))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body)
	}
	want := map[string]any{
		"error": "quota_exceeded", "resource": "clients", "plan": "free", "limit": 3.0, "usage": 3.0,
		"code": "quota_exceeded", "message": "quota exceeded",
	}
	if got := decodeJSONObject(t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}

func codedInviteEcho(f *fakeInviteAcceptor) *echo.Echo {
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.invites = f
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	h.Mount(e)
	return e
}

func TestClientAcceptInvite_codes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"invite not found", ErrInviteNotFound, 404, "invite_not_found", "invite not found"},
		{"app user not found", ErrUserNotFound, 404, "user_not_found", "user not found"},
		{"invite consumed", ErrInviteConsumed, 409, "invite_consumed", "invite already consumed"},
		{"client limit", ErrClientLimitReached, 409, "client_limit_reached", "client limit reached"},
		{"invite expired", ErrInviteExpired, 410, "invite_expired", "invite expired"},
		{"admin caller", ErrAdminCannotAccept, 403, "admin_cannot_accept", "admin cannot accept an invite"},
		{"own invite", ErrSelfInvite, 422, "self_invite", "trainer cannot accept own invite"},
		{"blocked client", program.ErrClientBlocked, 422, "client_blocked", "client is blocked"},
		{"unexpected", errors.New("boom"), 500, "internal_error", "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postAccept(codedInviteEcho(&fakeInviteAcceptor{err: tt.err}), bearerFor(t, uuid.New()), `{"token":"abc"}`)

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestClientAcceptInvite_tokenAndBodyErrorsCarryCodes(t *testing.T) {
	e := codedInviteEcho(&fakeInviteAcceptor{})

	assertCodedError(t, postAccept(e, "", `{"token":"abc"}`), 401, "missing_authorization", httpx.MsgMissingAuth)
	assertCodedError(t, postAccept(e, "Bearer nope", `{"token":"abc"}`), 401, "invalid_token", httpx.MsgInvalidToken)
	assertCodedError(t, postAccept(e, bearerFor(t, uuid.New()), `not-json`), 400, "bad_request", httpx.MsgInvalidJSON)
}

func TestClientAcceptInvite_twoConflictsAreToldApart(t *testing.T) {
	codeOf := func(err error) any {
		rec := postAccept(codedInviteEcho(&fakeInviteAcceptor{err: err}), bearerFor(t, uuid.New()), `{"token":"abc"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%v: status = %d, want 409", err, rec.Code)
		}
		return decodeJSONObject(t, rec)["code"]
	}

	consumed, full := codeOf(ErrInviteConsumed), codeOf(ErrClientLimitReached)

	if consumed == full {
		t.Fatalf("both 409 answers carry the code %v", consumed)
	}
}

func TestClientListTrainers_errorsCarryCodes(t *testing.T) {
	f := &fakeTrainerLister{err: fmt.Errorf("list client trainers: %w", errors.New("dial tcp 10.0.0.5:5432: connection refused"))}
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.trainers = f
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	h.Mount(e)

	assertCodedError(t, getClientTrainersAt(e, "/client/trainers", ""), 401, "missing_authorization", httpx.MsgMissingAuth)
	assertCodedError(t, getClientTrainersAt(e, "/client/trainers", "Bearer nope"), 401, "invalid_token", httpx.MsgInvalidToken)
	assertCodedError(t, getClientTrainersAt(e, "/client/trainers", bearerFor(t, uuid.New())), 500, "internal_error", httpx.MsgInternal)
}
