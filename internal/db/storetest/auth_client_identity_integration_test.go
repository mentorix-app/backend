//go:build integration

package storetest

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

func TestAuthStore_FindOrCreateClientByIdentity(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	id := auth.ClientIdentity{Provider: auth.ProviderGoogle, Subject: "google-sub-1", Email: "client@test.com", DisplayName: "Client"}
	first, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	profile, err := store.UserProfile(ctx, first)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "client@test.com" || profile.Name != "Client" {
		t.Errorf("profile = %+v", profile)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleClient {
		t.Errorf("roles = %v, want [%q]", profile.Roles, auth.RoleClient)
	}

	second, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("second sign-in: %v", err)
	}
	if second != first {
		t.Errorf("second sign-in user = %v, want %v", second, first)
	}
	assertUserAndIdentityCounts(t, pool, id.Subject, 1)
}

func TestAuthStore_FindOrCreateClientByIdentity_keepsNameOfExistingAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	id := auth.ClientIdentity{Provider: auth.ProviderApple, Subject: "apple-sub-1", DisplayName: "First Name"}
	first, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	id.DisplayName = "Another Name"
	second, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("second sign-in: %v", err)
	}
	if second != first {
		t.Fatalf("second sign-in user = %v, want %v", second, first)
	}
	profile, err := store.UserProfile(ctx, first)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Name != "First Name" {
		t.Errorf("name = %q, want it unchanged", profile.Name)
	}
}

func TestAuthStore_FindOrCreateClientByIdentity_withoutEmail(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID, err := store.FindOrCreateClientByIdentity(ctx, auth.ClientIdentity{Provider: auth.ProviderGoogle, Subject: "google-sub-2"})
	if err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != "" {
		t.Errorf("email = %q, want empty", profile.Email)
	}
}

func TestAuthStore_FindOrCreateClientByIdentity_parallelFirstSignIn(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	const workers = 8
	id := auth.ClientIdentity{Provider: auth.ProviderGoogle, Subject: "google-sub-race", Email: "race@test.com", DisplayName: "Race"}
	ids := make([]uuid.UUID, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ids[i], errs[i] = store.FindOrCreateClientByIdentity(ctx, id)
		}()
	}
	close(start)
	wg.Wait()

	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("worker %d user = %v, want %v", i, ids[i], ids[0])
		}
	}
	assertUserAndIdentityCounts(t, pool, id.Subject, 1)
}

// assertUserAndIdentityCounts relies on NewPool giving each test an empty schema.
func assertUserAndIdentityCounts(t *testing.T, pool *pgxpool.Pool, subject string, want int) {
	t.Helper()
	var users, identities int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM mentorix.users`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.auth_identities WHERE provider = $1 AND subject = $2`,
		auth.ProviderGoogle, subject).Scan(&identities); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if users != want || identities != want {
		t.Errorf("users = %d, identities = %d, want %d each", users, identities, want)
	}
}
