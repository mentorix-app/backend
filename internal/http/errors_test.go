package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func errorEcho() *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.Logger.SetOutput(io.Discard)
	return e
}

func serve(e *echo.Echo, method, target string, body io.Reader) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(method, target, body))
	return rec
}

func failWith(err error) echo.HandlerFunc {
	return func(echo.Context) error { return err }
}

// assertErrorBody checks the status and that the body has exactly the two keys
// code and message.
func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, echo.MIMEApplicationJSON) {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	want := map[string]any{"code": code, "message": message}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}

func TestErrorHandler_defaultCodeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "bad_request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.StatusConflict, "conflict"},
		{http.StatusGone, "gone"},
		{http.StatusRequestEntityTooLarge, "payload_too_large"},
		{http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{http.StatusUnprocessableEntity, "unprocessable_entity"},
		{http.StatusTooManyRequests, "too_many_requests"},
		{http.StatusInternalServerError, "internal_error"},
		{http.StatusServiceUnavailable, "service_unavailable"},
		{http.StatusTeapot, "bad_request"},
		{http.StatusUnavailableForLegalReasons, "bad_request"},
		{http.StatusBadGateway, "internal_error"},
		{http.StatusGatewayTimeout, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			e := errorEcho()
			e.GET("/x", failWith(echo.NewHTTPError(tt.status, "some text")))

			assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), tt.status, tt.code, "some text")
		})
	}
}

func TestNewError_setsTheExplicitCode(t *testing.T) {
	e := errorEcho()
	e.GET("/x", failWith(NewError(http.StatusConflict, "sample_conflict", "sample conflict")))

	assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusConflict, "sample_conflict", "sample conflict")
}

func TestNewError_emptyCodeFallsBackToTheStatusCode(t *testing.T) {
	e := errorEcho()
	e.GET("/x", failWith(NewError(http.StatusConflict, "", "text")))

	assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusConflict, "conflict", "text")
}

func TestNewError_keepsTheMessageAsAString(t *testing.T) {
	he := NewError(http.StatusUnauthorized, "invalid_token", "invalid token")

	if he.Code != http.StatusUnauthorized {
		t.Errorf("Code = %d", he.Code)
	}
	if msg, ok := he.Message.(string); !ok || msg != "invalid token" {
		t.Errorf("Message = %#v, want the string %q", he.Message, "invalid token")
	}
}

func TestNewErrorCause_keepsTheCauseReachable(t *testing.T) {
	cause := errors.New("account holds another telegram")
	he := NewErrorCause(http.StatusConflict, "telegram_already_linked", "account already has another telegram", cause)

	if !errors.Is(he, cause) {
		t.Fatal("errors.Is(he, cause) = false")
	}
	if msg, ok := he.Message.(string); !ok || msg != "account already has another telegram" {
		t.Errorf("Message = %#v", he.Message)
	}
}

func TestErrorHandler_neverPutsTheCauseInTheBody(t *testing.T) {
	const secret = "pq: password authentication failed for user app"
	cause := errors.New(secret)
	errs := map[string]error{
		"coded with cause":    NewErrorCause(http.StatusServiceUnavailable, "telegram_link_unavailable", "telegram linking is unavailable", cause),
		"plain with internal": echo.NewHTTPError(http.StatusBadRequest, "bad input").SetInternal(cause),
		"not an http error":   cause,
	}
	for name, err := range errs {
		for _, debug := range []bool{false, true} {
			t.Run(name+" debug="+strconv.FormatBool(debug), func(t *testing.T) {
				e := errorEcho()
				e.Debug = debug
				e.GET("/x", failWith(err))

				rec := serve(e, http.MethodGet, "/x", nil)

				if strings.Contains(rec.Body.String(), "password") {
					t.Fatalf("body leaks the cause: %s", rec.Body)
				}
				var got map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 2 {
					t.Fatalf("body = %s, want exactly code and message", rec.Body)
				}
			})
		}
	}
}

func TestErrorHandler_codedErrorWithCauseKeepsItsCode(t *testing.T) {
	e := errorEcho()
	e.Debug = true
	e.GET("/x", failWith(NewErrorCause(http.StatusServiceUnavailable, "telegram_link_unavailable",
		"telegram linking is unavailable", errors.New("redis down"))))

	assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusServiceUnavailable,
		"telegram_link_unavailable", "telegram linking is unavailable")
}

