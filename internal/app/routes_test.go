package app_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/apicheck"
	"mentorix-backend/internal/app"
	"mentorix-backend/internal/config"
)

func routeSet(e *echo.Echo) []string {
	out := make([]string, 0, len(e.Routes()))
	for _, r := range e.Routes() {
		out = append(out, fmt.Sprintf("%s %s", r.Method, r.Path))
	}
	sort.Strings(out)
	return out
}

func diffSets(a, b []string) (onlyA, onlyB []string) {
	inA := map[string]bool{}
	inB := map[string]bool{}
	for _, s := range a {
		inA[s] = true
	}
	for _, s := range b {
		inB[s] = true
	}
	for _, s := range a {
		if !inB[s] {
			onlyA = append(onlyA, s)
		}
	}
	for _, s := range b {
		if !inA[s] {
			onlyB = append(onlyB, s)
		}
	}
	return onlyA, onlyB
}

// lazyPool returns a pool that never connects: pgxpool dials on first use.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func productionLikeConfig() config.Config {
	return config.Config{
		JWTSecret:            "route-set-test-jwt-secret-min-32-chars",
		AccessTTLMinutes:     15,
		RefreshTTLDays:       30,
		TrainerInviteTTLDays: 7,
		BotToken:             "123:token",
		TelegramBotUsername:  "mentorix_bot",
		AuthLoginRateMax:     20,
		AuthRegisterRateMax:  10,
	}
}

// The server (cmd/api) and the OpenAPI contract test must serve the same
// routes. Everything cmd/api mounts besides the Telegram webhook comes from
// app.Mount, so this fails when Mount starts to depend on config or on a real
// pool in a way the contract mount does not reproduce.
func TestMount_serverRouteSetEqualsContractRouteSet(t *testing.T) {
	server := echo.New()
	app.Mount(server, app.Deps{Config: productionLikeConfig(), Pool: lazyPool(t), Logger: slog.Default()})

	contract := echo.New()
	apicheck.MountRoutes(contract, nil)

	onlyServer, onlyContract := diffSets(routeSet(server), routeSet(contract))
	if len(onlyServer) > 0 || len(onlyContract) > 0 {
		t.Fatalf("route sets differ\nonly in server: %v\nonly in contract mount: %v", onlyServer, onlyContract)
	}
	if n := len(routeSet(server)); n < 70 {
		t.Fatalf("only %d routes mounted; the comparison is vacuous", n)
	}
}

func TestMount_withoutPoolServesOnlyHealthAndWarns(t *testing.T) {
	var logs bytes.Buffer
	e := echo.New()
	res := app.Mount(e, app.Deps{
		Config: productionLikeConfig(),
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})

	got := strings.Join(routeSet(e), ",")
	if want := "GET /health,GET /health/ready"; got != want {
		t.Fatalf("routes = %s, want %s", got, want)
	}
	if res.Notifier != nil || res.TrainerClient != nil {
		t.Fatalf("result = %+v, want zero value without a pool", res)
	}
	if !strings.Contains(logs.String(), "DATABASE_URL not set; auth, exercise, and program routes are disabled") {
		t.Fatalf("missing warning in logs: %s", logs.String())
	}
}

func TestMount_notifierOnlyWithBotToken(t *testing.T) {
	pool := lazyPool(t)

	withToken := app.Mount(echo.New(), app.Deps{Config: productionLikeConfig(), Pool: pool, Logger: slog.Default()})
	if withToken.Notifier == nil || withToken.TrainerClient == nil {
		t.Fatalf("result with token = %+v, want notifier and trainer client", withToken)
	}
	t.Cleanup(func() { _ = withToken.Notifier.Close(context.Background()) })

	cfg := productionLikeConfig()
	cfg.BotToken = ""
	noToken := app.Mount(echo.New(), app.Deps{Config: cfg, Pool: pool, Logger: slog.Default()})
	if noToken.Notifier != nil {
		t.Fatal("notifier created without a bot token")
	}
	if noToken.TrainerClient == nil {
		t.Fatal("trainer client missing without a bot token")
	}
}

// DELETE /auth/me must be mounted and sit behind the token: without it the
// endpoint is either missing from the server or open to anyone.
func TestMount_servesDeleteMeBehindTheToken(t *testing.T) {
	e := echo.New()
	res := app.Mount(e, app.Deps{Config: productionLikeConfig(), Pool: lazyPool(t), Logger: slog.Default()})
	if res.Notifier != nil {
		t.Cleanup(func() { _ = res.Notifier.Close(context.Background()) })
	}

	if !slices.Contains(routeSet(e), "DELETE /auth/me") {
		t.Fatalf("DELETE /auth/me is not mounted; routes: %v", routeSet(e))
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/auth/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body.String())
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"code":"missing_authorization","message":"missing authorization"}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

