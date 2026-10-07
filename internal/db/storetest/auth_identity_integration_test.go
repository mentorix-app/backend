//go:build integration

package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

func registerPasswordUser(t *testing.T, store *auth.Store, email string) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id, err := store.RegisterTrainerEmailPassword(context.Background(), email, hash, "")
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
	return id
}

func identityOwners(t *testing.T, pool *pgxpool.Pool, provider, subject string) []uuid.UUID {
	t.Helper()
	var owner uuid.UUID
	err := pool.QueryRow(context.Background(),
		`SELECT user_id FROM mentorix.auth_identities WHERE provider = $1 AND subject = $2`, provider, subject).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatalf("read identity owner: %v", err)
	}
	return []uuid.UUID{owner}
}

func TestAuthStore_AttachIdentity_toPasswordAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := registerPasswordUser(t, store, "owner@example.com")

	if err := store.AttachIdentity(ctx, userID, auth.ProviderGoogle, "g-attach"); err != nil {
		t.Fatalf("AttachIdentity() error = %v", err)
	}

	if owners := identityOwners(t, pool, auth.ProviderGoogle, "g-attach"); len(owners) != 1 || owners[0] != userID {
		t.Errorf("owners = %v, want [%v]", owners, userID)
	}
	n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1 AND provider = 'google' AND password_hash IS NULL`, userID)
	if n != 1 {
		t.Errorf("google identities without password hash = %d, want 1", n)
	}
	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "owner@example.com" {
		t.Errorf("email = %q, want it unchanged", profile.Email)
	}
	if !slices.Equal(profile.Roles, []string{auth.RoleTrainer}) {
		t.Errorf("roles = %v, want [trainer] unchanged", profile.Roles)
	}
}

func TestAuthStore_AttachIdentity_twiceIsNoOp(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := registerPasswordUser(t, store, "twice@example.com")

	for i := 0; i < 2; i++ {
		if err := store.AttachIdentity(ctx, userID, auth.ProviderApple, "a-twice"); err != nil {
			t.Fatalf("AttachIdentity() call %d error = %v", i+1, err)
		}
	}
	n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE provider = 'apple' AND subject = 'a-twice'`)
	if n != 1 {
		t.Errorf("apple identities = %d, want 1", n)
	}
}

func TestAuthStore_AttachIdentity_ownedByAnotherUserIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	owner := newSocialUser(t, store, "g-owned")
	other := registerPasswordUser(t, store, "other@example.com")
	identities := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`)

	err := store.AttachIdentity(ctx, other, auth.ProviderGoogle, "g-owned")
	if !errors.Is(err, auth.ErrIdentityBelongsToAnotherAccount) {
		t.Fatalf("AttachIdentity() error = %v, want ErrIdentityBelongsToAnotherAccount", err)
	}
	if owners := identityOwners(t, pool, auth.ProviderGoogle, "g-owned"); len(owners) != 1 || owners[0] != owner {
		t.Errorf("owners = %v, want [%v]", owners, owner)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`); got != identities {
		t.Errorf("identities = %d, want %d", got, identities)
	}
}