func TestErrorHandler_structMessageIsRenderedUntouched(t *testing.T) {
	type quota struct {
		Error string `json:"error"`
		Limit int    `json:"limit"`
	}
	e := errorEcho()
	e.GET("/x", failWith(echo.NewHTTPError(http.StatusConflict, quota{Error: "quota_exceeded", Limit: 3})))

	rec := serve(e, http.MethodGet, "/x", nil)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"error":"quota_exceeded","limit":3}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestErrorHandler_errorMessageUsesItsText(t *testing.T) {
	e := errorEcho()
	e.GET("/x", failWith(echo.NewHTTPError(http.StatusBadRequest, errors.New("name is too long"))))

	assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusBadRequest, "bad_request", "name is too long")
}

// selfFormatting is an error that also knows its own JSON form. Echo prefers the
// JSON form, so the handler does too.
type selfFormatting struct{}

func (selfFormatting) Error() string { return "text form" }

func (selfFormatting) MarshalJSON() ([]byte, error) { return []byte(`{"form":"json"}`), nil }

func TestErrorHandler_jsonMarshalerMessageFormatsItself(t *testing.T) {
	e := errorEcho()
	e.GET("/x", failWith(echo.NewHTTPError(http.StatusBadRequest, selfFormatting{})))

	rec := serve(e, http.MethodGet, "/x", nil)

	if got, want := strings.TrimSpace(rec.Body.String()), `{"form":"json"}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestErrorHandler_plainErrorBecomesInternalError(t *testing.T) {
	e := errorEcho()
	e.GET("/x", failWith(errors.New("boom")))

	assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusInternalServerError, "internal_error", "Internal Server Error")
}

func TestErrorHandler_routerErrors(t *testing.T) {
	e := errorEcho()
	e.GET("/only-get", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	t.Run("unknown path", func(t *testing.T) {
		assertErrorBody(t, serve(e, http.MethodGet, "/nope", nil), http.StatusNotFound, "not_found", "Not Found")
	})
	t.Run("wrong method", func(t *testing.T) {
		rec := serve(e, http.MethodPost, "/only-get", nil)
		assertErrorBody(t, rec, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed")
		if allow := rec.Header().Get(echo.HeaderAllow); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want GET listed", allow)
		}
	})
}

func TestErrorHandler_bodyLimit(t *testing.T) {
	e := errorEcho()
	e.Use(middleware.BodyLimit("4B"))
	e.POST("/upload", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	rec := serve(e, http.MethodPost, "/upload", strings.NewReader("more than four bytes"))

	assertErrorBody(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large", "Request Entity Too Large")
}

func TestErrorHandler_panicBehindRecover(t *testing.T) {
	e := errorEcho()
	e.Use(middleware.Recover())
	e.GET("/x", func(echo.Context) error { panic("kaboom") })

	rec := serve(e, http.MethodGet, "/x", nil)

	assertErrorBody(t, rec, http.StatusInternalServerError, "internal_error", "Internal Server Error")
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Fatalf("body leaks the panic value: %s", rec.Body)
	}
}

func TestErrorHandler_headAnswersWithoutBody(t *testing.T) {
	e := errorEcho()
	e.HEAD("/x", failWith(NewError(http.StatusConflict, "sample_conflict", "sample conflict")))

	t.Run("handler error", func(t *testing.T) {
		rec := serve(e, http.MethodHead, "/x", nil)
		if rec.Code != http.StatusConflict || rec.Body.Len() != 0 {
			t.Fatalf("status = %d, body = %q, want 409 and no body", rec.Code, rec.Body)
		}
	})
	t.Run("router error", func(t *testing.T) {
		rec := serve(e, http.MethodHead, "/nope", nil)
		if rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
			t.Fatalf("status = %d, body = %q, want 404 and no body", rec.Code, rec.Body)
		}
	})
}

func TestErrorHandler_leavesACommittedResponseAlone(t *testing.T) {
	e := errorEcho()
	e.GET("/x", func(c echo.Context) error {
		if err := c.String(http.StatusOK, "partial"); err != nil {
			return err
		}
		return NewError(http.StatusConflict, "sample_conflict", "late failure")
	})

	rec := serve(e, http.MethodGet, "/x", nil)

	if rec.Code != http.StatusOK || rec.Body.String() != "partial" {
		t.Fatalf("status = %d, body = %q, want the response that was already sent", rec.Code, rec.Body)
	}
}

func TestErrorHandler_rendersAnInternalHTTPError(t *testing.T) {
	tests := []struct {
		name    string
		inner   *echo.HTTPError
		status  int
		code    string
		message string
	}{
		{"inner without a code", echo.NewHTTPError(http.StatusNotFound, "inner text"), http.StatusNotFound, "not_found", "inner text"},
		{"inner with a code", NewError(http.StatusGone, "invite_expired", "invite expired"), http.StatusGone, "invite_expired", "invite expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := errorEcho()
			e.GET("/x", failWith(echo.NewHTTPError(http.StatusInternalServerError, "outer text").SetInternal(tt.inner)))

			assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), tt.status, tt.code, tt.message)
		})
	}
}

// A marker counts only when NewError placed it directly. One that sits deeper in
// the chain of another error belongs to that other error, not to this response.
func TestErrorHandler_ignoresAMarkerBuriedInTheChain(t *testing.T) {
	buried := NewError(http.StatusGone, "invite_expired", "invite expired")
	tests := map[string]error{
		"wrapped with %w": fmt.Errorf("ctx: %w", buried),
		"joined":          errors.Join(errors.New("first"), buried),
	}
	for name, internal := range tests {
		t.Run(name, func(t *testing.T) {
			e := errorEcho()
			e.GET("/x", failWith(echo.NewHTTPError(http.StatusInternalServerError, "outer text").SetInternal(internal)))

			assertErrorBody(t, serve(e, http.MethodGet, "/x", nil), http.StatusInternalServerError, "internal_error", "outer text")
		})
	}
}

// brokenWriter is a response writer whose body can never be written.
type brokenWriter struct{ header http.Header }

func (w *brokenWriter) Header() http.Header { return w.header }

func (w *brokenWriter) WriteHeader(int) {}

func (w *brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestErrorHandler_logsAFailedWrite(t *testing.T) {
	var logs bytes.Buffer
	e := echo.New()
	e.Logger.SetOutput(&logs)
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), &brokenWriter{header: http.Header{}})

	ErrorHandler(NewError(http.StatusConflict, "sample_conflict", "sample conflict"), c)

	if !strings.Contains(logs.String(), "broken pipe") {
		t.Fatalf("logger output = %q, want the write error", logs.String())
	}
}

func TestRequestLog_codedErrorShowsCodeAndCauseButTheBodyOnlyTheCode(t *testing.T) {
	var buf bytes.Buffer
	e := errorEcho()
	e.Use(RequestLog(slog.New(slog.NewTextHandler(&buf, nil))))
	e.GET("/x", failWith(NewErrorCause(http.StatusConflict, "telegram_already_linked",
		"account already has another telegram", errors.New("existing identity 42"))))

	rec := serve(e, http.MethodGet, "/x", nil)

	assertErrorBody(t, rec, http.StatusConflict, "telegram_already_linked", "account already has another telegram")
	log := buf.String()
	for _, want := range []string{"telegram_already_linked", "existing identity 42", "level=WARN", "status=409"} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q does not contain %q", log, want)
		}
	}
	if strings.Contains(rec.Body.String(), "existing identity 42") {
		t.Errorf("body leaks the cause: %s", rec.Body)
	}
}

func TestRequestLog_codedErrorWithoutCauseShowsTheCode(t *testing.T) {
	var buf bytes.Buffer
	e := errorEcho()
	e.Use(RequestLog(slog.New(slog.NewTextHandler(&buf, nil))))
	e.GET("/x", failWith(NewError(http.StatusConflict, "sample_conflict", "sample conflict")))

	serve(e, http.MethodGet, "/x", nil)

	if log := buf.String(); !strings.Contains(log, "sample_conflict") || !strings.Contains(log, "sample conflict") {
		t.Fatalf("log = %q, want the code and the message", log)
	}
}