// A deploy that drops the setting on its way to the handler answers 503 on the
// Apple return address, which only a real Android sign-in would notice.
func TestMount_passesAppleAndroidPackageToCallback(t *testing.T) {
	cfg := productionLikeConfig()
	cfg.AppleAndroidPackage = "com.mentorix.mobile"
	e := echo.New()
	res := app.Mount(e, app.Deps{Config: cfg, Pool: lazyPool(t), Logger: slog.Default()})
	if res.Notifier != nil {
		t.Cleanup(func() { _ = res.Notifier.Close(context.Background()) })
	}

	form := url.Values{"state": {"s"}, "code": {"c"}}
	req := httptest.NewRequest(http.MethodPost, "/auth/apple/callback", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusTemporaryRedirect, rec.Body.String())
	}
	if loc := rec.Header().Get(echo.HeaderLocation); !strings.Contains(loc, "package=com.mentorix.mobile") {
		t.Fatalf("Location = %q, want package=com.mentorix.mobile", loc)
	}
}

// The handler is installed by Mount itself, before the early return for a missing
// pool, so cmd/api, the contract mount and the HTTP tests all answer errors the
// same way.
func TestMount_installsTheErrorHandler(t *testing.T) {
	tests := []struct {
		name string
		deps func(t *testing.T) app.Deps
	}{
		{"without a pool", func(*testing.T) app.Deps {
			return app.Deps{Config: productionLikeConfig(), Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))}
		}},
		{"with a pool", func(t *testing.T) app.Deps {
			return app.Deps{Config: productionLikeConfig(), Pool: lazyPool(t), Logger: slog.Default()}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			res := app.Mount(e, tt.deps(t))
			if res.Notifier != nil {
				t.Cleanup(func() { _ = res.Notifier.Close(context.Background()) })
			}

			unknown := httptest.NewRecorder()
			e.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
			wrongMethod := httptest.NewRecorder()
			e.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/health", nil))

			if unknown.Code != http.StatusNotFound {
				t.Errorf("unknown path status = %d, want 404", unknown.Code)
			}
			if got, want := strings.TrimSpace(unknown.Body.String()), `{"code":"not_found","message":"Not Found"}`; got != want {
				t.Errorf("unknown path body = %s, want %s", got, want)
			}
			if wrongMethod.Code != http.StatusMethodNotAllowed {
				t.Errorf("wrong method status = %d, want 405", wrongMethod.Code)
			}
			if got, want := strings.TrimSpace(wrongMethod.Body.String()), `{"code":"method_not_allowed","message":"Method Not Allowed"}`; got != want {
				t.Errorf("wrong method body = %s, want %s", got, want)
			}
		})
	}
}

// The three app endpoints for a client's program, workout marks and stats must be
// served and sit behind the token, one trainer per path.
func TestMount_servesTheClientAppRoutesBehindTheToken(t *testing.T) {
	e := echo.New()
	res := app.Mount(e, app.Deps{Config: productionLikeConfig(), Pool: lazyPool(t), Logger: slog.Default()})
	if res.Notifier != nil {
		t.Cleanup(func() { _ = res.Notifier.Close(context.Background()) })
	}
	trainerID := "11111111-1111-1111-1111-111111111111"

	for _, rt := range []struct{ method, path, route string }{
		{http.MethodGet, "/client/trainers/" + trainerID + "/program", "GET /client/trainers/:trainer_id/program"},
		{http.MethodPost, "/client/trainers/" + trainerID + "/completions", "POST /client/trainers/:trainer_id/completions"},
		{http.MethodGet, "/client/trainers/" + trainerID + "/analytics", "GET /client/trainers/:trainer_id/analytics"},
	} {
		if !slices.Contains(routeSet(e), rt.route) {
			t.Errorf("%s is not mounted; routes: %v", rt.route, routeSet(e))
			continue
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status = %d, want 401; body = %s", rt.route, rec.Code, rec.Body.String())
		}
	}

	// The signed-link page keeps its own route and needs no token.
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/client/analytics", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET /client/analytics: status = %d, want 400 from the missing link parameters; body = %s", rec.Code, rec.Body.String())
	}
}
