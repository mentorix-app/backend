//go:build integration

package storetest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

func TestAuthStore_FindOrCreateClientByIdentity(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	id := auth.ClientIdentity{Provider: auth.ProviderGoogle, Subject: "google-sub-1", Email: "client@test.com", DisplayName: "Client"}
	first, created, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	profile, err := store.UserProfile(ctx, first)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if !created {
		t.Error("first sign-in: created = false, want true")
	}
	if profile.Email != "client@test.com" || profile.Name != "Client" {
		t.Errorf("profile = %+v", profile)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleClient {
		t.Errorf("roles = %v, want [%q]", profile.Roles, auth.RoleClient)
	}

	second, createdAgain, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("second sign-in: %v", err)
	}
	if second != first {
		t.Errorf("second sign-in user = %v, want %v", second, first)
	}
	if createdAgain {
		t.Error("second sign-in: created = true, want false")
	}
	assertUserAndIdentityCounts(t, pool, id.Subject, 1)
}

func TestAuthStore_FindOrCreateClientByIdentity_keepsNameOfExistingAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	id := auth.ClientIdentity{Provider: auth.ProviderApple, Subject: "apple-sub-1", DisplayName: "First Name"}
	first, created, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	id.DisplayName = "Another Name"
	second, createdAgain, err := store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		t.Fatalf("second sign-in: %v", err)
	}
	if second != first {
		t.Fatalf("second sign-in user = %v, want %v", second, first)
	}
	if !created || createdAgain {
		t.Errorf("created = %v, then %v; want true, then false", created, createdAgain)
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

	userID, _, err := store.FindOrCreateClientByIdentity(ctx, auth.ClientIdentity{Provider: auth.ProviderGoogle, Subject: "google-sub-2"})
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
	created := make([]bool, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ids[i], created[i], errs[i] = store.FindOrCreateClientByIdentity(ctx, id)
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
	creators := 0
	for _, c := range created {
		if c {
			creators++
		}
	}
	if creators != 1 {
		t.Errorf("%d workers reported creating the account, want 1", creators)
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

func TestAuthStore_FillUserDisplayNameIfEmpty(t *testing.T) {
	tests := []struct {
		name       string
		stored     string
		fill       string
		wantName   string
		wantFilled bool
	}{
		{"empty name is filled", "", "Filled Name", "Filled Name", true},
		{"set name is kept", "Chosen Name", "Filled Name", "Chosen Name", false},
		{"name of spaces counts as empty", "   ", "Filled Name", "Filled Name", true},
		{"empty argument changes nothing", "Chosen Name", "", "Chosen Name", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			ctx := context.Background()

			userID, _, err := store.FindOrCreateClientByIdentity(ctx, auth.ClientIdentity{
				Provider: auth.ProviderApple, Subject: "apple-fill", DisplayName: tt.stored,
			})
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			filled, err := store.FillUserDisplayNameIfEmpty(ctx, userID, tt.fill)
			if err != nil {
				t.Fatalf("FillUserDisplayNameIfEmpty() error = %v", err)
			}
			if filled != tt.wantFilled {
				t.Errorf("filled = %v, want %v", filled, tt.wantFilled)
			}
			var got string
			if err := pool.QueryRow(ctx, `SELECT display_name FROM mentorix.users WHERE id = $1`, userID).Scan(&got); err != nil {
				t.Fatalf("read display_name: %v", err)
			}
			if got != tt.wantName {
				t.Errorf("display_name = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestAuthStore_FillUserDisplayNameIfEmpty_leavesOtherUsersAlone(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	target, _, err := store.FindOrCreateClientByIdentity(ctx, auth.ClientIdentity{Provider: auth.ProviderApple, Subject: "apple-target"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	other, _, err := store.FindOrCreateClientByIdentity(ctx, auth.ClientIdentity{Provider: auth.ProviderApple, Subject: "apple-other"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := store.FillUserDisplayNameIfEmpty(ctx, target, "Target Name"); err != nil {
		t.Fatalf("FillUserDisplayNameIfEmpty() error = %v", err)
	}
	profile, err := store.UserProfile(ctx, other)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Name != "" {
		t.Errorf("other user name = %q, want it still empty", profile.Name)
	}
	profile, err = store.UserProfile(ctx, target)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Name != "Target Name" {
		t.Errorf("target name = %q, want %q", profile.Name, "Target Name")
	}
}

// signInName signs in through the real service and returns the stored display name.
// requestName is the Apple request name; Google ignores it.
func signInName(t *testing.T, pool *pgxpool.Pool, provider string, claims auth.IDTokenClaims, requestName string) string {
	t.Helper()
	ctx := context.Background()
	verifier := scriptedVerifier{claims: claims}
	svc := auth.NewService(pool, "integration-test-jwt-secret-32chars", 15*time.Minute, 30*24*time.Hour,
		auth.WithGoogleVerifier(verifier), auth.WithAppleVerifier(verifier))
	var (
		issued auth.IssuedAuth
		err    error
	)
	if provider == auth.ProviderApple {
		issued, err = svc.SignInWithApple(ctx, "raw", requestName, "")
	} else {
		issued, err = svc.SignInWithGoogle(ctx, "raw", "")
	}
	if err != nil {
		t.Fatalf("sign-in with %s: %v", provider, err)
	}
	profile, err := auth.NewStore(pool).UserProfile(ctx, issued.UserID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	return profile.Name
}

func TestAuthService_SignIn_fillsEmptyNameOnce(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		// claimsWith builds the token claims. Apple takes the name from the request instead.
		claimsWith func(name string) auth.IDTokenClaims
	}{
		{
			name:       "apple name in the request",
			provider:   auth.ProviderApple,
			claimsWith: func(string) auth.IDTokenClaims { return auth.IDTokenClaims{Subject: "apple-flow"} },
		},
		{
			name:       "google name in the token",
			provider:   auth.ProviderGoogle,
			claimsWith: func(name string) auth.IDTokenClaims { return auth.IDTokenClaims{Subject: "google-flow", Name: name} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			signIn := func(name string) string {
				return signInName(t, pool, tt.provider, tt.claimsWith(name), name)
			}

			if got := signIn(""); got != "" {
				t.Fatalf("first sign-in without a name: name = %q, want empty", got)
			}
			if got := signIn("First Name"); got != "First Name" {
				t.Errorf("second sign-in with a name: name = %q, want %q", got, "First Name")
			}
			if got := signIn("Second Name"); got != "First Name" {
				t.Errorf("third sign-in with another name: name = %q, want %q kept", got, "First Name")
			}
		})
	}
}
