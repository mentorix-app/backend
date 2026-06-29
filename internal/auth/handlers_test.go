package auth

import (
	"context"
	"encoding/json"
	"errors"
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

	"mentorix-backend/internal/config"
	httpx "mentorix-backend/internal/http"
)

type fakeAuthService struct {
	registerIssued IssuedAuth
	registerErr    error
	loginIssued    IssuedAuth
	loginErr       error
	refreshIssued  IssuedAuth
	refreshErr     error
	logoutErr      error
	logoutAllErr   error
	profile        UserProfile
	profileErr     error
}

func (f *fakeAuthService) RegisterTrainer(_ context.Context, email, _, _ string) (IssuedAuth, error) {
	if f.registerErr != nil {
		return IssuedAuth{}, f.registerErr
	}
	out := f.registerIssued
	if out.Email == "" {
		out.Email = email
	}
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "test-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "test-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) Login(_ context.Context, email, _ string) (IssuedAuth, error) {
	if f.loginErr != nil {
		return IssuedAuth{}, f.loginErr
	}
	out := f.loginIssued
	if out.Email == "" {
		out.Email = email
	}
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "test-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "test-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) Refresh(_ context.Context, _ string) (IssuedAuth, error) {
	if f.refreshErr != nil {
		return IssuedAuth{}, f.refreshErr
	}
	out := f.refreshIssued
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "new-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "new-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) Logout(_ context.Context, _ string) error {
	return f.logoutErr
}

func (f *fakeAuthService) LogoutAll(_ context.Context, _ uuid.UUID) error {
	return f.logoutAllErr
}

func (f *fakeAuthService) UserProfile(_ context.Context, _ uuid.UUID) (UserProfile, error) {
	if f.profileErr != nil {
		return UserProfile{}, f.profileErr
	}
	return f.profile, nil
}

func (f *fakeAuthService) UpdateProfileName(_ context.Context, _ uuid.UUID, name string) (UserProfile, error) {
	if f.profileErr != nil {
		return UserProfile{}, f.profileErr
	}
	out := f.profile
	out.Name = name
	return out, nil
}

func testAuthHandlers(svc credentialService) *Handlers {
	return &Handlers{
		svc:        svc,
		jwtSecret:  testJWTSecret,
		cookie:     config.RefreshCookieSettings{Name: "mentorix_refresh", Path: "/auth"},
		refreshTTL: time.Hour,
		limiter:    nil,
	}
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(nil, testJWTSecret, config.RefreshCookieSettings{Name: "refresh", Path: "/auth"}, time.Hour, nil)
	if h == nil {
		t.Fatal("expected handlers")
	}
}

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	h := testAuthHandlers(&fakeAuthService{})
	e := echo.New()
	h.Mount(e)
	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	for _, key := range []string{"POST /auth/register", "POST /auth/login", "GET /auth/me", "PATCH /auth/me"} {
		if !found[key] {
			t.Fatalf("missing route %s", key)
		}
	}
}

func assertHTTPStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
	}
}

func TestRegister_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := NewRateLimiter(rdb, 5, time.Minute, 1, time.Minute)
	h := &Handlers{
		svc:        &fakeAuthService{},
		jwtSecret:  testJWTSecret,
		cookie:     config.RefreshCookieSettings{Name: "mentorix_refresh", Path: "/auth"},
		refreshTTL: time.Hour,
		limiter:    limiter,
	}
	e := echo.New()
	e.POST("/auth/register", h.Register)

	body := `{"email":"rate@test.com","password":"password123"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assertHTTPStatus(t, rec, http.StatusTooManyRequests)
}

func TestLogin_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	h := &Handlers{
		svc:        &fakeAuthService{},
		jwtSecret:  testJWTSecret,
		cookie:     config.RefreshCookieSettings{Name: "mentorix_refresh", Path: "/auth"},
		refreshTTL: time.Hour,
		limiter:    limiter,
	}
	e := echo.New()
	e.POST("/auth/login", h.Login)

	body := `{"email":"login@test.com","password":"password123"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assertHTTPStatus(t, rec, http.StatusTooManyRequests)
}