func TestAuthStore_AttachIdentity_concurrentUsersEndWithOneOwner(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	users := []uuid.UUID{
		registerPasswordUser(t, store, "race-a@example.com"),
		registerPasswordUser(t, store, "race-b@example.com"),
	}

	errs := make(chan error, len(users))
	start := make(chan struct{})
	for _, id := range users {
		go func() {
			<-start
			errs <- store.AttachIdentity(ctx, id, auth.ProviderGoogle, "g-race")
		}()
	}
	close(start)

	var ok, refused int
	for range users {
		err := <-errs
		switch {
		case err == nil:
			ok++
		case errors.Is(err, auth.ErrIdentityBelongsToAnotherAccount):
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Errorf("successes = %d, refused = %d; want 1 and 1", ok, refused)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE provider = 'google' AND subject = 'g-race'`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestAuthStore_AttachIdentity_unknownUserIsNotFound(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)

	err := store.AttachIdentity(context.Background(), uuid.New(), auth.ProviderGoogle, "g-ghost")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("AttachIdentity() error = %v, want pgx.ErrNoRows", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities`); n != 0 {
		t.Errorf("identities = %d, want 0", n)
	}
}

func TestAuthStore_UserProfile_signInMethods(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	passwordUser := registerPasswordUser(t, store, "methods@example.com")

	var telegramUser uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES (NULL, '') RETURNING id`).Scan(&telegramUser); err != nil {
		t.Fatalf("insert telegram user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, 'telegram', '4242')`, telegramUser); err != nil {
		t.Fatalf("insert telegram identity: %v", err)
	}

	noIdentity := uuid.Nil
	if err := pool.QueryRow(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES (NULL, '') RETURNING id`).Scan(&noIdentity); err != nil {
		t.Fatalf("insert bare user: %v", err)
	}

	tests := []struct {
		name   string
		userID uuid.UUID
		want   []string
	}{
		{name: "password account", userID: passwordUser, want: []string{auth.ProviderEmailPassword}},
		{name: "telegram only", userID: telegramUser, want: []string{auth.ProviderTelegram}},
		{name: "no identities", userID: noIdentity, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := store.UserProfile(ctx, tt.userID)
			if err != nil {
				t.Fatalf("UserProfile() error = %v", err)
			}
			if !slices.Equal(profile.SignInMethods, tt.want) {
				t.Errorf("sign in methods = %#v, want %#v", profile.SignInMethods, tt.want)
			}
		})
	}

	if err := store.AttachIdentity(ctx, passwordUser, auth.ProviderGoogle, "g-methods-1"); err != nil {
		t.Fatalf("AttachIdentity() error = %v", err)
	}
	if err := store.AttachIdentity(ctx, passwordUser, auth.ProviderApple, "a-methods"); err != nil {
		t.Fatalf("AttachIdentity() apple error = %v", err)
	}
	// A second Google identity must not repeat the provider.
	if err := store.AttachIdentity(ctx, passwordUser, auth.ProviderGoogle, "g-methods-2"); err != nil {
		t.Fatalf("AttachIdentity() second error = %v", err)
	}
	profile, err := store.UserProfile(ctx, passwordUser)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	want := []string{auth.ProviderApple, auth.ProviderEmailPassword, auth.ProviderGoogle}
	if !slices.Equal(profile.SignInMethods, want) {
		t.Errorf("sign in methods after attach = %v, want %v", profile.SignInMethods, want)
	}
}

// A trainer who registered with email and password is refused when Google
// reports the same verified email. After attaching that Google identity to the
// trainer's account, signing in with Google returns the trainer.
func TestAuthStore_AttachIdentity_resolvesEmailConflictOnSocialSignIn(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	trainer := registerPasswordUser(t, store, "trainer@example.com")
	claims := socialClaims("g-t3", "Trainer@Example.com", true)

	if _, _, err := store.SocialSignIn(ctx, auth.ProviderGoogle, claims, ""); !errors.Is(err, auth.ErrEmailBelongsToAnotherAccount) {
		t.Fatalf("first SocialSignIn() error = %v, want ErrEmailBelongsToAnotherAccount", err)
	}
	if err := store.AttachIdentity(ctx, trainer, auth.ProviderGoogle, claims.Subject); err != nil {
		t.Fatalf("AttachIdentity() error = %v", err)
	}
	got, created, err := store.SocialSignIn(ctx, auth.ProviderGoogle, claims, "")
	if err != nil {
		t.Fatalf("second SocialSignIn() error = %v", err)
	}
	if created || got != trainer {
		t.Errorf("SocialSignIn() = %v, created %v; want %v, false", got, created, trainer)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestAuthStore_AttachIdentity_sameUserConcurrentlySucceedsWithOneRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := registerPasswordUser(t, store, "same-race@example.com")

	const workers = 4
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			errs <- store.AttachIdentity(ctx, userID, auth.ProviderApple, "a-same-race")
		}()
	}
	close(start)

	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("AttachIdentity() error = %v", err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE provider = 'apple' AND subject = 'a-same-race'`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestAuthStore_EmailPasswordHash(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	passwordUser := registerPasswordUser(t, store, "hash@example.com")
	socialUser := newSocialUser(t, store, "g-hash")
	var telegramUser uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO mentorix.users (primary_email, display_name) VALUES (NULL, '') RETURNING id`).Scan(&telegramUser); err != nil {
		t.Fatalf("insert telegram user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, 'telegram', '777')`, telegramUser); err != nil {
		t.Fatalf("insert telegram identity: %v", err)
	}

	hash, err := store.EmailPasswordHash(ctx, passwordUser)
	if err != nil || hash == "" {
		t.Fatalf("password account: hash empty = %v, error = %v; want a hash", hash == "", err)
	}
	if ok, err := auth.PasswordMatches(hash, "password123"); err != nil || !ok {
		t.Errorf("stored hash does not match the registration password: ok = %v, err = %v", ok, err)
	}
	for name, id := range map[string]uuid.UUID{"social only": socialUser, "telegram only": telegramUser} {
		got, err := store.EmailPasswordHash(ctx, id)
		if err != nil || got != "" {
			t.Errorf("%s: hash empty = %v, error = %v; want no hash and no error", name, got == "", err)
		}
	}
	// An email_password identity without a hash means no password.
	nullHashUser := newSocialUser(t, store, "g-null-hash")
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, 'email_password', 'null-hash@example.com')`, nullHashUser); err != nil {
		t.Fatalf("insert email_password identity without hash: %v", err)
	}
	if got, err := store.EmailPasswordHash(ctx, nullHashUser); err != nil || got != "" {
		t.Errorf("null hash: hash empty = %v, error = %v; want no hash and no error", got == "", err)
	}
	// With several email_password identities, the one with a hash decides.
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, 'email_password', 'another-null@example.com')`, passwordUser); err != nil {
		t.Fatalf("insert second email_password identity: %v", err)
	}
	if got, err := store.EmailPasswordHash(ctx, passwordUser); err != nil || got != hash {
		t.Errorf("several identities: hash matches = %v, error = %v; want the stored hash", got == hash, err)
	}
	if _, err := store.EmailPasswordHash(ctx, uuid.New()); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown user: error = %v, want pgx.ErrNoRows", err)
	}
}

func TestAuthStore_UserIsAdmin(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	trainer := registerPasswordUser(t, store, "not-admin@example.com")
	admin := registerPasswordUser(t, store, "admin-check@example.com")
	promoteToAdminOnly(t, pool, admin)

	for name, tt := range map[string]struct {
		id   uuid.UUID
		want bool
	}{"trainer": {trainer, false}, "admin": {admin, true}, "unknown user": {uuid.New(), false}} {
		got, err := store.UserIsAdmin(ctx, tt.id)
		if err != nil || got != tt.want {
			t.Errorf("%s: UserIsAdmin() = %v, %v; want %v", name, got, err, tt.want)
		}
	}
}
