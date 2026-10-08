package app_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
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
