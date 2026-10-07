package auth

import (
	"encoding/json"
	"errors"
	"fmt"
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
)

const attachIdentityBody = `{"provider":"google","id_token":"raw-id-token"}`

func attachIdentityRequest(h *Handlers, uid uuid.UUID, body string) *httptest.ResponseRecorder {
	e := echo.New()
	e.POST("/auth/me/identities", func(c echo.Context) error {
		if uid != uuid.Nil {
			c.Set(ContextUserIDKey, uid)
		}
		return h.AttachIdentity(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/me/identities", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return serve(e, req)
}

func decodeMeResponse(t *testing.T, rec *httptest.ResponseRecorder) MeResponse {
	t.Helper()
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestAttachIdentity_successReturnsProfileWithNewMethod(t *testing.T) {
	uid := uuid.New()
	svc := &fakeAuthService{profile: UserProfile{
		Email:         "person@example.com",
		Name:          "Person",
		Roles:         []string{RoleTrainer},
		SignInMethods: []string{ProviderEmailPassword, ProviderGoogle},
	}}

	rec := attachIdentityRequest(testAuthHandlers(svc), uid, attachIdentityBody)

	assertHTTPStatus(t, rec, http.StatusOK)
	resp := decodeMeResponse(t, rec)
	if resp.UserID != uid.String() || resp.Email != "person@example.com" {
		t.Fatalf("resp = %+v", resp)
	}
	if got, want := strings.Join(resp.SignInMethods, ","), "email_password,google"; got != want {
		t.Errorf("sign_in_methods = %q, want %q", got, want)
	}
	want := attachCall{userID: uid, provider: ProviderGoogle, idToken: "raw-id-token"}
	if len(svc.attachCalls) != 1 || svc.attachCalls[0] != want {
		t.Fatalf("service calls = %+v, want [%+v]", svc.attachCalls, want)
	}
}

func TestAttachIdentity_passesCurrentPasswordToService(t *testing.T) {
	uid := uuid.New()
	svc := &fakeAuthService{}

	rec := attachIdentityRequest(testAuthHandlers(svc), uid, `{"provider":"apple","id_token":"raw","current_password":"s3cret-pass"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	want := attachCall{userID: uid, provider: ProviderApple, idToken: "raw", currentPassword: "s3cret-pass"}
	if len(svc.attachCalls) != 1 || svc.attachCalls[0] != want {
		t.Fatalf("service calls = %+v, want [%+v]", svc.attachCalls, want)
	}
}

func TestAttachIdentity_forbiddenResponseDoesNotEchoPassword(t *testing.T) {
	svc := &fakeAuthService{attachErr: ErrPasswordIncorrect}

	rec := attachIdentityRequest(testAuthHandlers(svc), uuid.New(), `{"provider":"apple","id_token":"raw","current_password":"s3cret-pass"}`)

	assertHTTPStatus(t, rec, http.StatusForbidden)
	if strings.Contains(rec.Body.String(), "s3cret-pass") {
		t.Errorf("response echoes the password: %s", rec.Body.String())
	}
}

func TestAttachIdentity_badRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `nope`},
		{name: "missing provider", body: `{"id_token":"raw"}`},
		{name: "unknown provider", body: `{"provider":"facebook","id_token":"raw"}`},
		{name: "email_password is not a social provider", body: `{"provider":"email_password","id_token":"raw"}`},
		{name: "provider is case sensitive", body: `{"provider":"Apple","id_token":"raw"}`},
		{name: "missing id_token", body: `{"provider":"apple"}`},
		{name: "blank id_token", body: `{"provider":"apple","id_token":"  "}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := attachIdentityRequest(testAuthHandlers(svc), uuid.New(), tt.body)

			assertHTTPStatus(t, rec, http.StatusBadRequest)
			if len(svc.attachCalls) != 0 {
				t.Errorf("service called for a rejected request: %+v", svc.attachCalls)
			}
		})
	}
}

func TestAttachIdentity_serviceErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{name: "provider not configured", err: ErrProviderNotConfigured, wantStatus: http.StatusBadRequest, wantMessage: "provider not configured"},
		{name: "invalid id token", err: ErrInvalidIDToken, wantStatus: http.StatusUnauthorized, wantMessage: "invalid id token"},
		{name: "user not found", err: fmt.Errorf("attach: %w", pgx.ErrNoRows), wantStatus: http.StatusNotFound, wantMessage: httpx.MsgUserNotFound},
		{name: "identity of another account", err: ErrIdentityBelongsToAnotherAccount, wantStatus: http.StatusConflict, wantMessage: "this sign-in already belongs to another account"},
		{name: "password required", err: ErrPasswordRequired, wantStatus: http.StatusForbidden, wantMessage: "current password is required"},
		{name: "password incorrect", err: ErrPasswordIncorrect, wantStatus: http.StatusForbidden, wantMessage: "current password is incorrect"},
		{name: "admin account", err: ErrAdminCannotAttachIdentity, wantStatus: http.StatusForbidden, wantMessage: "admins cannot add sign-in methods"},
		{name: "provider unavailable", err: fmt.Errorf("fetch keys: %w", ErrIDTokenProviderUnavailable), wantStatus: http.StatusServiceUnavailable, wantMessage: "sign-in provider unavailable"},
		{name: "unexpected", err: errors.New("db down"), wantStatus: http.StatusInternalServerError, wantMessage: "attach identity failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := attachIdentityRequest(testAuthHandlers(&fakeAuthService{attachErr: tt.err}), uuid.New(), attachIdentityBody)

			assertHTTPStatus(t, rec, tt.wantStatus)
			if got := httpErrorMessage(t, rec); got != tt.wantMessage {
				t.Errorf("message = %q, want %q", got, tt.wantMessage)
			}
			if strings.Contains(rec.Body.String(), "raw-id-token") {
				t.Error("response echoes the id token")
			}
		})
	}
}

