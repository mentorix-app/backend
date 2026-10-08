//go:build integration

package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

func newClientWithIdentity(t *testing.T, store *auth.Store, provider, subject, email string) uuid.UUID {
	t.Helper()
	id, err := store.FindOrCreateClientByIdentity(context.Background(), auth.ClientIdentity{
		Provider: provider, Subject: subject, Email: email, DisplayName: subject,
	})
	if err != nil {
		t.Fatalf("create client %s/%s: %v", provider, subject, err)
	}
	return id
}

// linkToNewTrainer registers a trainer and links clientID to it.
func linkToNewTrainer(t *testing.T, pool *pgxpool.Pool, store *auth.Store, clientID uuid.UUID, trainerEmail string) {
	t.Helper()
	ctx := context.Background()
	trainerUserID, err := store.RegisterTrainerEmailPassword(ctx, trainerEmail, "hash", "Coach")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id)
SELECT id, $2 FROM mentorix.trainers WHERE user_id = $1`, trainerUserID, clientID); err != nil {
		t.Fatalf("link client to trainer: %v", err)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func identityOwner(t *testing.T, pool *pgxpool.Pool, provider, subject string) (uuid.UUID, bool) {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`SELECT user_id FROM mentorix.auth_identities WHERE provider = $1 AND subject = $2`, provider, subject).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false
	}
	if err != nil {
		t.Fatalf("identity owner: %v", err)
	}
	return id, true
}

func userExists(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) bool {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM mentorix.users WHERE id = $1`, id) == 1
}

func TestAuthStore_LinkTelegram_attachesUnknownTelegram(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "app@test.com")

	got, err := store.LinkTelegram(ctx, app, "tg-100")
	if err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	if got != app {
		t.Errorf("remaining user = %v, want %v", got, app)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-100"); !ok || owner != app {
		t.Errorf("telegram owner = %v, %v, want %v", owner, ok, app)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderGoogle, "g-1"); !ok || owner != app {
		t.Errorf("google owner = %v, %v, want %v", owner, ok, app)
	}
}

func TestAuthStore_LinkTelegram_sameTelegramAgainIsNoop(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	app := newClientWithIdentity(t, store, auth.ProviderApple, "a-1", "")
	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("first link: %v", err)
	}

	got, err := store.LinkTelegram(ctx, app, "tg-100")
	if err != nil {
		t.Fatalf("second link: %v", err)
	}
	if got != app {
		t.Errorf("remaining user = %v, want %v", got, app)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1`, app); n != 2 {
		t.Errorf("identities of the account = %d, want 2", n)
	}
}

func TestAuthStore_LinkTelegram_anotherTelegramOnAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")
	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("first link: %v", err)
	}

	_, err := store.LinkTelegram(ctx, app, "tg-200")
	if !errors.Is(err, auth.ErrTelegramAlreadyLinked) {
		t.Fatalf("error = %v, want ErrTelegramAlreadyLinked", err)
	}
	if _, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-200"); ok {
		t.Error("second telegram was attached")
	}
}

func TestAuthStore_LinkTelegram_unknownAppUser(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)

	_, err := store.LinkTelegram(context.Background(), uuid.New(), "tg-100")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("error = %v, want pgx.ErrNoRows", err)
	}
	if _, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-100"); ok {
		t.Error("telegram identity was created for a missing user")
	}
}

func TestAuthStore_LinkTelegram_movesIntoTelegramAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	linkToNewTrainer(t, pool, store, old, "coach@test.com")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "app@test.com")
	if err := store.InsertRefreshSession(ctx, app, []byte("app-session-hash"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	got, err := store.LinkTelegram(ctx, app, "tg-100")
	if err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	if got != old {
		t.Fatalf("remaining user = %v, want the telegram account %v", got, old)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderGoogle, "g-1"); !ok || owner != old {
		t.Errorf("google owner = %v, %v, want %v", owner, ok, old)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-100"); !ok || owner != old {
		t.Errorf("telegram owner = %v, %v, want %v", owner, ok, old)
	}
	if userExists(t, pool, app) {
		t.Error("app user still exists")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_refresh_sessions WHERE user_id = $1`, app); n != 0 {
		t.Errorf("refresh sessions of the removed account = %d, want 0", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE client_user_id = $1`, old); n != 1 {
		t.Errorf("trainer links of the telegram account = %d, want 1", n)
	}
	profile, err := store.UserProfile(ctx, old)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "app@test.com" {
		t.Errorf("email = %q, want it filled from the app account", profile.Email)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleClient {
		t.Errorf("roles = %v, want [client]", profile.Roles)
	}
}

func TestAuthStore_LinkTelegram_keepsEmailOfTelegramAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "old@test.com")
	app := newClientWithIdentity(t, store, auth.ProviderApple, "a-1", "app@test.com")

	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	profile, err := store.UserProfile(ctx, old)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "old@test.com" {
		t.Errorf("email = %q, want old@test.com", profile.Email)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderApple, "a-1"); !ok || owner != old {
		t.Errorf("apple owner = %v, %v, want %v", owner, ok, old)
	}
}

func TestAuthStore_LinkTelegram_grantsClientRoleToTelegramAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	if _, err := pool.Exec(ctx, `DELETE FROM mentorix.user_roles WHERE user_id = $1`, old); err != nil {
		t.Fatalf("drop role: %v", err)
	}
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")

	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	roles, err := store.UserRoles(ctx, old)
	if err != nil {
		t.Fatalf("UserRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleClient {
		t.Errorf("roles = %v, want [client]", roles)
	}
}

// assertLinkRefused checks the refusal left both accounts and their identities as they were.
func assertLinkRefused(t *testing.T, pool *pgxpool.Pool, err error, want error, app, old uuid.UUID) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if !userExists(t, pool, app) || !userExists(t, pool, old) {
		t.Error("a refused link removed an account")
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderGoogle, "g-1"); !ok || owner != app {
		t.Errorf("google owner = %v, %v, want %v", owner, ok, app)
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-100"); !ok || owner != old {
		t.Errorf("telegram owner = %v, %v, want %v", owner, ok, old)
	}
}

func TestAuthStore_LinkTelegram_refusedWhenAppAccountHasTrainerLink(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "app@test.com")
	linkToNewTrainer(t, pool, store, app, "coach@test.com")

	_, err := store.LinkTelegram(context.Background(), app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramLinkConflict, app, old)
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE client_user_id = $1`, app); n != 1 {
		t.Errorf("trainer links of the app account = %d, want 1", n)
	}
}

