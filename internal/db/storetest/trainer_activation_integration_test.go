//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/subscription"
)

// trainerTerms is the terms acceptance stored on a trainers row.
type trainerTerms struct {
	found      bool
	id         uuid.UUID
	version    *string
	acceptedAt *time.Time
}

func trainerTermsOf(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) trainerTerms {
	t.Helper()
	var got trainerTerms
	err := pool.QueryRow(context.Background(),
		`SELECT id, terms_version, terms_accepted_at FROM mentorix.trainers WHERE user_id = $1`, userID).
		Scan(&got.id, &got.version, &got.acceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return trainerTerms{}
	}
	if err != nil {
		t.Fatalf("load trainer terms: %v", err)
	}
	got.found = true
	return got
}

func assertTerms(t *testing.T, got trainerTerms, wantVersion string) {
	t.Helper()
	if !got.found {
		t.Fatal("no trainers row")
	}
	if got.version == nil || *got.version != wantVersion {
		t.Errorf("terms_version = %v, want %q", got.version, wantVersion)
	}
	if got.acceptedAt == nil {
		t.Fatal("terms_accepted_at is null")
	}
}

func assertRoles(t *testing.T, store *auth.Store, userID uuid.UUID, want ...string) {
	t.Helper()
	got, err := store.UserRoles(context.Background(), userID)
	if err != nil {
		t.Fatalf("UserRoles: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("roles = %v, want %v", got, want)
	}
}

func TestTrainerActivation_clientBecomesATrainer(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activate", "activate@test.com")
	if got := trainerTermsOf(t, pool, client); got.found {
		t.Fatal("the fixture client already has a trainers row")
	}
	started := time.Now().Add(-time.Minute)

	if err := store.ActivateTrainer(ctx, client, "2026-10-01"); err != nil {
		t.Fatalf("ActivateTrainer() error = %v", err)
	}

	assertRoles(t, store, client, auth.RoleClient, auth.RoleTrainer)
	terms := trainerTermsOf(t, pool, client)
	assertTerms(t, terms, "2026-10-01")
	if terms.acceptedAt.Before(started) || terms.acceptedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("terms_accepted_at = %v, want about now", terms.acceptedAt)
	}
	profile, err := store.UserProfile(ctx, client)
	if err != nil {
		t.Fatalf("UserProfile: %v", err)
	}
	if !reflect.DeepEqual(profile.Roles, []string{auth.RoleClient, auth.RoleTrainer}) {
		t.Errorf("profile roles = %v", profile.Roles)
	}
	sub, err := subscription.NewService(pool).ForUser(ctx, client)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	if sub == nil || sub.Plan != subscription.PlanFree || sub.Source != nil {
		t.Errorf("subscription = %+v, want the free plan without a source", sub)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_plan_entitlements WHERE trainer_id = $1`, terms.id); n != 0 {
		t.Errorf("entitlement rows = %d, want none", n)
	}
}

func TestTrainerActivation_sameVersionChangesNothing(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activate-twice", "")
	if err := store.ActivateTrainer(ctx, client, "2026-10-01"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	first := trainerTermsOf(t, pool, client)
	counts := tableCounts(t, pool)

	if err := store.ActivateTrainer(ctx, client, "2026-10-01"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	second := trainerTermsOf(t, pool, client)
	if second.id != first.id {
		t.Errorf("trainers.id changed: %v -> %v", first.id, second.id)
	}
	if !second.acceptedAt.Equal(*first.acceptedAt) {
		t.Errorf("terms_accepted_at moved: %v -> %v", first.acceptedAt, second.acceptedAt)
	}
	assertTerms(t, second, "2026-10-01")
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
	assertRoles(t, store, client, auth.RoleClient, auth.RoleTrainer)
}

func TestTrainerActivation_anotherVersionOverwritesVersionAndTime(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activate-new-terms", "")
	if err := store.ActivateTrainer(ctx, client, "2026-10-01"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	first := trainerTermsOf(t, pool, client)
	counts := tableCounts(t, pool)

	if err := store.ActivateTrainer(ctx, client, "2027-01-15"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	second := trainerTermsOf(t, pool, client)
	assertTerms(t, second, "2027-01-15")
	if !second.acceptedAt.After(*first.acceptedAt) {
		t.Errorf("terms_accepted_at = %v, want later than %v", second.acceptedAt, first.acceptedAt)
	}
	if second.id != first.id {
		t.Errorf("trainers.id changed: %v -> %v", first.id, second.id)
	}
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
}

// A trainer from email registration has a trainers row without terms; activating records them
// and does not add the client role.
func TestTrainerActivation_registeredTrainerRecordsTerms(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	trainer, err := store.RegisterTrainerEmailPassword(ctx, "registered-activate@test.com", "hash", "Coach")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	before := trainerTermsOf(t, pool, trainer)
	if !before.found || before.version != nil || before.acceptedAt != nil {
		t.Fatalf("registered trainer terms = %+v, want a row without terms", before)
	}
	counts := tableCounts(t, pool)

	if err := store.ActivateTrainer(ctx, trainer, "2026-10-01"); err != nil {
		t.Fatalf("ActivateTrainer() error = %v", err)
	}

	after := trainerTermsOf(t, pool, trainer)
	assertTerms(t, after, "2026-10-01")
	if after.id != before.id {
		t.Errorf("trainers.id changed: %v -> %v", before.id, after.id)
	}
	assertRoles(t, store, trainer, auth.RoleTrainer)
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
}

func TestTrainerActivation_accountWithoutRoleRowsGetsTheTrainerRole(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activate-no-role", "")
	if _, err := pool.Exec(context.Background(), `DELETE FROM mentorix.user_roles WHERE user_id = $1`, client); err != nil {
		t.Fatalf("drop roles: %v", err)
	}

	if err := store.ActivateTrainer(context.Background(), client, "2026-10-01"); err != nil {
		t.Fatalf("ActivateTrainer() error = %v", err)
	}

	assertRoles(t, store, client, auth.RoleTrainer)
	assertTerms(t, trainerTermsOf(t, pool, client), "2026-10-01")
}

func TestTrainerActivation_adminIsRefusedAndNothingChanges(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	admin, err := store.RegisterTrainerEmailPassword(ctx, "activate-admin@test.com", "hash", "Admin")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	promoteToAdminOnly(t, pool, admin)
	counts := tableCounts(t, pool)

	err = store.ActivateTrainer(ctx, admin, "2026-10-01")

	if !errors.Is(err, auth.ErrAdminCannotActivate) {
		t.Fatalf("error = %v, want ErrAdminCannotActivate", err)
	}
	assertRoles(t, store, admin, auth.RoleAdmin)
	if got := trainerTermsOf(t, pool, admin); got.found {
		t.Errorf("a trainers row was created for an admin: %+v", got)
	}
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
}

func TestTrainerActivation_unknownUserIsNoRows(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	counts := tableCounts(t, pool)

	err := store.ActivateTrainer(context.Background(), uuid.New(), "2026-10-01")

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("error = %v, want pgx.ErrNoRows", err)
	}
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
}

// An admin role granted while the activation waits for the user row lock must be seen: the
// roles are read after the lock, so the account is refused and nothing is created.
func TestTrainerActivation_readsRolesAfterTheLock(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	user := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activate-admin-race", "")
	if _, err := pool.Exec(context.Background(), `DELETE FROM mentorix.user_roles WHERE user_id = $1`, user); err != nil {
		t.Fatalf("drop roles: %v", err)
	}

	err := deleteBlockedBy(t, pool, func() error { return store.ActivateTrainer(context.Background(), user, "2026-10-01") },
		`INSERT INTO mentorix.user_roles (user_id, role) VALUES ($1, 'admin')`, user)

	if !errors.Is(err, auth.ErrAdminCannotActivate) {
		t.Fatalf("error = %v, want ErrAdminCannotActivate", err)
	}
	assertRoles(t, store, user, auth.RoleAdmin)
	if got := trainerTermsOf(t, pool, user); got.found {
		t.Errorf("a trainers row was created for an admin: %+v", got)
	}
}

// Quota writers lock the trainers row and then touch the user row through a foreign key.
// The activation takes the two in the same order, so a writer that is mid-flight finishes
// instead of ending in a deadlock.
func TestTrainerActivation_locksTheTrainerRowBeforeTheUserRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	trainer, err := store.RegisterTrainerEmailPassword(ctx, "activate-lock@test.com", "hash", "Coach")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.trainers WHERE user_id = $1 FOR UPDATE`, trainer); err != nil {
		t.Fatalf("lock trainers row: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- store.ActivateTrainer(ctx, trainer, "2026-10-01") }()
	waitForLockWait(t, pool)
	// A writer that holds the trainers row now touches the user row. If the activation held
	// the user row already, this would deadlock with it.
	if _, err := tx.Exec(ctx, `UPDATE mentorix.users SET display_name = 'Coach 2' WHERE id = $1`, trainer); err != nil {
		t.Fatalf("touch the user row while the activation waits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ActivateTrainer() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the activation did not finish after the trainers row was released")
	}
	assertTerms(t, trainerTermsOf(t, pool, trainer), "2026-10-01")
}

type meBody struct {
	UserID       string   `json:"user_id"`
	Roles        []string `json:"roles"`
	Subscription *struct {
		Plan   string  `json:"plan"`
		Source *string `json:"source"`
	} `json:"subscription"`
}

func decodeMe(t *testing.T, body []byte) meBody {
	t.Helper()
	var me meBody
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("body is not a profile: %v; body = %s", err, body)
	}
	return me
}

// POST /auth/me/trainer through app.Mount: the profile it returns is the one GET /auth/me
// returns afterwards, with the free plan, and the second call answers the same.
func TestTrainerActivation_mountedRoute(t *testing.T) {
	m := newMountedEnv(t)
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-activate", "")
	token := m.accessToken(t, client)
	if me := decodeMe(t, m.do(http.MethodGet, "/auth/me", token, "").Body.Bytes()); me.Subscription != nil {
		t.Fatalf("subscription before the activation = %+v, want null", me.Subscription)
	}

	first := m.do(http.MethodPost, "/auth/me/trainer", token, `{"terms_version":"2026-10-01"}`)
	again := m.do(http.MethodPost, "/auth/me/trainer", token, `{"terms_version":"2026-10-01"}`)

	for _, rec := range []struct {
		name string
		code int
		body []byte
	}{{"first", first.Code, first.Body.Bytes()}, {"repeat", again.Code, again.Body.Bytes()}} {
		if rec.code != http.StatusOK {
			t.Fatalf("%s call: status = %d, want 200; body = %s", rec.name, rec.code, rec.body)
		}
		me := decodeMe(t, rec.body)
		if me.UserID != client.String() || !reflect.DeepEqual(me.Roles, []string{auth.RoleClient, auth.RoleTrainer}) {
			t.Errorf("%s call: profile = %+v", rec.name, me)
		}
		if me.Subscription == nil || me.Subscription.Plan != "free" || me.Subscription.Source != nil {
			t.Errorf("%s call: subscription = %+v, want free without a source", rec.name, me.Subscription)
		}
	}
	got := decodeMe(t, m.do(http.MethodGet, "/auth/me", token, "").Body.Bytes())
	if !reflect.DeepEqual(got.Roles, []string{auth.RoleClient, auth.RoleTrainer}) || got.Subscription == nil || got.Subscription.Plan != "free" {
		t.Errorf("GET /auth/me after the activation = %+v", got)
	}
}

func TestTrainerActivation_mountedRouteRefusals(t *testing.T) {
	m := newMountedEnv(t)
	ctx := context.Background()
	admin, err := m.store.RegisterTrainerEmailPassword(ctx, "mount-activate-admin@test.com", "hash", "Admin")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	promoteToAdminOnly(t, m.pool, admin)
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-activate-bad", "")
	counts := tableCounts(t, m.pool)

	assertErrorBody(t, m.do(http.MethodPost, "/auth/me/trainer", m.accessToken(t, admin), `{"terms_version":"2026-10-01"}`),
		http.StatusForbidden, "admin_cannot_activate")
	assertErrorBody(t, m.do(http.MethodPost, "/auth/me/trainer", m.accessToken(t, uuid.New()), `{"terms_version":"2026-10-01"}`),
		http.StatusNotFound, "user_not_found")
	assertErrorBody(t, m.do(http.MethodPost, "/auth/me/trainer", m.accessToken(t, client), `{"terms_version":"2026-13-01"}`),
		http.StatusBadRequest, "bad_request")
	assertErrorBody(t, m.do(http.MethodPost, "/auth/me/trainer", "", `{"terms_version":"2026-10-01"}`),
		http.StatusUnauthorized, "missing_authorization")

	assertTableDeltas(t, counts, tableCounts(t, m.pool), nil)
}
