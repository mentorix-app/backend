package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

const socialLoginBody = `{"provider":"google","id_token":"raw-id-token","name":"Person"}`

func socialLoginEcho(h *Handlers) *echo.Echo {
	e := echo.New()
	e.POST("/auth/social-login", h.SocialLogin)
	return e
}

func httpErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error body %q: %v", rec.Body.String(), err)
	}
	return resp.Message
}

// serve runs the request through echo so *echo.HTTPError values render as JSON.
func serve(e *echo.Echo, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHandlers_Mount_registersSocialRoutes(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)
	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	for _, key := range []string{"POST /auth/social-login", "POST /auth/me/roles"} {
		if !found[key] {
			t.Fatalf("missing route %s", key)
		}
	}

	rec := postAuthJSON(e, "/auth/me/roles", `{"role":"client"}`)
	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestSocialLogin_existingAccountReturns200(t *testing.T) {
	svc := &fakeAuthService{socialIssued: IssuedAuth{RefreshToken: "app-refresh", Email: "person@example.com"}}
	e := socialLoginEcho(testAuthHandlers(svc))

	rec := postAuthJSON(e, "/auth/social-login", socialLoginBody)

	assertHTTPStatus(t, rec, http.StatusOK)
	resp := decodeTokenResponse(t, rec)
	if resp.AccessToken == "" || resp.UserID == "" || resp.Email != "person@example.com" || resp.TokenType != TokenTypeBearer {
		t.Fatalf("resp = %+v", resp)
	}
	want := socialLoginCall{provider: ProviderGoogle, idToken: "raw-id-token", name: "Person"}
	if len(svc.socialCalls) != 1 || svc.socialCalls[0] != want {
		t.Fatalf("service calls = %+v, want [%+v]", svc.socialCalls, want)
	}
}

func TestSocialLogin_newAccountReturns201(t *testing.T) {
	svc := &fakeAuthService{socialCreated: true}
	e := socialLoginEcho(testAuthHandlers(svc))

	rec := postAuthJSON(e, "/auth/social-login", `{"provider":"apple","id_token":"raw"}`)

	assertHTTPStatus(t, rec, http.StatusCreated)
	if len(svc.socialCalls) != 1 || svc.socialCalls[0].provider != ProviderApple || svc.socialCalls[0].name != "" {
		t.Fatalf("service calls = %+v", svc.socialCalls)
	}
}

func TestSocialLogin_tokenDelivery(t *testing.T) {
	tests := []struct {
		name       string
		created    bool
		field      string
		wantStatus int
		wantBody   bool
	}{
		{name: "existing, body", created: false, field: `,"token_delivery":"body"`, wantStatus: http.StatusOK, wantBody: true},
		{name: "new, body", created: true, field: `,"token_delivery":"body"`, wantStatus: http.StatusCreated, wantBody: true},
		{name: "existing, default cookie", created: false, field: ``, wantStatus: http.StatusOK},
		{name: "new, default cookie", created: true, field: ``, wantStatus: http.StatusCreated},
		{name: "existing, explicit cookie", created: false, field: `,"token_delivery":"cookie"`, wantStatus: http.StatusOK},
		{name: "new, empty delivery", created: true, field: `,"token_delivery":""`, wantStatus: http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{socialCreated: tt.created, socialIssued: IssuedAuth{RefreshToken: "app-refresh"}}
			e := socialLoginEcho(testAuthHandlers(svc))

			rec := postAuthJSON(e, "/auth/social-login", `{"provider":"google","id_token":"raw"`+tt.field+`}`)

			assertHTTPStatus(t, rec, tt.wantStatus)
			if got := rec.Header().Get(echo.HeaderCacheControl); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			cookies := rec.Result().Cookies()
			if tt.wantBody {
				if got := decodeTokenResponse(t, rec).RefreshToken; got != "app-refresh" {
					t.Errorf("refresh_token = %q, want app-refresh", got)
				}
				if len(cookies) != 0 {
					t.Errorf("got %d cookies, want none", len(cookies))
				}
				return
			}
			if strings.Contains(rec.Body.String(), "refresh_token") {
				t.Errorf("body must not contain refresh_token: %s", rec.Body.String())
			}
			if len(cookies) != 1 || cookies[0].Value != "app-refresh" {
				t.Errorf("cookies = %v, want one refresh cookie", cookies)
			}
		})
	}
}

