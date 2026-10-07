//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

func socialClaims(subject, email string, verified bool) auth.IDTokenClaims {
	return auth.IDTokenClaims{Subject: subject, Email: email, EmailVerified: verified}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestAuthStore_SocialSignIn_createsUserWithoutRoles(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID, created, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("g-1", "Person@Example.com", true), "Person")
	if err != nil {
		t.Fatalf("SocialSignIn() error = %v", err)
	}
	if !created || userID == uuid.Nil {
		t.Fatalf("created = %v, user id = %v; want a new user", created, userID)
	}

	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "person@example.com" {
		t.Errorf("email = %q, want lower-cased verified email", profile.Email)
	}
	if profile.Name != "Person" {
		t.Errorf("name = %q, want Person", profile.Name)
	}
	if len(profile.Roles) != 0 {
		t.Errorf("roles = %v, want none", profile.Roles)
	}
	n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1 AND provider = 'google' AND subject = 'g-1' AND password_hash IS NULL`, userID)
	if n != 1 {
		t.Errorf("google identities = %d, want 1 without password hash", n)
	}
}

func TestAuthStore_SocialSignIn_signInAgainReturnsSameUser(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	first, created, err := store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("a-1", "a@example.com", true), "A")
	if err != nil || !created {
		t.Fatalf("first SocialSignIn() = %v, %v, %v", first, created, err)
	}
	// A changed email on the second token must not matter: the key is the subject.
	second, created, err := store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("a-1", "changed@example.com", true), "Other")
	if err != nil {
		t.Fatalf("second SocialSignIn() error = %v", err)
	}
	if created {
		t.Error("second sign-in reported created")
	}
	if second != first {
		t.Errorf("second user = %v, want %v", second, first)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestAuthStore_SocialSignIn_sameSubjectOnDifferentProvidersAreDifferentUsers(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	a, _, err := store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("same", "", false), "")
	if err != nil {
		t.Fatalf("apple: %v", err)
	}
	g, _, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("same", "", false), "")
	if err != nil {
		t.Fatalf("google: %v", err)
	}
	if a == g {
		t.Error("apple and google subjects resolved to one user")
	}
}

func TestAuthStore_SocialSignIn_emailConflictCreatesNothing(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := store.RegisterTrainerEmailPassword(ctx, "taken@example.com", hash, "Owner"); err != nil {
		t.Fatalf("register: %v", err)
	}
	users := countRows(t, pool, `SELECT count(*) FROM mentorix.users`)
	identities := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`)

	_, created, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("g-2", "Taken@Example.com", true), "Thief")
	if !errors.Is(err, auth.ErrEmailBelongsToAnotherAccount) {
		t.Fatalf("SocialSignIn() error = %v, want ErrEmailBelongsToAnotherAccount", err)
	}
	if created {
		t.Error("created = true on conflict")
	}
	if got := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); got != users {
		t.Errorf("users = %d, want %d", got, users)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`); got != identities {
		t.Errorf("identities = %d, want %d", got, identities)
	}
}

func TestAuthStore_SocialSignIn_knownIdentityIgnoresEmailConflict(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	first, _, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("g-3", "", false), "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := store.RegisterTrainerEmailPassword(ctx, "later@example.com", hash, ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	again, created, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("g-3", "later@example.com", true), "")
	if err != nil || created || again != first {
		t.Fatalf("SocialSignIn() = %v, %v, %v; want %v, false, nil", again, created, err, first)
	}
}

func TestAuthStore_SocialSignIn_unverifiedEmailDoesNotConflictAndIsNotStored(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := store.RegisterTrainerEmailPassword(ctx, "owner@example.com", hash, ""); err != nil {
		t.Fatalf("register: %v", err)
	}

	userID, created, err := store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("a-2", "owner@example.com", false), "")
	if err != nil {
		t.Fatalf("SocialSignIn() error = %v", err)
	}
	if !created {
		t.Fatal("expected a new user")
	}
	n := countRows(t, pool, `SELECT count(*) FROM mentorix.users WHERE id = $1 AND primary_email IS NULL`, userID)
	if n != 1 {
		t.Error("primary_email should be NULL for an unverified email")
	}
}

func TestAuthStore_SocialSignIn_noEmailStoresNull(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID, _, err := store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("a-3", "", true), "")
	if err != nil {
		t.Fatalf("SocialSignIn() error = %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users WHERE id = $1 AND primary_email IS NULL`, userID); n != 1 {
		t.Error("primary_email should be NULL")
	}
}

func TestAuthStore_SocialSignIn_concurrentFirstSignInCreatesOneUser(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	const workers = 8
	type result struct {
		id      uuid.UUID
		created bool
		err     error
	}
	results := make(chan result, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			id, created, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("race", "", false), "")
			results <- result{id, created, err}
		}()
	}
	close(start)

	var ids = map[uuid.UUID]struct{}{}
	createdCount := 0
	for i := 0; i < workers; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("SocialSignIn() error = %v", r.err)
			continue
		}
		ids[r.id] = struct{}{}
		if r.created {
			createdCount++
		}
	}
	if len(ids) != 1 {
		t.Errorf("distinct users = %d, want 1", len(ids))
	}
	if createdCount != 1 {
		t.Errorf("created reported %d times, want 1", createdCount)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); n != 1 {
		t.Errorf("users rows = %d, want 1 (no orphans)", n)
	}
}

func newSocialUser(t *testing.T, store *auth.Store, subject string) uuid.UUID {
	t.Helper()
	id, _, err := store.SocialSignIn(context.Background(), auth.ProviderGoogle, socialClaims(subject, "", false), "")
	if err != nil {
		t.Fatalf("SocialSignIn() error = %v", err)
	}
	return id
}

