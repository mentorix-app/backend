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
	refreshIssued IssuedAuth
	refreshErr    error
	logoutErr     error
	logoutAllErr  error
	profile       UserProfile
	profileErr    error
	googleIssued  IssuedAuth
	googleErr     error
	appleIssued   IssuedAuth
	appleErr      error
	linkIssued    IssuedAuth
	linkErr       error
	deleteErr     error
	activateErr   error
	activate      activationRecorder

	// Arguments of the last calls, for body-mode assertions.
	googleToken  string
	googleNonce  string
	appleToken   string
	appleName    string
	appleNonce   string
	refreshToken string
	logoutToken  string
	linkUserID   uuid.UUID
	linkCode     string
	deleteCalls  []uuid.UUID
	deleteProofs []AppleProof
}

func (f *fakeAuthService) LinkTelegram(_ context.Context, appUserID uuid.UUID, code string) (IssuedAuth, error) {
	f.linkUserID, f.linkCode = appUserID, code
	if f.linkErr != nil {
		return IssuedAuth{}, f.linkErr
	}
	out := f.linkIssued
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "link-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "link-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	if out.RefreshExpires.IsZero() {
		out.RefreshExpires = time.Now().UTC().Add(24 * time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) SignInWithGoogle(_ context.Context, rawIDToken, nonce string) (IssuedAuth, error) {
	f.googleToken, f.googleNonce = rawIDToken, nonce
	if f.googleErr != nil {
		return IssuedAuth{}, f.googleErr
	}
	out := f.googleIssued
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "google-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "google-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	if out.RefreshExpires.IsZero() {
		out.RefreshExpires = time.Now().UTC().Add(24 * time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) SignInWithApple(_ context.Context, rawIDToken, name, nonce string) (IssuedAuth, error) {
	f.appleToken, f.appleName, f.appleNonce = rawIDToken, name, nonce
	if f.appleErr != nil {
		return IssuedAuth{}, f.appleErr
	}
	out := f.appleIssued
	if out.UserID == uuid.Nil {
		out.UserID = uuid.New()
	}
	if out.AccessToken == "" {
		out.AccessToken = "apple-access-token"
	}
	if out.RefreshToken == "" {
		out.RefreshToken = "apple-refresh-token"
	}
	if out.AccessExpires.IsZero() {
		out.AccessExpires = time.Now().UTC().Add(time.Hour)
	}
	if out.RefreshExpires.IsZero() {
		out.RefreshExpires = time.Now().UTC().Add(24 * time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) Refresh(_ context.Context, plain string) (IssuedAuth, error) {
	f.refreshToken = plain
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
	if out.RefreshExpires.IsZero() {
		out.RefreshExpires = time.Now().UTC().Add(24 * time.Hour)
	}
	return out, nil
}

func (f *fakeAuthService) Logout(_ context.Context, plain string) error {
	f.logoutToken = plain
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
	for _, key := range []string{"POST /auth/google", "POST /auth/apple", "GET /auth/me", "PATCH /auth/me"} {
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

func TestUpdateMe_nameLimit(t *testing.T) {
	tests := []struct {
		name  string
		dname string
		want  int
	}{
		{"name at limit", strings.Repeat("я", 100), http.StatusOK},
		{"name over limit", strings.Repeat("я", 101), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			h := testAuthHandlers(&fakeAuthService{profile: UserProfile{Email: "trainer@test.com"}})
			body, err := json.Marshal(MePatchRequest{Name: tt.dname})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(string(body)))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(ContextUserIDKey, uuid.New())

			err = h.UpdateMe(c)
			if tt.want == http.StatusOK {
				if err != nil {
					t.Fatalf("UpdateMe: %v", err)
				}
				return
			}
			he, ok := err.(*echo.HTTPError)
			if !ok || he.Code != tt.want {
				t.Fatalf("error = %v, want %d", err, tt.want)
			}
		})
	}
}

func TestSignIn_rateLimitCountsForwardedBFFAddress(t *testing.T) {
	const secret = "bff-shared-secret-0123456789-abcdef"

	post := func(e *echo.Echo, forwarded string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(idTokenBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(httpx.HeaderBFFSecret, secret)
		req.Header.Set(httpx.HeaderBFFClientIP, forwarded)
		req.RemoteAddr = "10.0.0.1:5555"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	newEcho := func(t *testing.T) *echo.Echo {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		h := testAuthHandlers(&fakeAuthService{googleErr: ErrInvalidIDToken})
		h.limiter = NewRateLimiter(rdb, 1, time.Minute)
		e := echo.New()
		e.IPExtractor = echo.ExtractIPDirect()
		e.Use(httpx.BFFClientIP(secret, nil))
		e.POST("/auth/google", h.Google)
		return e
	}

	t.Run("different forwarded addresses are counted separately", func(t *testing.T) {
		e := newEcho(t)
		for _, addr := range []string{"198.51.100.1", "198.51.100.2"} {
			if got := post(e, addr); got != http.StatusUnauthorized {
				t.Fatalf("sign-in from %s: status = %d, want %d", addr, got, http.StatusUnauthorized)
			}
		}
	})

	t.Run("the same forwarded address shares one counter", func(t *testing.T) {
		e := newEcho(t)
		if got := post(e, "198.51.100.1"); got != http.StatusUnauthorized {
			t.Fatalf("first sign-in: status = %d, want %d", got, http.StatusUnauthorized)
		}
		if got := post(e, "198.51.100.1"); got != http.StatusTooManyRequests {
			t.Fatalf("second sign-in: status = %d, want %d", got, http.StatusTooManyRequests)
		}
	})
}