func TestAttachIdentity_profileErrors(t *testing.T) {
	tests := []struct {
		name       string
		svc        *fakeAuthService
		uid        uuid.UUID
		wantStatus int
	}{
		{name: "profile not found", svc: &fakeAuthService{profileErr: pgx.ErrNoRows}, uid: uuid.New(), wantStatus: http.StatusNotFound},
		{name: "profile failure", svc: &fakeAuthService{profileErr: errors.New("db down")}, uid: uuid.New(), wantStatus: http.StatusInternalServerError},
		{name: "no user in context", svc: &fakeAuthService{}, uid: uuid.Nil, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := attachIdentityRequest(testAuthHandlers(tt.svc), tt.uid, attachIdentityBody)

			assertHTTPStatus(t, rec, tt.wantStatus)
		})
	}
}

func TestAttachIdentity_providerUnavailableKeepsDetailOutOfResponse(t *testing.T) {
	const detail = "dial tcp 10.1.2.3:443: connect: connection refused"
	svc := &fakeAuthService{attachErr: fmt.Errorf("%s: %w", detail, ErrIDTokenProviderUnavailable)}

	rec := attachIdentityRequest(testAuthHandlers(svc), uuid.New(), attachIdentityBody)

	assertHTTPStatus(t, rec, http.StatusServiceUnavailable)
	if strings.Contains(rec.Body.String(), "10.1.2.3") || strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("response body leaks the internal error: %s", rec.Body.String())
	}
}

func TestAttachIdentity_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &fakeAuthService{}
	h := testAuthHandlers(svc)
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	uid := uuid.New()

	assertHTTPStatus(t, attachIdentityRequest(h, uid, attachIdentityBody), http.StatusOK)
	rec := attachIdentityRequest(h, uid, attachIdentityBody)

	assertHTTPStatus(t, rec, http.StatusTooManyRequests)
	if len(svc.attachCalls) != 1 {
		t.Fatalf("service calls = %d, want only the first request", len(svc.attachCalls))
	}
}