func TestAuthStore_LinkTelegram_refusedWhenAppAccountIsTrainer(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")
	if err := store.GrantRole(ctx, app, auth.RoleTrainer); err != nil {
		t.Fatalf("grant trainer: %v", err)
	}

	_, err := store.LinkTelegram(ctx, app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramLinkConflict, app, old)
}

func TestAuthStore_LinkTelegram_refusedWhenAppAccountHasAnotherTelegram(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")
	if _, err := store.LinkTelegram(ctx, app, "tg-200"); err != nil {
		t.Fatalf("first link: %v", err)
	}

	_, err := store.LinkTelegram(ctx, app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramAlreadyLinked, app, old)
	if owner, ok := identityOwner(t, pool, auth.ProviderTelegram, "tg-200"); !ok || owner != app {
		t.Errorf("first telegram owner = %v, %v, want %v", owner, ok, app)
	}
}

func TestAuthStore_UserProfile_telegramLinked(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")

	before, err := store.UserProfile(ctx, app)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if before.TelegramLinked {
		t.Error("TelegramLinked = true before linking")
	}
	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	after, err := store.UserProfile(ctx, app)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if !after.TelegramLinked {
		t.Error("TelegramLinked = false after linking")
	}
}

func TestAuthStore_LinkTelegram_parallelLinksOfSameTelegramBothSucceed(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	for i := range 5 {
		app := newClientWithIdentity(t, store, auth.ProviderGoogle, fmt.Sprintf("g-race-%d", i), "")
		telegramID := fmt.Sprintf("tg-race-%d", i)

		const workers = 2
		got := make([]uuid.UUID, workers)
		errs := make([]error, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for w := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				got[w], errs[w] = store.LinkTelegram(ctx, app, telegramID)
			}()
		}
		close(start)
		wg.Wait()

		for w := range workers {
			if errs[w] != nil {
				t.Fatalf("iteration %d worker %d: %v", i, w, errs[w])
			}
			if got[w] != app {
				t.Errorf("iteration %d worker %d user = %v, want %v", i, w, got[w], app)
			}
		}
		if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE provider = $1 AND subject = $2`, auth.ProviderTelegram, telegramID); n != 1 {
			t.Errorf("iteration %d: telegram identity rows = %d, want 1", i, n)
		}
	}
}

func TestAuthStore_LinkTelegram_refusedWhenAppAccountHasPasswordSignIn(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")
	if _, err := pool.Exec(ctx, `
INSERT INTO mentorix.auth_identities (user_id, provider, subject, password_hash)
VALUES ($1, 'email_password', 'app@test.com', 'hash')`, app); err != nil {
		t.Fatalf("insert password identity: %v", err)
	}

	_, err := store.LinkTelegram(ctx, app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramLinkConflict, app, old)
	if owner, ok := identityOwner(t, pool, auth.ProviderEmailPassword, "app@test.com"); !ok || owner != app {
		t.Errorf("password identity owner = %v, %v, want %v", owner, ok, app)
	}
}

func TestAuthStore_LinkTelegram_deleteRefusedByForeignKeyIsConflict(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "old@test.com")
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "app@test.com")
	// A RESTRICT reference without a trainer link or a trainer role.
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.programs (created_by, modified_by) VALUES ($1, $1)`, app); err != nil {
		t.Fatalf("insert restricting row: %v", err)
	}

	_, err := store.LinkTelegram(ctx, app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramLinkConflict, app, old)

	profile, err := store.UserProfile(ctx, old)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "old@test.com" {
		t.Errorf("owner email = %q, want it unchanged", profile.Email)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleClient {
		t.Errorf("owner roles = %v, want [client]", profile.Roles)
	}
}

func TestAuthStore_LinkTelegram_fillsEmailStoredAsEmptyString(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	if _, err := pool.Exec(ctx, `UPDATE mentorix.users SET primary_email = '' WHERE id = $1`, old); err != nil {
		t.Fatalf("set empty email: %v", err)
	}
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "app@test.com")

	if _, err := store.LinkTelegram(ctx, app, "tg-100"); err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	profile, err := store.UserProfile(ctx, old)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "app@test.com" {
		t.Errorf("email = %q, want app@test.com", profile.Email)
	}
}

func TestAuthStore_LinkTelegram_adminTelegramAccountIsConflict(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	old := newClientWithIdentity(t, store, auth.ProviderTelegram, "tg-100", "")
	if _, err := pool.Exec(ctx, `DELETE FROM mentorix.user_roles WHERE user_id = $1`, old); err != nil {
		t.Fatalf("drop role: %v", err)
	}
	if err := store.GrantRole(ctx, old, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	app := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-1", "")

	_, err := store.LinkTelegram(ctx, app, "tg-100")
	assertLinkRefused(t, pool, err, auth.ErrTelegramLinkConflict, app, old)

	roles, err := store.UserRoles(ctx, old)
	if err != nil {
		t.Fatalf("UserRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleAdmin {
		t.Errorf("owner roles = %v, want [admin]", roles)
	}
}