func TestSocialLogin_badRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `not-json`},
		{name: "missing provider", body: `{"id_token":"raw"}`},
		{name: "unknown provider", body: `{"provider":"facebook","id_token":"raw"}`},
		{name: "email_password is not a social provider", body: `{"provider":"email_password","id_token":"raw"}`},
		{name: "provider is case sensitive", body: `{"provider":"Google","id_token":"raw"}`},
		{name: "missing id_token", body: `{"provider":"google"}`},
		{name: "blank id_token", body: `{"provider":"google","id_token":"   "}`},
		{name: "invalid token_delivery", body: `{"provider":"google","id_token":"raw","token_delivery":"header"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			e := socialLoginEcho(testAuthHandlers(svc))

			rec := postAuthJSON(e, "/auth/social-login", tt.body)

			assertHTTPStatus(t, rec, http.StatusBadRequest)
			if len(svc.socialCalls) != 0 {
				t.Errorf("service called for a rejected request: %+v", svc.socialCalls)
			}
		})
	}
}

func TestSocialLogin_serviceErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{name: "invalid id token", err: ErrInvalidIDToken, wantStatus: http.StatusUnauthorized, wantMessage: "invalid id token"},
		{name: "wrapped invalid id token", err: errors.Join(errors.New("detail"), ErrInvalidIDToken), wantStatus: http.StatusUnauthorized, wantMessage: "invalid id token"},
		{name: "provider unavailable", err: fmt.Errorf("fetch keys: %w", ErrIDTokenProviderUnavailable), wantStatus: http.StatusServiceUnavailable, wantMessage: "sign-in provider unavailable"},
		{name: "provider not configured", err: ErrProviderNotConfigured, wantStatus: http.StatusBadRequest, wantMessage: "provider not configured"},
		{name: "email belongs to another account", err: ErrEmailBelongsToAnotherAccount, wantStatus: http.StatusConflict, wantMessage: "an account with this email already exists"},
		{name: "unexpected", err: errors.New("db down"), wantStatus: http.StatusInternalServerError, wantMessage: "social login failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := socialLoginEcho(testAuthHandlers(&fakeAuthService{socialErr: tt.err}))

			rec := postAuthJSON(e, "/auth/social-login", socialLoginBody)

			assertHTTPStatus(t, rec, tt.wantStatus)
			if got := httpErrorMessage(t, rec); got != tt.wantMessage {
				t.Errorf("message = %q, want %q", got, tt.wantMessage)
			}
			if got := rec.Result().Cookies(); len(got) != 0 {
				t.Errorf("cookies = %v, want none on failure", got)
			}
			if strings.Contains(rec.Body.String(), "raw-id-token") {
				t.Error("response echoes the id token")
			}
		})
	}
}

func TestSocialLogin_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &fakeAuthService{}
	h := testAuthHandlers(svc)
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := socialLoginEcho(h)

	assertHTTPStatus(t, postAuthJSON(e, "/auth/social-login", socialLoginBody), http.StatusOK)
	rec := postAuthJSON(e, "/auth/social-login", socialLoginBody)

	assertHTTPStatus(t, rec, http.StatusTooManyRequests)
	if len(svc.socialCalls) != 1 {
		t.Fatalf("service calls = %d, want only the first request", len(svc.socialCalls))
	}
}

func TestSocialLogin_invalidRequestsDoNotSpendRateLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := socialLoginEcho(h)

	assertHTTPStatus(t, postAuthJSON(e, "/auth/social-login", `{"provider":"nope","id_token":"raw"}`), http.StatusBadRequest)
	assertHTTPStatus(t, postAuthJSON(e, "/auth/social-login", socialLoginBody), http.StatusOK)
}

func TestSocialLogin_rateLimitBackendError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	mr.Close()
	e := socialLoginEcho(h)

	assertHTTPStatus(t, postAuthJSON(e, "/auth/social-login", socialLoginBody), http.StatusInternalServerError)
}

func addRoleRequest(h *Handlers, uid uuid.UUID, body string) *httptest.ResponseRecorder {
	e := echo.New()
	e.POST("/auth/me/roles", func(c echo.Context) error {
		if uid != uuid.Nil {
			c.Set(ContextUserIDKey, uid)
		}
		return h.AddRole(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/me/roles", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return serve(e, req)
}

func TestAddRole_success(t *testing.T) {
	createdAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	for _, role := range []string{RoleClient, RoleTrainer} {
		t.Run(role, func(t *testing.T) {
			uid := uuid.New()
			svc := &fakeAuthService{profile: UserProfile{Email: "person@example.com", Name: "Person", CreatedAt: createdAt, Roles: []string{role}}}
			h := testAuthHandlers(svc)

			rec := addRoleRequest(h, uid, `{"role":"`+role+`"}`)

			assertHTTPStatus(t, rec, http.StatusOK)
			var resp MeResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.UserID != uid.String() || resp.Email != "person@example.com" || len(resp.Roles) != 1 || resp.Roles[0] != role {
				t.Fatalf("resp = %+v", resp)
			}
			if len(svc.addRoleCalls) != 1 || svc.addRoleCalls[0] != (addRoleCall{userID: uid, role: role}) {
				t.Fatalf("service calls = %+v", svc.addRoleCalls)
			}
		})
	}
}

func TestAddRole_trainerResponseCarriesFreePlan(t *testing.T) {
	plan := subscription.PlanFree
	h := testAuthHandlers(&fakeAuthService{profile: UserProfile{Roles: []string{RoleTrainer}}})
	h.subs = &fakeSubs{sub: &subscription.Subscription{Plan: plan}}

	rec := addRoleRequest(h, uuid.New(), `{"role":"trainer"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != plan {
		t.Fatalf("subscription = %+v, want free plan", resp.Subscription)
	}
}

func TestAddRole_badRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `nope`},
		{name: "missing role", body: `{}`},
		{name: "admin is not self-assignable", body: `{"role":"admin"}`},
		{name: "unknown role", body: `{"role":"owner"}`},
		{name: "role is case sensitive", body: `{"role":"Trainer"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := addRoleRequest(testAuthHandlers(svc), uuid.New(), tt.body)

			assertHTTPStatus(t, rec, http.StatusBadRequest)
			if len(svc.addRoleCalls) != 0 {
				t.Errorf("service called for a rejected request: %+v", svc.addRoleCalls)
			}
		})
	}
}

func TestAddRole_errors(t *testing.T) {
	tests := []struct {
		name       string
		svc        *fakeAuthService
		uid        uuid.UUID
		wantStatus int
	}{
		{name: "user no longer exists", svc: &fakeAuthService{addRoleErr: pgx.ErrNoRows}, uid: uuid.New(), wantStatus: http.StatusNotFound},
		{name: "role conflict", svc: &fakeAuthService{addRoleErr: ErrRoleConflict}, uid: uuid.New(), wantStatus: http.StatusConflict},
		{name: "store failure", svc: &fakeAuthService{addRoleErr: errors.New("db down")}, uid: uuid.New(), wantStatus: http.StatusInternalServerError},
		{name: "profile not found", svc: &fakeAuthService{profileErr: pgx.ErrNoRows}, uid: uuid.New(), wantStatus: http.StatusNotFound},
		{name: "profile failure", svc: &fakeAuthService{profileErr: errors.New("db down")}, uid: uuid.New(), wantStatus: http.StatusInternalServerError},
		{name: "no user in context", svc: &fakeAuthService{}, uid: uuid.Nil, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := addRoleRequest(testAuthHandlers(tt.svc), tt.uid, `{"role":"client"}`)

			assertHTTPStatus(t, rec, tt.wantStatus)
		})
	}
}

func TestAddRole_userNotFoundMessage(t *testing.T) {
	for name, svc := range map[string]*fakeAuthService{
		"profile read":  {profileErr: pgx.ErrNoRows},
		"role insert":   {addRoleErr: pgx.ErrNoRows},
		"wrapped error": {addRoleErr: fmt.Errorf("grant: %w", pgx.ErrNoRows)},
	} {
		t.Run(name, func(t *testing.T) {
			rec := addRoleRequest(testAuthHandlers(svc), uuid.New(), `{"role":"client"}`)
			assertHTTPStatus(t, rec, http.StatusNotFound)
			if got := httpErrorMessage(t, rec); got != httpx.MsgUserNotFound {
				t.Errorf("message = %q, want %q", got, httpx.MsgUserNotFound)
			}
		})
	}
}

func TestMe_emptyRolesIsArray(t *testing.T) {
	for _, roles := range [][]string{nil, {}} {
		e := echo.New()
		h := testAuthHandlers(&fakeAuthService{profile: UserProfile{Email: "", Roles: roles}})
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(ContextUserIDKey, uuid.New())

		if err := h.Me(c); err != nil {
			t.Fatalf("Me: %v", err)
		}
		if !strings.Contains(rec.Body.String(), `"roles":[]`) {
			t.Errorf("roles = %v: body %s does not contain \"roles\":[]", roles, rec.Body.String())
		}
	}
}

func TestSocialRequestTypes_jsonKeys(t *testing.T) {
	var s SocialLoginRequest
	if err := json.Unmarshal([]byte(`{"provider":"apple","id_token":"t","name":"n","token_delivery":"body"}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s != (SocialLoginRequest{Provider: "apple", IDToken: "t", Name: "n", TokenDelivery: "body"}) {
		t.Errorf("SocialLoginRequest = %+v", s)
	}
	var r AddRoleRequest
	if err := json.Unmarshal([]byte(`{"role":"client"}`), &r); err != nil || r.Role != "client" {
		t.Errorf("AddRoleRequest = %+v, err = %v", r, err)
	}
}