func TestAttachIdentity_invalidRequestsDoNotSpendRateLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	uid := uuid.New()

	assertHTTPStatus(t, attachIdentityRequest(h, uid, `{"provider":"nope","id_token":"raw"}`), http.StatusBadRequest)
	assertHTTPStatus(t, attachIdentityRequest(h, uid, attachIdentityBody), http.StatusOK)
}

func TestAttachIdentity_rateLimitBackendError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	mr.Close()

	assertHTTPStatus(t, attachIdentityRequest(h, uuid.New(), attachIdentityBody), http.StatusInternalServerError)
}

func TestHandlers_Mount_attachIdentityRequiresBearer(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)

	rec := postAuthJSON(e, "/auth/me/identities", attachIdentityBody)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestMe_signInMethods(t *testing.T) {
	tests := []struct {
		name    string
		methods []string
		want    string
	}{
		{name: "nil is an empty array", methods: nil, want: `"sign_in_methods":[]`},
		{name: "empty is an empty array", methods: []string{}, want: `"sign_in_methods":[]`},
		{name: "listed in order", methods: []string{ProviderApple, ProviderEmailPassword}, want: `"sign_in_methods":["apple","email_password"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := testAuthHandlers(&fakeAuthService{profile: UserProfile{SignInMethods: tt.methods}})
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/auth/me", nil), rec)
			c.Set(ContextUserIDKey, uuid.New())

			if err := h.Me(c); err != nil {
				t.Fatalf("Me: %v", err)
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body %s does not contain %s", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestAddRole_responseCarriesSignInMethods(t *testing.T) {
	svc := &fakeAuthService{profile: UserProfile{Roles: []string{RoleClient}, SignInMethods: []string{ProviderTelegram}}}

	rec := addRoleRequest(testAuthHandlers(svc), uuid.New(), `{"role":"client"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if got := decodeMeResponse(t, rec).SignInMethods; len(got) != 1 || got[0] != ProviderTelegram {
		t.Errorf("sign_in_methods = %v, want [telegram]", got)
	}
}

func TestAttachIdentity_hasItsOwnRateLimitBucket(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	uid := uuid.New()
	e := echo.New()
	e.POST("/auth/login", h.Login)
	e.POST("/auth/me/identities", func(c echo.Context) error {
		c.Set(ContextUserIDKey, uid)
		return h.AttachIdentity(c)
	})

	assertHTTPStatus(t, postAuthJSON(e, "/auth/me/identities", attachIdentityBody), http.StatusOK)
	assertHTTPStatus(t, postAuthJSON(e, "/auth/login", `{"email":"user@example.com","password":"password123"}`), http.StatusOK)
	assertHTTPStatus(t, postAuthJSON(e, "/auth/me/identities", attachIdentityBody), http.StatusTooManyRequests)
}

func TestAttachIdentity_currentPasswordTooLongIsRejectedBeforeRateLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &fakeAuthService{}
	h := testAuthHandlers(svc)
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	uid := uuid.New()
	tooLong := strings.Repeat("a", maxPasswordLen+1)
	atMax := strings.Repeat("a", maxPasswordLen)

	rec := attachIdentityRequest(h, uid, `{"provider":"google","id_token":"raw","current_password":"`+tooLong+`"}`)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
	if got := httpErrorMessage(t, rec); got != "invalid current_password" {
		t.Errorf("message = %q, want invalid current_password", got)
	}
	if len(svc.attachCalls) != 0 {
		t.Errorf("service called for a rejected request: %+v", svc.attachCalls)
	}
	// The rejected request spent no budget, and a password at the limit passes.
	rec = attachIdentityRequest(h, uid, `{"provider":"google","id_token":"raw","current_password":"`+atMax+`"}`)
	assertHTTPStatus(t, rec, http.StatusOK)
}
