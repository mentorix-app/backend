//go:build integration

package storetest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/trainerclient"
)

type appInviteEnv struct {
	pool  *pgxpool.Pool
	auth  *auth.Store
	svc   *trainerclient.Service
	store *trainerclient.Store
}

func newAppInviteEnv(t *testing.T) appInviteEnv {
	t.Helper()
	pool := NewPool(t)
	svc := trainerclient.NewService(pool, program.NewService(pool), trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)
	return appInviteEnv{pool: pool, auth: auth.NewStore(pool), svc: svc, store: trainerclient.NewStore(pool)}
}

func (e appInviteEnv) trainer(t *testing.T, email, name string) uuid.UUID {
	t.Helper()
	id, err := e.auth.RegisterTrainerEmailPassword(context.Background(), email, "hash", name)
	if err != nil {
		t.Fatalf("register trainer %s: %v", email, err)
	}
	return id
}

func (e appInviteEnv) invite(t *testing.T, trainerUserID uuid.UUID) string {
	t.Helper()
	invite, err := e.svc.CreateInvite(context.Background(), trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	return strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
}

func (e appInviteEnv) roles(t *testing.T, userID uuid.UUID) []string {
	t.Helper()
	roles, err := sqlc.New(e.pool).ListUserRoles(context.Background(), pgconv.ToPGUUID(userID))
	if err != nil {
		t.Fatalf("ListUserRoles: %v", err)
	}
	return roles
}

// withoutRoles drops the client role so a refused accept can be shown to leave none behind.
func (e appInviteEnv) withoutRoles(t *testing.T, userID uuid.UUID) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `DELETE FROM mentorix.user_roles WHERE user_id = $1 AND role = 'client'`, userID); err != nil {
		t.Fatalf("drop client role: %v", err)
	}
}

func (e appInviteEnv) assertNoClientRole(t *testing.T, userID uuid.UUID) {
	t.Helper()
	if contains(e.roles(t, userID), auth.RoleClient) {
		t.Fatalf("roles = %v, a refused accept must not leave the client role", e.roles(t, userID))
	}
}

func (e appInviteEnv) assertUnconsumed(t *testing.T, token string) {
	t.Helper()
	if n := countRows(t, e.pool, `SELECT count(*) FROM mentorix.trainer_invites WHERE token = $1 AND consumed_at IS NULL`, token); n != 1 {
		t.Fatalf("unconsumed invite rows = %d, want 1", n)
	}
}