func TestRegister_invalidJSON(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestRegister_invalidEmail(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)

	body := `{"email":"not-an-email","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestRegister_shortPassword(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)

	body := `{"email":"user@example.com","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestRegister_emailTaken(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{registerErr: ErrEmailTaken})
	e.POST("/auth/register", h.Register)

	body := `{"email":"user@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusConflict)
}

func TestRegister_success(t *testing.T) {
	userID := uuid.New()
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{
		registerIssued: IssuedAuth{UserID: userID, Email: "user@example.com"},
	})
	e.POST("/auth/register", h.Register)

	body := `{"email":"user@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusCreated)
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AccessToken == "" || resp.TokenType != TokenTypeBearer {
		t.Fatalf("unexpected token response: %+v", resp)
	}
	if resp.UserID != userID.String() {
		t.Fatalf("user_id = %q, want %q", resp.UserID, userID)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Fatal("expected refresh cookie")
	}
}

func TestLogin_invalidJSON(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/login", h.Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestLogin_invalidEmail(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/login", h.Login)

	body := `{"email":"bad","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestLogin_invalidCredentials(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{loginErr: ErrInvalidCredentials})
	e.POST("/auth/login", h.Login)

	body := `{"email":"user@example.com","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestLogin_success(t *testing.T) {
	userID := uuid.New()
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{
		loginIssued: IssuedAuth{UserID: userID, Email: "user@example.com"},
	})
	e.POST("/auth/login", h.Login)

	body := `{"email":"user@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() {
		t.Fatalf("user_id = %q", resp.UserID)
	}
}

func TestRefresh_missingCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestRefresh_invalidToken(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{refreshErr: ErrInvalidRefresh})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "bad-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestRefresh_success(t *testing.T) {
	userID := uuid.New()
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{
		refreshIssued: IssuedAuth{UserID: userID, Email: "user@example.com"},
	})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "valid-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() {
		t.Fatalf("user_id = %q", resp.UserID)
	}
}

func TestLogout_clearsCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header")
	}
	if cookies[0].MaxAge != -1 {
		t.Fatalf("cookie MaxAge = %d, want -1", cookies[0].MaxAge)
	}
}

func TestLogout_revokeError(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{logoutErr: errors.New("db down")})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: h.cookie.Name, Value: "refresh-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Logout(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestRefresh_internalError(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{refreshErr: errors.New("db down")})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: h.cookie.Name, Value: "refresh-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Refresh(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestLogoutAll_missingUserInContext(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/logout-all", h.LogoutAll)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestLogoutAll_success(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	if err := h.LogoutAll(c); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	assertHTTPStatus(t, rec, http.StatusNoContent)
}

func TestMe_missingUserInContext(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.GET("/auth/me", h.Me)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestMe_notFound(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{profileErr: pgx.ErrNoRows})
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	err := h.Me(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound || he.Message != httpx.MsgUserNotFound {
		t.Fatalf("error = %v, want 404 user not found", err)
	}
}

func TestMe_success(t *testing.T) {
	userID := uuid.New()
	createdAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{
		profile: UserProfile{
			Email:     "trainer@test.com",
			CreatedAt: createdAt,
			Roles:     []string{RoleTrainer},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, userID)

	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	assertHTTPStatus(t, rec, http.StatusOK)
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() || resp.Email != "trainer@test.com" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestUpdateMe_missingUserInContext(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"name":"Coach"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.UpdateMe(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestUpdateMe_invalidJSON(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	err := h.UpdateMe(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestUpdateMe_notFound(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{profileErr: pgx.ErrNoRows})
	req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"name":"Coach"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	err := h.UpdateMe(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestUpdateMe_success(t *testing.T) {
	userID := uuid.New()
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{
		profile: UserProfile{
			Email: "trainer@test.com",
			Roles: []string{RoleTrainer},
		},
	})
	req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"name":"Coach"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, userID)

	if err := h.UpdateMe(c); err != nil {
		t.Fatalf("UpdateMe: %v", err)
	}
	assertHTTPStatus(t, rec, http.StatusOK)
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Name != "Coach" {
		t.Fatalf("name = %q, want Coach", resp.Name)
	}
}
