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
	"mentorix-backend/internal/subscription"
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

	socialIssued  IssuedAuth
	socialCreated bool
	socialErr     error
	socialCalls   []socialLoginCall
	addRoleErr    error
	addRoleCalls  []addRoleCall
}

type socialLoginCall struct {
	provider, idToken, name string
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

func (f *fakeAuthService) SocialLogin(_ context.Context, provider, idToken, name string) (IssuedAuth, bool, error) {
	f.socialCalls = append(f.socialCalls, socialLoginCall{provider: provider, idToken: idToken, name: name})
	if f.socialErr != nil {
		return IssuedAuth{}, false, f.socialErr
	}
	out := f.socialIssued
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
	return out, f.socialCreated, nil
}

func (f *fakeAuthService) AddRole(_ context.Context, userID uuid.UUID, role string) error {
	f.addRoleCalls = append(f.addRoleCalls, addRoleCall{userID: userID, role: role})
	return f.addRoleErr
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

type fakeSubs struct {
	sub *subscription.Subscription
	err error
}

func (f *fakeSubs) ForUser(context.Context, uuid.UUID) (*subscription.Subscription, error) {
	return f.sub, f.err
}

func TestMe_withSubscription(t *testing.T) {
	userID := uuid.New()
	src := subscription.SourceAdmin
	sub := &subscription.Subscription{
		Plan:   subscription.PlanElite,
		Source: &src,
		Limits: subscription.Limits{},
		Usage:  subscription.Usage{},
		Permissions: subscription.Permissions{
			CanCreateExercise: true,
			CanEditExercises:  true,
			CanCreateProgram:  true,
			CanEditPrograms:   true,
			CanCreateInvite:   true,
			CanManageClients:  true,
		},
	}
	h := NewHandlers(nil, testJWTSecret, config.RefreshCookieSettings{Name: "refresh", Path: "/auth"}, time.Hour, nil,
		WithSubscriptions(&fakeSubs{sub: sub}),
	)
	h.svc = &fakeAuthService{
		profile: UserProfile{Email: "trainer@test.com", Roles: []string{RoleTrainer}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, userID)

	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != subscription.PlanElite {
		t.Fatalf("subscription = %+v, want elite", resp.Subscription)
	}
}

func TestMe_subscriptionError(t *testing.T) {
	h := NewHandlers(nil, testJWTSecret, config.RefreshCookieSettings{Name: "refresh", Path: "/auth"}, time.Hour, nil,
		WithSubscriptions(&fakeSubs{err: errors.New("db down")}),
	)
	h.svc = &fakeAuthService{
		profile: UserProfile{Email: "trainer@test.com", Roles: []string{RoleTrainer}},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	err := h.Me(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestMe_withNilSubscription(t *testing.T) {
	h := NewHandlers(nil, testJWTSecret, config.RefreshCookieSettings{Name: "refresh", Path: "/auth"}, time.Hour, nil,
		WithSubscriptions(&fakeSubs{sub: nil}),
	)
	h.svc = &fakeAuthService{
		profile: UserProfile{Email: "admin@test.com", Roles: []string{RoleAdmin}},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Subscription != nil {
		t.Fatalf("subscription = %+v, want null", resp.Subscription)
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

// recordingAuthService records the refresh tokens passed to Refresh and Logout.
type recordingAuthService struct {
	fakeAuthService
	refreshed []string
	loggedOut []string
	// logoutErrs fails Logout for the given token.
	logoutErrs map[string]error
}

func (r *recordingAuthService) Refresh(ctx context.Context, plain string) (IssuedAuth, error) {
	r.refreshed = append(r.refreshed, plain)
	return r.fakeAuthService.Refresh(ctx, plain)
}

func (r *recordingAuthService) Logout(ctx context.Context, plain string) error {
	r.loggedOut = append(r.loggedOut, plain)
	if err := r.logoutErrs[plain]; err != nil {
		return err
	}
	return r.fakeAuthService.Logout(ctx, plain)
}

func postAuthJSON(e *echo.Echo, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func decodeTokenResponse(t *testing.T, rec *httptest.ResponseRecorder) TokenResponse {
	t.Helper()
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestRegister_bodyDelivery(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{registerIssued: IssuedAuth{RefreshToken: "app-refresh"}})
	e.POST("/auth/register", h.Register)

	rec := postAuthJSON(e, "/auth/register", `{"email":"user@example.com","password":"password123","token_delivery":"body"}`)

	assertHTTPStatus(t, rec, http.StatusCreated)
	if got := decodeTokenResponse(t, rec).RefreshToken; got != "app-refresh" {
		t.Fatalf("refresh_token = %q, want app-refresh", got)
	}
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Fatalf("got %d cookies, want none", n)
	}
}

func TestRegister_cookieDeliveryByDefault(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)

	rec := postAuthJSON(e, "/auth/register", `{"email":"user@example.com","password":"password123"}`)

	assertHTTPStatus(t, rec, http.StatusCreated)
	if got := decodeTokenResponse(t, rec).RefreshToken; got != "" {
		t.Fatalf("refresh_token = %q, want empty", got)
	}
	if strings.Contains(rec.Body.String(), "refresh_token") {
		t.Fatalf("body must not contain refresh_token: %s", rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].Value == "" {
		t.Fatalf("cookies = %v, want one refresh cookie", cookies)
	}
}

func TestRegister_invalidTokenDelivery(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)

	rec := postAuthJSON(e, "/auth/register", `{"email":"user@example.com","password":"password123","token_delivery":"header"}`)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestLogin_bodyDelivery(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{loginIssued: IssuedAuth{RefreshToken: "app-refresh"}})
	e.POST("/auth/login", h.Login)

	rec := postAuthJSON(e, "/auth/login", `{"email":"user@example.com","password":"password123","token_delivery":"body"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if got := decodeTokenResponse(t, rec).RefreshToken; got != "app-refresh" {
		t.Fatalf("refresh_token = %q, want app-refresh", got)
	}
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Fatalf("got %d cookies, want none", n)
	}
}

func TestLogin_cookieDeliveryExplicitAndEmpty(t *testing.T) {
	for _, field := range []string{`,"token_delivery":"cookie"`, `,"token_delivery":""`, ``} {
		e := echo.New()
		h := testAuthHandlers(&fakeAuthService{})
		e.POST("/auth/login", h.Login)

		rec := postAuthJSON(e, "/auth/login", `{"email":"user@example.com","password":"password123"`+field+`}`)

		assertHTTPStatus(t, rec, http.StatusOK)
		if strings.Contains(rec.Body.String(), "refresh_token") {
			t.Fatalf("%q: body must not contain refresh_token: %s", field, rec.Body.String())
		}
		if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].Value == "" {
			t.Fatalf("%q: cookies = %v, want one refresh cookie", field, cookies)
		}
	}
}

func TestLogin_invalidTokenDelivery(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/login", h.Login)

	rec := postAuthJSON(e, "/auth/login", `{"email":"user@example.com","password":"password123","token_delivery":"BODY"}`)

	assertHTTPStatus(t, rec, http.StatusBadRequest)
}

func TestRefresh_bodyToken(t *testing.T) {
	svc := &recordingAuthService{fakeAuthService: fakeAuthService{refreshIssued: IssuedAuth{RefreshToken: "rotated"}}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	rec := postAuthJSON(e, "/auth/refresh", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if got := decodeTokenResponse(t, rec).RefreshToken; got != "rotated" {
		t.Fatalf("refresh_token = %q, want rotated", got)
	}
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Fatalf("got %d cookies, want none", n)
	}
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "app-refresh" {
		t.Fatalf("refreshed = %v, want [app-refresh]", svc.refreshed)
	}
}

func TestRefresh_bodyTokenWithoutContentType(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"app-refresh"}`))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "app-refresh" {
		t.Fatalf("refreshed = %v, want [app-refresh]", svc.refreshed)
	}
}

// A refresh that arrived by cookie must never expose the new refresh token to page scripts.
func TestRefresh_cookieOnlyNeverReturnsTokenInJSON(t *testing.T) {
	svc := &recordingAuthService{fakeAuthService: fakeAuthService{refreshIssued: IssuedAuth{RefreshToken: "rotated"}}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	for name, body := range map[string]string{
		"no body":      "",
		"empty object": "{}",
		"empty token":  `{"refresh_token":""}`,
		"null":         "null",
		"array":        "[]",
		"number token": `{"refresh_token":123}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assertHTTPStatus(t, rec, http.StatusOK)
			if strings.Contains(rec.Body.String(), "refresh_token") || strings.Contains(rec.Body.String(), "rotated") {
				t.Fatalf("body leaks refresh token: %s", rec.Body.String())
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Value != "rotated" || !cookies[0].HttpOnly {
				t.Fatalf("cookies = %v, want one HttpOnly cookie with the rotated token", cookies)
			}
		})
	}
	for _, got := range svc.refreshed {
		if got != "cookie-refresh" {
			t.Fatalf("refreshed = %v, want only cookie-refresh", svc.refreshed)
		}
	}
}

func TestRefresh_bodyTokenWinsOverCookie(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"body-refresh"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "body-refresh" {
		t.Fatalf("refreshed = %v, want [body-refresh]", svc.refreshed)
	}
	if decodeTokenResponse(t, rec).RefreshToken == "" {
		t.Fatal("expected refresh_token in JSON")
	}
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Fatalf("got %d cookies, want none", n)
	}
}

func TestRefresh_invalidBodyTokenKeepsCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{refreshErr: ErrInvalidRefresh})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"stale"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_invalidCookieTokenClearsCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{refreshErr: ErrInvalidRefresh})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "bad-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestRefresh_malformedBodyFallsBackToCookie(t *testing.T) {
	svc := &recordingAuthService{fakeAuthService: fakeAuthService{refreshIssued: IssuedAuth{RefreshToken: "rotated"}}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "cookie-refresh" {
		t.Fatalf("refreshed = %v, want [cookie-refresh]", svc.refreshed)
	}
	if strings.Contains(rec.Body.String(), "refresh_token") {
		t.Fatalf("body leaks refresh token: %s", rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].Value != "rotated" {
		t.Fatalf("cookies = %v, want one rotated cookie", cookies)
	}
}

func TestRefresh_malformedBodyWithoutCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/refresh", h.Refresh)

	rec := postAuthJSON(e, "/auth/refresh", "not-json")

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
}

func TestRefresh_whitespaceBodyTokenFallsBackToCookie(t *testing.T) {
	svc := &recordingAuthService{fakeAuthService: fakeAuthService{refreshIssued: IssuedAuth{RefreshToken: "rotated"}}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"   "}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "cookie-refresh" {
		t.Fatalf("refreshed = %v, want [cookie-refresh]", svc.refreshed)
	}
	if strings.Contains(rec.Body.String(), "refresh_token") {
		t.Fatalf("body leaks refresh token: %s", rec.Body.String())
	}
}

func TestLogout_bodyTokenRevokesAndKeepsCookie(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	rec := postAuthJSON(e, "/auth/logout", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 1 || svc.loggedOut[0] != "app-refresh" {
		t.Fatalf("loggedOut = %v, want [app-refresh]", svc.loggedOut)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestLogout_bodyAndCookieRevokesBothAndClearsCookie(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader(`{"refresh_token":"body-refresh"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 2 || svc.loggedOut[0] != "body-refresh" || svc.loggedOut[1] != "cookie-refresh" {
		t.Fatalf("loggedOut = %v, want [body-refresh cookie-refresh]", svc.loggedOut)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestLogout_cookieTokenRevokedAndCleared(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 1 || svc.loggedOut[0] != "cookie-refresh" {
		t.Fatalf("loggedOut = %v, want [cookie-refresh]", svc.loggedOut)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestLogout_malformedBodyUsesCookie(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 1 || svc.loggedOut[0] != "cookie-refresh" {
		t.Fatalf("loggedOut = %v, want [cookie-refresh]", svc.loggedOut)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestLogout_malformedBodyWithoutCookie(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	rec := postAuthJSON(e, "/auth/logout", "not-json")

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 0 {
		t.Fatalf("loggedOut = %v, want none", svc.loggedOut)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestLogout_bodyTokenRevokeError(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{logoutErr: errors.New("db down")})
	e.POST("/auth/logout", h.Logout)

	rec := postAuthJSON(e, "/auth/logout", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestAuthJSON_noStore(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)
	e.POST("/auth/login", h.Login)
	e.POST("/auth/refresh", h.Refresh)

	cases := map[string][2]string{
		"register cookie": {"/auth/register", `{"email":"user@example.com","password":"password123"}`},
		"register body":   {"/auth/register", `{"email":"user@example.com","password":"password123","token_delivery":"body"}`},
		"login cookie":    {"/auth/login", `{"email":"user@example.com","password":"password123"}`},
		"login body":      {"/auth/login", `{"email":"user@example.com","password":"password123","token_delivery":"body"}`},
		"refresh body":    {"/auth/refresh", `{"refresh_token":"app-refresh"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := postAuthJSON(e, tc[0], tc[1])
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store (status %d)", got, rec.Code)
			}
		})
	}
	t.Run("refresh cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store (status %d)", got, rec.Code)
		}
	})
}

func TestRefreshRequest_jsonKey(t *testing.T) {
	raw, err := json.Marshal(RefreshRequest{RefreshToken: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"refresh_token":"x"}` {
		t.Fatalf("json = %s", raw)
	}
}

func logoutWithBodyAndCookie(e *echo.Echo, bodyToken, cookieToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader(`{"refresh_token":"`+bodyToken+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: cookieToken})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLogout_bodyRevokeFailsCookieStillRevoked(t *testing.T) {
	svc := &recordingAuthService{logoutErrs: map[string]error{"body-refresh": errors.New("db down")}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	rec := logoutWithBodyAndCookie(e, "body-refresh", "cookie-refresh")

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
	if len(svc.loggedOut) != 2 || svc.loggedOut[0] != "body-refresh" || svc.loggedOut[1] != "cookie-refresh" {
		t.Fatalf("loggedOut = %v, want [body-refresh cookie-refresh]", svc.loggedOut)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestLogout_cookieRevokeFailsAfterBodyRevoked(t *testing.T) {
	svc := &recordingAuthService{logoutErrs: map[string]error{"cookie-refresh": errors.New("db down")}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	rec := logoutWithBodyAndCookie(e, "body-refresh", "cookie-refresh")

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
	if len(svc.loggedOut) != 2 || svc.loggedOut[0] != "body-refresh" || svc.loggedOut[1] != "cookie-refresh" {
		t.Fatalf("loggedOut = %v, want [body-refresh cookie-refresh]", svc.loggedOut)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestLogout_sameBodyAndCookieTokenRevokedOnce(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	rec := logoutWithBodyAndCookie(e, "same-refresh", "same-refresh")

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 1 || svc.loggedOut[0] != "same-refresh" {
		t.Fatalf("loggedOut = %v, want [same-refresh]", svc.loggedOut)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestRefresh_invalidCookieTokenWithBlankBodyTokenClearsCookie(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{refreshErr: ErrInvalidRefresh})
	e.POST("/auth/refresh", h.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"  "}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "bad-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %v, want one clearing cookie", cookies)
	}
}

func TestLogout_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &recordingAuthService{}
	h := testAuthHandlers(svc)
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := echo.New()
	e.POST("/auth/logout", h.Logout)

	first := logoutWithBodyAndCookie(e, "body-refresh", "cookie-refresh")
	assertHTTPStatus(t, first, http.StatusNoContent)
	second := logoutWithBodyAndCookie(e, "body-refresh-2", "cookie-refresh-2")

	assertHTTPStatus(t, second, http.StatusTooManyRequests)
	if len(svc.loggedOut) != 2 {
		t.Fatalf("loggedOut = %v, want only the first request's tokens", svc.loggedOut)
	}
	if got := second.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestLogout_rateLimitBackendError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	mr.Close()
	e := echo.New()
	e.POST("/auth/logout", h.Logout)

	rec := postAuthJSON(e, "/auth/logout", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestLogout_nilLimiterAllowsRequest(t *testing.T) {
	h := testAuthHandlers(&fakeAuthService{})
	if h.limiter != nil {
		t.Fatal("test handlers must have a nil limiter")
	}
	e := echo.New()
	e.POST("/auth/logout", h.Logout)

	rec := postAuthJSON(e, "/auth/logout", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusNoContent)
}

func TestLogout_cookieOnlyRevokeFails(t *testing.T) {
	svc := &recordingAuthService{logoutErrs: map[string]error{"cookie-refresh": errors.New("db down")}}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-refresh"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_paddedBodyTokenIsTrimmed(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := testAuthHandlers(svc)
	e.POST("/auth/refresh", h.Refresh)

	rec := postAuthJSON(e, "/auth/refresh", `{"refresh_token":" abc "}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "abc" {
		t.Fatalf("refreshed = %v, want [abc]", svc.refreshed)
	}
}

const (
	originAllowed    = "https://app.example.com"
	originDisallowed = "https://evil.example.net"
)

func originTestHandlers(svc credentialService, allowed ...string) *Handlers {
	h := testAuthHandlers(svc)
	WithAllowedOrigins(allowed)(h)
	return h
}

func originRequest(path, origin, body string, cookie string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if origin != "" {
		req.Header.Set(echo.HeaderOrigin, origin)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: cookie})
	}
	return req
}

type originCase struct {
	name    string
	allowed []string
	origin  string
	want    int
}

// originCases apply to every endpoint that acts on the refresh cookie.
var originCases = []originCase{
	{"disallowed origin", []string{originAllowed}, originDisallowed, http.StatusForbidden},
	{"allowed origin", []string{originAllowed}, originAllowed, http.StatusOK},
	{"allowed origin with trailing slash in config", []string{originAllowed + "/"}, originAllowed, http.StatusOK},
	{"allowed origin differing in case", []string{originAllowed}, "HTTPS://App.Example.com", http.StatusOK},
	{"no origin header", []string{originAllowed}, "", http.StatusOK},
	{"empty allowed list", nil, originDisallowed, http.StatusOK},
	{"wildcard in list", []string{originAllowed, "*"}, originDisallowed, http.StatusOK},
	{"null origin", []string{originAllowed}, "null", http.StatusForbidden},
	{"scheme mismatch", []string{"https://cabinet.example.com"}, "http://cabinet.example.com", http.StatusForbidden},
	{"port mismatch", []string{"https://cabinet.example.com"}, "https://cabinet.example.com:8443", http.StatusForbidden},
	{"second configured entry matches", []string{"https://other.example.com", originAllowed}, originAllowed, http.StatusOK},
	{"whitespace around configured entry", []string{"  " + originAllowed + "  "}, originAllowed, http.StatusOK},
	{"suffix lookalike", []string{"https://cabinet.example.com"}, "https://cabinet.example.com.evil.test", http.StatusForbidden},
}

func TestRefresh_cookieOriginCheck(t *testing.T) {
	for _, tc := range originCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &recordingAuthService{}
			e := echo.New()
			h := originTestHandlers(svc, tc.allowed...)
			e.POST("/auth/refresh", h.Refresh)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, originRequest("/auth/refresh", tc.origin, "", "cookie-refresh"))

			assertHTTPStatus(t, rec, tc.want)
			if tc.want == http.StatusForbidden {
				if len(svc.refreshed) != 0 {
					t.Fatalf("refreshed = %v, want none", svc.refreshed)
				}
				if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
					t.Fatalf("Set-Cookie = %v, want none", got)
				}
				if !strings.Contains(rec.Body.String(), "origin not allowed") {
					t.Fatalf("body = %s, want origin not allowed", rec.Body.String())
				}
			}
		})
	}
}

func TestLogout_cookieOriginCheck(t *testing.T) {
	for _, tc := range originCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &recordingAuthService{}
			e := echo.New()
			h := originTestHandlers(svc, tc.allowed...)
			e.POST("/auth/logout", h.Logout)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, originRequest("/auth/logout", tc.origin, "", "cookie-refresh"))

			want := tc.want
			if want == http.StatusOK {
				want = http.StatusNoContent
			}
			assertHTTPStatus(t, rec, want)
			if want == http.StatusForbidden {
				if len(svc.loggedOut) != 0 {
					t.Fatalf("loggedOut = %v, want none", svc.loggedOut)
				}
				if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
					t.Fatalf("Set-Cookie = %v, want none", got)
				}
				if !strings.Contains(rec.Body.String(), "origin not allowed") {
					t.Fatalf("body = %s, want origin not allowed", rec.Body.String())
				}
			}
		})
	}
}

// Rejected cross-site requests must not spend the victim IP's rate-limit budget.
func TestCookieRequests_disallowedOriginDoesNotConsumeRateLimit(t *testing.T) {
	endpoints := []struct {
		name    string
		path    string
		success int
	}{
		{"refresh", "/auth/refresh", http.StatusOK},
		{"logout", "/auth/logout", http.StatusNoContent},
	}
	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			h := originTestHandlers(&recordingAuthService{}, originAllowed)
			h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
			e := echo.New()
			e.POST("/auth/refresh", h.Refresh)
			e.POST("/auth/logout", h.Logout)

			for i := 0; i < 5; i++ {
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, originRequest(ep.path, originDisallowed, "", "cookie-refresh"))
				assertHTTPStatus(t, rec, http.StatusForbidden)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, originRequest(ep.path, originAllowed, "", "cookie-refresh"))
			assertHTTPStatus(t, rec, ep.success)
		})
	}
}

func TestCookieRequests_allowedOriginStillRateLimited(t *testing.T) {
	for _, path := range []string{"/auth/refresh", "/auth/logout"} {
		t.Run(path, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			h := originTestHandlers(&recordingAuthService{}, originAllowed)
			h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
			e := echo.New()
			e.POST("/auth/refresh", h.Refresh)
			e.POST("/auth/logout", h.Logout)

			first := httptest.NewRecorder()
			e.ServeHTTP(first, originRequest(path, originAllowed, "", "cookie-refresh"))
			if first.Code >= 300 {
				t.Fatalf("first status = %d", first.Code)
			}
			second := httptest.NewRecorder()
			e.ServeHTTP(second, originRequest(path, originAllowed, "", "cookie-refresh"))
			assertHTTPStatus(t, second, http.StatusTooManyRequests)
		})
	}
}

func TestOriginCheckDisabled(t *testing.T) {
	cases := []struct {
		name    string
		origins []string
		want    bool
	}{
		{"nil list", nil, true},
		{"empty list", []string{}, true},
		{"wildcard only", []string{"*"}, true},
		{"wildcard among entries", []string{originAllowed, "*"}, true},
		{"padded wildcard", []string{" * "}, true},
		{"explicit origins", []string{originAllowed}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OriginCheckDisabled(tc.origins); got != tc.want {
				t.Fatalf("OriginCheckDisabled(%v) = %v, want %v", tc.origins, got, tc.want)
			}
		})
	}
}

func TestRefresh_bodyTokenSkipsOriginCheck(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := originTestHandlers(svc, originAllowed)
	e.POST("/auth/refresh", h.Refresh)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, originRequest("/auth/refresh", originDisallowed, `{"refresh_token":"app-refresh"}`, ""))

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "app-refresh" {
		t.Fatalf("refreshed = %v, want [app-refresh]", svc.refreshed)
	}
}

func TestLogout_bodyAndCookieDisallowedOriginForbidden(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := originTestHandlers(svc, originAllowed)
	e.POST("/auth/logout", h.Logout)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, originRequest("/auth/logout", originDisallowed, `{"refresh_token":"body-refresh"}`, "cookie-refresh"))

	assertHTTPStatus(t, rec, http.StatusForbidden)
	if len(svc.loggedOut) != 0 {
		t.Fatalf("loggedOut = %v, want none", svc.loggedOut)
	}
}

func TestLogout_bodyOnlyDisallowedOriginAllowed(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := originTestHandlers(svc, originAllowed)
	e.POST("/auth/logout", h.Logout)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, originRequest("/auth/logout", originDisallowed, `{"refresh_token":"app-refresh"}`, ""))

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.loggedOut) != 1 || svc.loggedOut[0] != "app-refresh" {
		t.Fatalf("loggedOut = %v, want [app-refresh]", svc.loggedOut)
	}
}

func TestLogout_cookieAllowedOriginAndNoOriginPass(t *testing.T) {
	for _, origin := range []string{originAllowed, ""} {
		svc := &recordingAuthService{}
		e := echo.New()
		h := originTestHandlers(svc, originAllowed)
		e.POST("/auth/logout", h.Logout)

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, originRequest("/auth/logout", origin, "", "cookie-refresh"))

		assertHTTPStatus(t, rec, http.StatusNoContent)
		if len(svc.loggedOut) != 1 {
			t.Fatalf("origin %q: loggedOut = %v, want one", origin, svc.loggedOut)
		}
	}
}

func TestRefresh_bodyAndCookieDisallowedOriginSkipsOriginCheck(t *testing.T) {
	svc := &recordingAuthService{}
	e := echo.New()
	h := originTestHandlers(svc, originAllowed)
	e.POST("/auth/refresh", h.Refresh)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, originRequest("/auth/refresh", originDisallowed, `{"refresh_token":"body-refresh"}`, "cookie-refresh"))

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.refreshed) != 1 || svc.refreshed[0] != "body-refresh" {
		t.Fatalf("refreshed = %v, want [body-refresh]", svc.refreshed)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_rateLimitBackendError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	mr.Close()
	e := echo.New()
	e.POST("/auth/refresh", h.Refresh)

	rec := postAuthJSON(e, "/auth/refresh", `{"refresh_token":"app-refresh"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestRefresh_noTokenWhileRateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := echo.New()
	e.POST("/auth/refresh", h.Refresh)

	first := postAuthJSON(e, "/auth/refresh", "")
	assertHTTPStatus(t, first, http.StatusUnauthorized)
	second := postAuthJSON(e, "/auth/refresh", "")

	assertHTTPStatus(t, second, http.StatusTooManyRequests)
}