func TestSocialLogin_providerUnavailableKeepsDetailOutOfResponseButInLog(t *testing.T) {
	const detail = "dial tcp 10.1.2.3:443: connect: connection refused"
	svc := &fakeAuthService{socialErr: fmt.Errorf("%s: %w", detail, ErrIDTokenProviderUnavailable)}
	var logs bytes.Buffer
	e := echo.New()
	e.Use(httpx.RequestLog(slog.New(slog.NewJSONHandler(&logs, nil))))
	e.POST("/auth/social-login", testAuthHandlers(svc).SocialLogin)

	rec := postAuthJSON(e, "/auth/social-login", socialLoginBody)

	assertHTTPStatus(t, rec, http.StatusServiceUnavailable)
	if strings.Contains(rec.Body.String(), "10.1.2.3") || strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("response body leaks the internal error: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "connection refused") {
		t.Errorf("request log does not carry the internal error: %s", logs.String())
	}
}

func TestSocialLogin_nameLength(t *testing.T) {
	tests := []struct {
		name       string
		given      string
		wantStatus int
	}{
		{name: "100 ASCII runes", given: strings.Repeat("a", 100), wantStatus: http.StatusOK},
		{name: "100 multibyte runes", given: strings.Repeat("Ж", 100), wantStatus: http.StatusOK},
		{name: "101 runes", given: strings.Repeat("a", 101), wantStatus: http.StatusBadRequest},
		{name: "101 multibyte runes", given: strings.Repeat("Ж", 101), wantStatus: http.StatusBadRequest},
		{name: "100 runes plus padding is trimmed first", given: "  " + strings.Repeat("a", 100) + "  ", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			e := socialLoginEcho(testAuthHandlers(svc))
			body, err := json.Marshal(SocialLoginRequest{Provider: ProviderGoogle, IDToken: "raw", Name: tt.given})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			rec := postAuthJSON(e, "/auth/social-login", string(body))

			assertHTTPStatus(t, rec, tt.wantStatus)
			if tt.wantStatus == http.StatusBadRequest {
				if got := httpErrorMessage(t, rec); got != "name is too long" {
					t.Errorf("message = %q", got)
				}
				if len(svc.socialCalls) != 0 {
					t.Error("service called for a rejected request")
				}
			}
		})
	}
}

func TestSocialLogin_invalidName(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "NUL rune", body: `{"provider":"google","id_token":"raw","name":"Ann\u0000a"}`},
		{name: "only NUL", body: `{"provider":"google","id_token":"raw","name":"\u0000"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			e := socialLoginEcho(testAuthHandlers(svc))

			rec := postAuthJSON(e, "/auth/social-login", tt.body)

			assertHTTPStatus(t, rec, http.StatusBadRequest)
			if got := httpErrorMessage(t, rec); got != "invalid name" {
				t.Errorf("message = %q, want invalid name", got)
			}
			if len(svc.socialCalls) != 0 {
				t.Error("service called for a rejected request")
			}
		})
	}
}