func (e appInviteEnv) linkCount(t *testing.T, clientUserID uuid.UUID) int {
	t.Helper()
	return countRows(t, e.pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE client_user_id = $1`, clientUserID)
}

func TestTrainerInvite_acceptAsUser_linksConsumesAndGrantsClientRole(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerUserID := env.trainer(t, "app-accept-trainer@test.com", "Trainer Anna")
	token := env.invite(t, trainerUserID)
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-accept-1", "app-accept@test.com")
	if _, err := env.pool.Exec(ctx, `DELETE FROM mentorix.user_roles WHERE user_id = $1`, appUser); err != nil {
		t.Fatalf("drop roles: %v", err)
	}

	result, err := env.store.AcceptInviteAsUser(ctx, token, appUser)
	if err != nil {
		t.Fatalf("AcceptInviteAsUser: %v", err)
	}
	if result.AlreadyLinked || result.UserID != appUser || result.TrainerDisplayName != "Trainer Anna" {
		t.Fatalf("result = %+v", result)
	}
	if env.linkCount(t, appUser) != 1 {
		t.Fatalf("trainer_clients rows = %d, want 1", env.linkCount(t, appUser))
	}
	var consumedBy uuid.UUID
	if err := env.pool.QueryRow(ctx, `SELECT consumed_by FROM mentorix.trainer_invites WHERE token = $1`, token).Scan(&consumedBy); err != nil {
		t.Fatalf("read invite: %v", err)
	}
	if consumedBy != appUser {
		t.Fatalf("consumed_by = %v, want %v", consumedBy, appUser)
	}
	if !contains(env.roles(t, appUser), auth.RoleClient) {
		t.Fatalf("roles = %v, want client", env.roles(t, appUser))
	}
}

func TestTrainerInvite_acceptAsUser_repeatBySameUserIsAlreadyLinked(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	token := env.invite(t, env.trainer(t, "app-repeat-trainer@test.com", "Trainer"))
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-repeat-1", "app-repeat@test.com")

	if _, err := env.store.AcceptInviteAsUser(ctx, token, appUser); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	repeat, err := env.store.AcceptInviteAsUser(ctx, token, appUser)
	if err != nil {
		t.Fatalf("repeat accept: %v", err)
	}
	if !repeat.AlreadyLinked {
		t.Fatal("expected already_linked on repeat")
	}
	if env.linkCount(t, appUser) != 1 {
		t.Fatalf("trainer_clients rows = %d, want 1", env.linkCount(t, appUser))
	}
}

func TestTrainerInvite_acceptAsUser_secondUserGetsConsumed(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	token := env.invite(t, env.trainer(t, "app-consumed-trainer@test.com", "Trainer"))
	first := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-consumed-1", "app-consumed-1@test.com")
	second := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-consumed-2", "app-consumed-2@test.com")

	env.withoutRoles(t, second)
	if _, err := env.store.AcceptInviteAsUser(ctx, token, first); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	_, err := env.store.AcceptInviteAsUser(ctx, token, second)
	if !errors.Is(err, trainerclient.ErrInviteConsumed) {
		t.Fatalf("error = %v, want ErrInviteConsumed", err)
	}
	if env.linkCount(t, second) != 0 {
		t.Fatal("second user must not be linked")
	}
	env.assertNoClientRole(t, second)
}

func TestTrainerInvite_acceptAsUser_expiredLinksNothing(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	token := env.invite(t, env.trainer(t, "app-expired-trainer@test.com", "Trainer"))
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-expired-1", "app-expired@test.com")
	env.withoutRoles(t, appUser)
	if _, err := env.pool.Exec(ctx, `UPDATE mentorix.trainer_invites SET expires_at = now() - interval '1 hour' WHERE token = $1`, token); err != nil {
		t.Fatalf("expire invite: %v", err)
	}

	_, err := env.store.AcceptInviteAsUser(ctx, token, appUser)
	if !errors.Is(err, trainerclient.ErrInviteExpired) {
		t.Fatalf("error = %v, want ErrInviteExpired", err)
	}
	if env.linkCount(t, appUser) != 0 {
		t.Fatal("expired invite must not link")
	}
	env.assertNoClientRole(t, appUser)
	env.assertUnconsumed(t, token)
}

func TestTrainerInvite_acceptAsUser_unknownToken(t *testing.T) {
	env := newAppInviteEnv(t)
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-unknown-tok", "app-unknown-tok@test.com")

	_, err := env.store.AcceptInviteAsUser(context.Background(), "missing-token", appUser)
	if !errors.Is(err, trainerclient.ErrInviteNotFound) {
		t.Fatalf("error = %v, want ErrInviteNotFound", err)
	}
}

func TestTrainerInvite_acceptAsUser_ownInviteRefused(t *testing.T) {
	env := newAppInviteEnv(t)
	trainerUserID := env.trainer(t, "app-self-trainer@test.com", "Trainer")
	token := env.invite(t, trainerUserID)

	_, err := env.store.AcceptInviteAsUser(context.Background(), token, trainerUserID)
	if !errors.Is(err, trainerclient.ErrSelfInvite) {
		t.Fatalf("error = %v, want ErrSelfInvite", err)
	}
	if env.linkCount(t, trainerUserID) != 0 {
		t.Fatal("own invite must not link")
	}
	env.assertNoClientRole(t, trainerUserID)
	env.assertUnconsumed(t, token)
}

func TestTrainerInvite_acceptAsUser_trainerAcceptsAnotherTrainersInvite(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerA := env.trainer(t, "app-cross-a@test.com", "Trainer A")
	trainerB := env.trainer(t, "app-cross-b@test.com", "Trainer B")
	token := env.invite(t, trainerA)

	result, err := env.store.AcceptInviteAsUser(ctx, token, trainerB)
	if err != nil {
		t.Fatalf("AcceptInviteAsUser: %v", err)
	}
	if result.AlreadyLinked {
		t.Fatal("expected a new link")
	}
	roles := env.roles(t, trainerB)
	if !contains(roles, auth.RoleTrainer) || !contains(roles, auth.RoleClient) {
		t.Fatalf("roles = %v, want trainer and client", roles)
	}
	if env.linkCount(t, trainerB) != 1 {
		t.Fatalf("trainer_clients rows = %d, want 1", env.linkCount(t, trainerB))
	}
}

func TestTrainerInvite_acceptAsUser_quotaFullKeepsInviteUnconsumed(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerUserID := env.trainer(t, "app-quota-trainer@test.com", "Trainer")
	// Issue the invite first: CreateInvite refuses once the client quota is full.
	token := env.invite(t, trainerUserID)
	for i := 0; i < 3; i++ {
		if _, err := env.store.AcceptInvite(ctx, env.invite(t, trainerUserID), fmt.Sprintf("app-quota-tg-%d", i), "Client"); err != nil {
			t.Fatalf("fill quota %d: %v", i, err)
		}
	}
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-quota-1", "app-quota@test.com")
	env.withoutRoles(t, appUser)

	_, err := env.store.AcceptInviteAsUser(ctx, token, appUser)
	if !errors.Is(err, trainerclient.ErrClientLimitReached) {
		t.Fatalf("error = %v, want ErrClientLimitReached", err)
	}
	var qe *subscription.QuotaError
	if errors.As(err, &qe) {
		t.Fatalf("error = %v, the accept path must return the sentinel, not a QuotaError", err)
	}
	if env.linkCount(t, appUser) != 0 {
		t.Fatal("quota refusal must not link")
	}
	env.assertNoClientRole(t, appUser)
	env.assertUnconsumed(t, token)
}

// The real store and service behind the real handler: a full plan answers a plain
// 409, not the structured quota_exceeded body that POST /trainer/invites returns.
func TestTrainerInvite_acceptAsUser_quotaFullHTTPBody(t *testing.T) {
	const secret = "test-jwt-secret-at-least-32-chars-long"
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerUserID := env.trainer(t, "app-quota-http-trainer@test.com", "Trainer")
	token := env.invite(t, trainerUserID)
	for i := 0; i < 3; i++ {
		if _, err := env.store.AcceptInvite(ctx, env.invite(t, trainerUserID), fmt.Sprintf("app-quota-http-tg-%d", i), "Client"); err != nil {
			t.Fatalf("fill quota %d: %v", i, err)
		}
	}
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-quota-http-1", "app-quota-http@test.com")
	env.withoutRoles(t, appUser)

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   appUser.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	trainerclient.NewHandlers(env.svc, env.pool, secret).Mount(e)

	req := httptest.NewRequest(http.MethodPost, "/client/invites/accept", strings.NewReader(`{"token":"`+token+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signed)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body %s", rec.Code, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"message":"client limit reached"}` {
		t.Fatalf("body = %s, want exactly {\"message\":\"client limit reached\"}", got)
	}
	env.assertUnconsumed(t, token)
}

func TestTrainerInvite_acceptAsUser_unknownUser(t *testing.T) {
	env := newAppInviteEnv(t)
	token := env.invite(t, env.trainer(t, "app-nouser-trainer@test.com", "Trainer"))

	_, err := env.store.AcceptInviteAsUser(context.Background(), token, uuid.New())
	if !errors.Is(err, trainerclient.ErrUserNotFound) {
		t.Fatalf("error = %v, want ErrUserNotFound", err)
	}
	if n := countRows(t, env.pool, `SELECT count(*) FROM mentorix.trainer_invites WHERE token = $1 AND consumed_at IS NULL`, token); n != 1 {
		t.Fatalf("unconsumed invite rows = %d, want 1", n)
	}
}

func TestTrainerInvite_acceptAsUser_blockedClientGetsNoRole(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerUserID := env.trainer(t, "app-blocked-trainer@test.com", "Trainer")
	token := env.invite(t, trainerUserID)
	appUser := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-blocked-1", "app-blocked@test.com")
	env.withoutRoles(t, appUser)
	if _, err := env.pool.Exec(ctx, `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
SELECT id, $2, 'blocked' FROM mentorix.trainers WHERE user_id = $1`, trainerUserID, appUser); err != nil {
		t.Fatalf("insert blocked link: %v", err)
	}

	_, err := env.store.AcceptInviteAsUser(ctx, token, appUser)
	if !errors.Is(err, program.ErrClientBlocked) {
		t.Fatalf("error = %v, want ErrClientBlocked", err)
	}
	env.assertNoClientRole(t, appUser)
	env.assertUnconsumed(t, token)
}

func TestTrainerInvite_acceptAsUser_adminIsRefused(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	token := env.invite(t, env.trainer(t, "app-admin-trainer@test.com", "Trainer"))
	admin := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-admin-1", "app-admin@test.com")
	env.withoutRoles(t, admin)
	if err := env.auth.GrantRole(ctx, admin, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}

	_, err := env.store.AcceptInviteAsUser(ctx, token, admin)
	if !errors.Is(err, trainerclient.ErrAdminCannotAccept) {
		t.Fatalf("error = %v, want ErrAdminCannotAccept", err)
	}
	if env.linkCount(t, admin) != 0 {
		t.Fatal("admin must not be linked")
	}
	env.assertUnconsumed(t, token)
	if roles := env.roles(t, admin); len(roles) != 1 || roles[0] != auth.RoleAdmin {
		t.Fatalf("roles = %v, want [admin]", roles)
	}
}
