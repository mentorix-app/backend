package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	httpx "mentorix-backend/internal/http"
)

// codedEcho mounts the real routes behind the error handler that app.Mount installs.
func codedEcho(h *Handlers, middlewares ...echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	e.Use(middlewares...)
	h.Mount(e)
	return e
}

func assertCodedError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body)
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

func bearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	token, _, err := signAccessToken(userID, []byte(testJWTSecret), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + token
}

type codedRequest struct {
	method string
	path   string
	body   string
	// authorization is sent as is; "user" stands for a valid access token.
	authorization string
	cookie        string
}

func (r codedRequest) serve(t *testing.T, e *echo.Echo) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
	if r.body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	switch r.authorization {
	case "":
	case "user":
		req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
	default:
		req.Header.Set(echo.HeaderAuthorization, r.authorization)
	}
	if r.cookie != "" {
		req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: r.cookie})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

const (
	registerBody = `{"email":"user@example.com","password":"password123"}`
	loginBody    = `{"email":"user@example.com","password":"password123"}`
	idTokenBody  = `{"id_token":"x"}`
	linkBody     = `{"code":"ABCD2345"}`
)

func TestErrorCodes_handlerAnswers(t *testing.T) {
	tests := []struct {
		name        string
		svc         *fakeAuthService
		req         codedRequest
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"register email taken", &fakeAuthService{registerErr: ErrEmailTaken},
			codedRequest{method: "POST", path: "/auth/register", body: registerBody},
			409, "email_taken", "email already registered"},
		{"register invalid json keeps the default", &fakeAuthService{},
			codedRequest{method: "POST", path: "/auth/register", body: "not-json"},
			400, "bad_request", httpx.MsgInvalidJSON},
		{"register failure keeps the default", &fakeAuthService{registerErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/register", body: registerBody},
			500, "internal_error", "registration failed"},

		{"login wrong credentials", &fakeAuthService{loginErr: ErrInvalidCredentials},
			codedRequest{method: "POST", path: "/auth/login", body: loginBody},
			401, "invalid_credentials", "invalid email or password"},
		{"login malformed email", &fakeAuthService{},
			codedRequest{method: "POST", path: "/auth/login", body: `{"email":"nope","password":"password123"}`},
			401, "invalid_credentials", "invalid email or password"},

		{"google provider not configured", &fakeAuthService{googleErr: ErrProviderNotConfigured},
			codedRequest{method: "POST", path: "/auth/google", body: idTokenBody},
			503, "provider_not_configured", "google sign-in is not configured"},
		{"google invalid id token", &fakeAuthService{googleErr: fmt.Errorf("%w: token is expired", ErrInvalidIDToken)},
			codedRequest{method: "POST", path: "/auth/google", body: idTokenBody},
			401, "invalid_id_token", "invalid id token"},
		{"google missing id token keeps the default", &fakeAuthService{},
			codedRequest{method: "POST", path: "/auth/google", body: `{}`},
			400, "bad_request", "id_token is required"},
		{"google failure keeps the default", &fakeAuthService{googleErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/google", body: idTokenBody},
			500, "internal_error", "google sign-in failed"},

		{"apple provider not configured", &fakeAuthService{appleErr: ErrProviderNotConfigured},
			codedRequest{method: "POST", path: "/auth/apple", body: idTokenBody},
			503, "provider_not_configured", "apple sign-in is not configured"},
		{"apple invalid id token", &fakeAuthService{appleErr: fmt.Errorf("%w: audience not allowed", ErrInvalidIDToken)},
			codedRequest{method: "POST", path: "/auth/apple", body: idTokenBody},
			401, "invalid_id_token", "invalid id token"},
		{"apple failure keeps the default", &fakeAuthService{appleErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/apple", body: idTokenBody},
			500, "internal_error", "apple sign-in failed"},

		{"refresh without a token", &fakeAuthService{},
			codedRequest{method: "POST", path: "/auth/refresh"},
			401, "missing_refresh_token", "missing refresh token"},
		{"refresh with a stale cookie", &fakeAuthService{refreshErr: ErrInvalidRefresh},
			codedRequest{method: "POST", path: "/auth/refresh", cookie: "stale"},
			401, "invalid_refresh_token", "invalid or expired refresh token"},
		{"refresh with a stale body token", &fakeAuthService{refreshErr: ErrInvalidRefresh},
			codedRequest{method: "POST", path: "/auth/refresh", body: `{"refresh_token":"stale"}`},
			401, "invalid_refresh_token", "invalid or expired refresh token"},
		{"refresh failure keeps the default", &fakeAuthService{refreshErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/refresh", cookie: "x"},
			500, "internal_error", "refresh failed"},

		{"logout failure keeps the default", &fakeAuthService{logoutErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/logout", body: `{"refresh_token":"x"}`},
			500, "internal_error", "logout failed"},

		{"link invalid code", &fakeAuthService{linkErr: ErrInvalidLinkCode},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			400, "invalid_link_code", "invalid or expired code"},
		{"link codes unavailable", &fakeAuthService{linkErr: ErrLinkCodesUnavailable},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			503, "telegram_link_unavailable", "telegram linking is unavailable"},
		{"link account gone", &fakeAuthService{linkErr: pgx.ErrNoRows},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			404, "user_not_found", httpx.MsgUserNotFound},
		{"link conflict", &fakeAuthService{linkErr: ErrTelegramLinkConflict},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			409, "telegram_link_conflict", "telegram is linked to an account that cannot be merged"},
		{"link already linked", &fakeAuthService{linkErr: ErrTelegramAlreadyLinked},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			409, "telegram_already_linked", "account already has another telegram"},
		{"link invalid json keeps the default", &fakeAuthService{},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: "not-json", authorization: "user"},
			400, "bad_request", httpx.MsgInvalidJSON},
		{"link failure keeps the default", &fakeAuthService{linkErr: errors.New("db down")},
			codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
			500, "internal_error", "telegram link failed"},

		{"me account gone", &fakeAuthService{profileErr: pgx.ErrNoRows},
			codedRequest{method: "GET", path: "/auth/me", authorization: "user"},
			404, "user_not_found", httpx.MsgUserNotFound},
		{"me failure keeps the default", &fakeAuthService{profileErr: errors.New("db down")},
			codedRequest{method: "GET", path: "/auth/me", authorization: "user"},
			500, "internal_error", httpx.MsgInternal},
		{"update me account gone", &fakeAuthService{profileErr: pgx.ErrNoRows},
			codedRequest{method: "PATCH", path: "/auth/me", body: `{"name":"Anna"}`, authorization: "user"},
			404, "user_not_found", httpx.MsgUserNotFound},

		{"no authorization header", &fakeAuthService{},
			codedRequest{method: "GET", path: "/auth/me"},
			401, "missing_authorization", httpx.MsgMissingAuth},
		{"wrong authorization scheme", &fakeAuthService{},
			codedRequest{method: "GET", path: "/auth/me", authorization: "Basic abc"},
			401, "missing_authorization", httpx.MsgMissingBearerToken},
		{"garbage token", &fakeAuthService{},
			codedRequest{method: "GET", path: "/auth/me", authorization: "Bearer not-a-jwt"},
			401, "invalid_token", httpx.MsgInvalidToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := tt.req.serve(t, codedEcho(testAuthHandlers(tt.svc)))

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestErrorCodes_tokenWithoutAUserIDIsInvalid(t *testing.T) {
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    Issuer,
		Subject:   "not-a-uuid",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}

	rec := codedRequest{method: "GET", path: "/auth/me", authorization: "Bearer " + signed}.
		serve(t, codedEcho(testAuthHandlers(&fakeAuthService{})))

	assertCodedError(t, rec, 401, "invalid_token", httpx.MsgInvalidToken)
}

func TestErrorCodes_rateLimitedEveryRoute(t *testing.T) {
	routes := []codedRequest{
		{method: "POST", path: "/auth/register", body: registerBody},
		{method: "POST", path: "/auth/login", body: loginBody},
		{method: "POST", path: "/auth/google", body: idTokenBody},
		{method: "POST", path: "/auth/apple", body: idTokenBody},
		{method: "POST", path: "/auth/refresh", cookie: "x"},
		{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"},
	}
	for _, r := range routes {
		t.Run(r.path, func(t *testing.T) {
			mr := miniredis.RunT(t)
			h := testAuthHandlers(&fakeAuthService{})
			h.limiter = NewRateLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 1, time.Minute, 1, time.Minute)
			e := codedEcho(h)

			r.serve(t, e)
			rec := r.serve(t, e)

			assertCodedError(t, rec, 429, "rate_limited", "too many requests")
		})
	}
}

func TestErrorCodes_appleCallbackKeepsTheDefaults(t *testing.T) {
	tests := []struct {
		name        string
		androidPkg  string
		contentType string
		body        string
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"not configured", "", echo.MIMEApplicationForm, "code=c", 503, "service_unavailable", "apple sign-in return is not configured"},
		{"empty form", "com.mentorix.mobile", echo.MIMEApplicationForm, "", 400, "bad_request", "form body is required"},
		{"broken percent escape", "com.mentorix.mobile", echo.MIMEApplicationForm, "state=%zz", 400, "bad_request", "invalid form body"},
		{"body over the limit", "com.mentorix.mobile", echo.MIMEApplicationForm, "code=" + strings.Repeat("a", appleCallbackBodyLimit), 413, "payload_too_large", "Request Entity Too Large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := testAuthHandlers(&fakeAuthService{})
			WithAppleAndroidPackage(tt.androidPkg)(h)
			e := codedEcho(h)
			req := httptest.NewRequest(http.MethodPost, appleCallbackPath, strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, tt.contentType)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestErrorCodes_telegramLinkConflictsAreToldApart(t *testing.T) {
	codeOf := func(linkErr error) string {
		rec := codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"}.
			serve(t, codedEcho(testAuthHandlers(&fakeAuthService{linkErr: linkErr})))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%v: status = %d, want 409", linkErr, rec.Code)
		}
		var body httpx.ErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Code
	}

	conflict, already := codeOf(ErrTelegramLinkConflict), codeOf(ErrTelegramAlreadyLinked)

	if conflict == already {
		t.Fatalf("both 409 answers carry the code %q", conflict)
	}
}

// Each Telegram link refusal keeps its cause for the request log and errors.Is,
// and none of it reaches the body.
func TestErrorCodes_telegramLinkKeepsTheCauseOutOfTheBody(t *testing.T) {
	tests := []struct {
		name        string
		sentinel    error
		detail      string
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"codes unavailable", ErrLinkCodesUnavailable, "redis at 10.0.0.5 refused the connection",
			503, "telegram_link_unavailable", "telegram linking is unavailable"},
		{"link conflict", ErrTelegramLinkConflict, "identity 42 has a trainer link",
			409, "telegram_link_conflict", "telegram is linked to an account that cannot be merged"},
		{"already linked", ErrTelegramAlreadyLinked, "identity 42 belongs to user 7",
			409, "telegram_already_linked", "account already has another telegram"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			cause := fmt.Errorf("%w: %s", tt.sentinel, tt.detail)
			e := codedEcho(testAuthHandlers(&fakeAuthService{linkErr: cause}),
				httpx.RequestLog(slog.New(slog.NewTextHandler(&logs, nil))))

			rec := codedRequest{method: "POST", path: "/auth/telegram/link", body: linkBody, authorization: "user"}.serve(t, e)

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
			if strings.Contains(rec.Body.String(), tt.detail) {
				t.Errorf("body leaks the cause: %s", rec.Body)
			}
			for _, want := range []string{tt.wantCode, tt.detail} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("log %q does not contain %q", logs.String(), want)
				}
			}
		})
	}
}