func TestAuthStore_AddRole_client(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newSocialUser(t, store, "r-1")

	for i := 0; i < 2; i++ {
		if err := store.AddRole(ctx, userID, auth.RoleClient); err != nil {
			t.Fatalf("AddRole() call %d error = %v", i+1, err)
		}
	}
	roles, err := store.UserRoles(ctx, userID)
	if err != nil {
		t.Fatalf("UserRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleClient {
		t.Errorf("roles = %v, want [client]", roles)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainers WHERE user_id = $1`, userID); n != 0 {
		t.Errorf("trainers rows = %d, want 0 for a client", n)
	}
}

func TestAuthStore_AddRole_trainerCreatesTrainerRowOnce(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newSocialUser(t, store, "r-2")

	for i := 0; i < 2; i++ {
		if err := store.AddRole(ctx, userID, auth.RoleTrainer); err != nil {
			t.Fatalf("AddRole() call %d error = %v", i+1, err)
		}
	}
	roles, err := store.UserRoles(ctx, userID)
	if err != nil {
		t.Fatalf("UserRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleTrainer {
		t.Errorf("roles = %v, want [trainer]", roles)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainers WHERE user_id = $1`, userID); n != 1 {
		t.Errorf("trainers rows = %d, want 1", n)
	}
}

func TestAuthStore_AddRole_trainerKeepsExistingTrainerRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(ctx, "existing-trainer@example.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.AddRole(ctx, userID, auth.RoleTrainer); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainers WHERE user_id = $1`, userID); n != 1 {
		t.Errorf("trainers rows = %d, want 1", n)
	}
}

func TestAuthStore_AddRole_adminAccountConflicts(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	adminID, err := store.RegisterTrainerEmailPassword(ctx, "admin-role@example.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	promoteToAdminOnly(t, pool, adminID)

	for _, role := range []string{auth.RoleTrainer, auth.RoleClient} {
		if err := store.AddRole(ctx, adminID, role); !errors.Is(err, auth.ErrRoleConflict) {
			t.Errorf("AddRole(%s) error = %v, want ErrRoleConflict", role, err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainers WHERE user_id = $1`, adminID); n != 0 {
		t.Errorf("trainers rows = %d, want 0 after the rejected trainer role", n)
	}
	roles, err := store.UserRoles(ctx, adminID)
	if err != nil {
		t.Fatalf("UserRoles() error = %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleAdmin {
		t.Errorf("roles = %v, want [admin]", roles)
	}
}

func TestAuthStore_AddRole_rejectsRolesOutsideTheSelfServiceSet(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newSocialUser(t, store, "r-3")

	for _, role := range []string{auth.RoleAdmin, "", "owner"} {
		if err := store.AddRole(ctx, userID, role); err == nil {
			t.Errorf("AddRole(%q) succeeded, want an error", role)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.user_roles WHERE user_id = $1`, userID); n != 0 {
		t.Errorf("roles = %d, want 0", n)
	}
}

func TestAuthStore_SocialSignIn_concurrentSameEmailThroughTwoProvidersCreatesOneUser(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	providers := []string{auth.ProviderApple, auth.ProviderGoogle}
	errs := make(chan error, len(providers))
	start := make(chan struct{})
	for _, p := range providers {
		go func() {
			<-start
			_, _, err := store.SocialSignIn(ctx, p, socialClaims("sub-"+p, "same@example.com", true), "")
			errs <- err
		}()
	}
	close(start)

	var ok, conflicts int
	for range providers {
		err := <-errs
		switch {
		case err == nil:
			ok++
		case errors.Is(err, auth.ErrEmailBelongsToAnotherAccount):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Errorf("successes = %d, conflicts = %d; want 1 and 1", ok, conflicts)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestAuthStore_RegisterTrainerEmailPassword_afterSocialAccountWithSameEmail(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	if _, _, err := store.SocialSignIn(ctx, auth.ProviderGoogle, socialClaims("g-reg", "held@example.com", true), ""); err != nil {
		t.Fatalf("SocialSignIn() error = %v", err)
	}
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	_, err = store.RegisterTrainerEmailPassword(ctx, "held@example.com", hash, "")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("RegisterTrainerEmailPassword() error = %v, want ErrEmailTaken", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestAuthStore_SocialSignIn_afterPasswordRegistrationWithSameEmail(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := store.RegisterTrainerEmailPassword(ctx, "first@example.com", hash, ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, _, err = store.SocialSignIn(ctx, auth.ProviderApple, socialClaims("a-after", "first@example.com", true), "")
	if !errors.Is(err, auth.ErrEmailBelongsToAnotherAccount) {
		t.Fatalf("SocialSignIn() error = %v, want ErrEmailBelongsToAnotherAccount", err)
	}
}

func TestAuthStore_primaryEmailIsUniqueIgnoringCase(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES ('Case@Example.com', '')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES ('case@example.COM', '')`)
	if err == nil {
		t.Fatal("second insert with the same email in another case succeeded")
	}
}

func TestAuthStore_primaryEmailNullsDoNotConflict(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES (NULL, '')`); err != nil {
			t.Fatalf("insert %d with NULL email: %v", i+1, err)
		}
	}
}

func TestAuthStore_AddRole_unknownUserIsNotFound(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	for _, role := range []string{auth.RoleClient, auth.RoleTrainer} {
		if err := store.AddRole(ctx, uuid.New(), role); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("AddRole(%s) error = %v, want pgx.ErrNoRows", role, err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainers`); n != 0 {
		t.Errorf("trainers rows = %d, want 0", n)
	}
}
