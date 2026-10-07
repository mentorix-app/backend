package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeAuthStore struct {
	registerUserID       uuid.UUID
	registerErr          error
	identity             emailIdentityRow
	identityErr          error
	insertSession        error
	rotateUserID         uuid.UUID
	rotateErr            error
	rotateGrace          time.Duration
	primaryEmail         string
	primaryErr           error
	profile              UserProfile
	profileErr           error
	updateDisplayNameErr error
	revokeErr            error
	revokeAllErr         error

	socialUserID  uuid.UUID
	socialCreated bool
	socialErr     error
	socialCalls   []socialSignInCall
	addRoleErr    error
	addRoleCalls  []addRoleCall
	attachErr     error
	attachCalls   []attachIdentityCall
	passwordHash  string
	passwordErr   error
	isAdmin       bool
	adminErr      error
}

type attachIdentityCall struct {
	userID   uuid.UUID
	provider string
	subject  string
}

type socialSignInCall struct {
	provider    string
	claims      IDTokenClaims
	displayName string
}

type addRoleCall struct {
	userID uuid.UUID
	role   string
}

func (f *fakeAuthStore) SocialSignIn(_ context.Context, provider string, claims IDTokenClaims, displayName string) (uuid.UUID, bool, error) {
	f.socialCalls = append(f.socialCalls, socialSignInCall{provider: provider, claims: claims, displayName: displayName})
	if f.socialErr != nil {
		return uuid.Nil, false, f.socialErr
	}
	if f.socialUserID == uuid.Nil {
		f.socialUserID = uuid.New()
	}
	return f.socialUserID, f.socialCreated, nil
}

func (f *fakeAuthStore) AddRole(_ context.Context, userID uuid.UUID, role string) error {
	f.addRoleCalls = append(f.addRoleCalls, addRoleCall{userID: userID, role: role})
	return f.addRoleErr
}

func (f *fakeAuthStore) AttachIdentity(_ context.Context, userID uuid.UUID, provider, subject string) error {
	f.attachCalls = append(f.attachCalls, attachIdentityCall{userID: userID, provider: provider, subject: subject})
	return f.attachErr
}

func (f *fakeAuthStore) EmailPasswordHash(context.Context, uuid.UUID) (string, error) {
	return f.passwordHash, f.passwordErr
}

func (f *fakeAuthStore) UserIsAdmin(context.Context, uuid.UUID) (bool, error) {
	return f.isAdmin, f.adminErr
}

func (f *fakeAuthStore) RegisterTrainerEmailPassword(_ context.Context, _, _, _ string) (uuid.UUID, error) {
	if f.registerErr != nil {
		return uuid.Nil, f.registerErr
	}
	if f.registerUserID == uuid.Nil {
		f.registerUserID = uuid.New()
	}
	return f.registerUserID, nil
}

func (f *fakeAuthStore) getEmailPasswordIdentity(_ context.Context, _ string) (emailIdentityRow, error) {
	return f.identity, f.identityErr
}

func (f *fakeAuthStore) InsertRefreshSession(context.Context, uuid.UUID, []byte, time.Time) error {
	return f.insertSession
}

func (f *fakeAuthStore) RotateRefreshSession(_ context.Context, _, _ []byte, _ time.Time, grace time.Duration) (uuid.UUID, error) {
	f.rotateGrace = grace
	if f.rotateErr != nil {
		return uuid.Nil, f.rotateErr
	}
	if f.rotateUserID == uuid.Nil {
		f.rotateUserID = uuid.New()
	}
	return f.rotateUserID, nil
}

func (f *fakeAuthStore) RevokeRefreshSession(context.Context, []byte) error {
	return f.revokeErr
}

func (f *fakeAuthStore) RevokeAllUserRefreshSessions(context.Context, uuid.UUID) error {
	return f.revokeAllErr
}

func (f *fakeAuthStore) UserPrimaryEmail(context.Context, uuid.UUID) (string, error) {
	if f.primaryErr != nil {
		return "", f.primaryErr
	}
	if f.primaryEmail == "" {
		return "user@test.com", nil
	}
	return f.primaryEmail, nil
}

func (f *fakeAuthStore) UserProfile(context.Context, uuid.UUID) (UserProfile, error) {
	return f.profile, f.profileErr
}

func (f *fakeAuthStore) UpdateUserDisplayName(_ context.Context, _ uuid.UUID, name string) error {
	if f.updateDisplayNameErr != nil {
		return f.updateDisplayNameErr
	}
	f.profile.Name = name
	return nil
}

func testAuthService(store authStore) *Service {
	return &Service{
		store:      store,
		jwtSecret:  []byte("test-jwt-secret-at-least-32-chars"),
		accessTTL:  15 * time.Minute,
		refreshTTL: 30 * 24 * time.Hour,
	}
}

func TestService_RegisterTrainer(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	issued, err := svc.RegisterTrainer(context.Background(), "trainer@test.com", "password123", "Trainer")
	if err != nil {
		t.Fatalf("RegisterTrainer() error = %v", err)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" {
		t.Fatal("expected access and refresh tokens")
	}
	if issued.Email != "trainer@test.com" {
		t.Errorf("email = %q", issued.Email)
	}
}

func TestService_RegisterTrainer_weakPassword(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	_, err := svc.RegisterTrainer(context.Background(), "a@b.com", "short", "")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestService_Login_success(t *testing.T) {
	userID := uuid.New()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	svc := testAuthService(&fakeAuthStore{
		identity: emailIdentityRow{UserID: userID, PasswordHash: hash},
	})
	issued, err := svc.Login(context.Background(), "trainer@test.com", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if issued.UserID != userID {
		t.Errorf("user id = %v, want %v", issued.UserID, userID)
	}
}

func TestService_Login_invalidCredentials(t *testing.T) {
	tests := []struct {
		name    string
		store   *fakeAuthStore
		wantErr error
	}{
		{
			name:    "not found",
			store:   &fakeAuthStore{identityErr: pgx.ErrNoRows},
			wantErr: ErrInvalidCredentials,
		},
		{
			name:    "empty hash",
			store:   &fakeAuthStore{identity: emailIdentityRow{UserID: uuid.New()}},
			wantErr: ErrInvalidCredentials,
		},
		{
			name: "wrong password",
			store: &fakeAuthStore{
				identity: emailIdentityRow{
					UserID:       uuid.New(),
					PasswordHash: mustHash(t, "password123"),
				},
			},
			wantErr: ErrInvalidCredentials,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := testAuthService(tt.store)
			password := "wrong"
			if tt.name == "wrong password" {
				password = "other"
			}
			_, err := svc.Login(context.Background(), "trainer@test.com", password)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Login() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_Refresh(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{rotateUserID: uuid.New()})
	issued, err := svc.Refresh(context.Background(), "refresh-plain-token")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" {
		t.Fatal("expected tokens")
	}
}

func TestService_Refresh_empty(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	_, err := svc.Refresh(context.Background(), "")
	if !errors.Is(err, ErrInvalidRefresh) {
		t.Errorf("Refresh() error = %v, want ErrInvalidRefresh", err)
	}
}

func TestService_Refresh_invalid(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{rotateErr: ErrInvalidRefresh})
	_, err := svc.Refresh(context.Background(), "stale")
	if !errors.Is(err, ErrInvalidRefresh) {
		t.Errorf("Refresh() error = %v, want ErrInvalidRefresh", err)
	}
}

func TestService_Refresh_reusePassesThroughAsInvalid(t *testing.T) {
	familyID := uuid.New()
	reuse := &RefreshReuseError{FamilyID: familyID, UserID: uuid.New(), Revoked: 1}
	svc := testAuthService(&fakeAuthStore{rotateErr: reuse})
	_, err := svc.Refresh(context.Background(), "spent")
	if !errors.Is(err, ErrRefreshTokenReused) {
		t.Errorf("Refresh() error = %v, want ErrRefreshTokenReused", err)
	}
	if !errors.Is(err, ErrInvalidRefresh) {
		t.Errorf("Refresh() error = %v, want it to satisfy ErrInvalidRefresh", err)
	}
	if !strings.Contains(err.Error(), familyID.String()) {
		t.Error("Refresh() dropped the family id from the error")
	}
}

func TestService_Refresh_graceReachesStore(t *testing.T) {
	tests := []struct {
		name string
		opts []ServiceOption
		want time.Duration
	}{
		{"default", nil, DefaultRefreshReuseGrace},
		{"option", []ServiceOption{WithRefreshReuseGrace(5 * time.Second)}, 5 * time.Second},
		{"negative is clamped to zero", []ServiceOption{WithRefreshReuseGrace(-time.Second)}, 0},
		{"zero disables the retry", []ServiceOption{WithRefreshReuseGrace(0)}, 0},
		{"upper bound is kept", []ServiceOption{WithRefreshReuseGrace(MaxRefreshReuseGrace)}, MaxRefreshReuseGrace},
		{"too large is clamped to the maximum", []ServiceOption{WithRefreshReuseGrace(time.Hour)}, MaxRefreshReuseGrace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := NewService(nil, "test-jwt-secret-at-least-32-chars", time.Minute, time.Hour, tt.opts...)
			svc.store = store
			if _, err := svc.Refresh(context.Background(), "token"); err != nil {
				t.Fatalf("Refresh() error = %v", err)
			}
			if store.rotateGrace != tt.want {
				t.Errorf("grace = %v, want %v", store.rotateGrace, tt.want)
			}
		})
	}
}

func TestRefreshReuseError_textForTheLog(t *testing.T) {
	familyID, userID := uuid.New(), uuid.New()
	err := &RefreshReuseError{FamilyID: familyID, UserID: userID, Revoked: 2}
	for _, want := range []string{"refresh_token_reuse", "family=" + familyID.String(), "user=" + userID.String(), "revoked=2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Error() = %q, missing %q", err.Error(), want)
		}
	}
	if !errors.Is(err, ErrRefreshTokenReused) || !errors.Is(err, ErrInvalidRefresh) {
		t.Error("reuse error does not satisfy ErrRefreshTokenReused and ErrInvalidRefresh")
	}
	if got := (&RefreshReuseError{FamilyID: familyID, UserID: userID}).Error(); !strings.Contains(got, "revoked=0") {
		t.Errorf("Error() = %q, want revoked=0", got)
	}
}

func TestDefaultRefreshReuseGrace(t *testing.T) {
	if DefaultRefreshReuseGrace != 30*time.Second {
		t.Errorf("DefaultRefreshReuseGrace = %v, want 30s", DefaultRefreshReuseGrace)
	}
}

func TestService_Logout(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	if err := svc.Logout(context.Background(), ""); err != nil {
		t.Errorf("Logout empty: %v", err)
	}
	if err := svc.Logout(context.Background(), "token"); err != nil {
		t.Errorf("Logout: %v", err)
	}
}

func TestService_Logout_revokeError(t *testing.T) {
	want := errors.New("revoke failed")
	svc := testAuthService(&fakeAuthStore{revokeErr: want})
	if err := svc.Logout(context.Background(), "token"); !errors.Is(err, want) {
		t.Fatalf("Logout() error = %v, want %v", err, want)
	}
}

func TestService_LogoutAll(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	if err := svc.LogoutAll(context.Background(), uuid.New()); err != nil {
		t.Errorf("LogoutAll: %v", err)
	}
}

func TestService_LogoutAll_revokeError(t *testing.T) {
	want := errors.New("revoke all failed")
	svc := testAuthService(&fakeAuthStore{revokeAllErr: want})
	if err := svc.LogoutAll(context.Background(), uuid.New()); !errors.Is(err, want) {
		t.Fatalf("LogoutAll() error = %v, want %v", err, want)
	}
}

func TestService_UserProfile(t *testing.T) {
	want := UserProfile{Email: "a@b.com", Roles: []string{RoleTrainer}}
	svc := testAuthService(&fakeAuthStore{profile: want})
	got, err := svc.UserProfile(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if got.Email != want.Email {
		t.Errorf("email = %q, want %q", got.Email, want.Email)
	}
}

func TestService_UserPrimaryEmail(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{primaryEmail: "primary@test.com"})
	email, err := svc.UserPrimaryEmail(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("UserPrimaryEmail() error = %v", err)
	}
	if email != "primary@test.com" {
		t.Errorf("email = %q", email)
	}
}

func TestService_UserPrimaryEmail_error(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{primaryErr: errors.New("db down")})
	_, err := svc.UserPrimaryEmail(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_UpdateProfileName(t *testing.T) {
	want := UserProfile{Email: "a@b.com", Roles: []string{RoleTrainer}}
	svc := testAuthService(&fakeAuthStore{profile: want})
	got, err := svc.UpdateProfileName(context.Background(), uuid.New(), "  Coach  ")
	if err != nil {
		t.Fatalf("UpdateProfileName() error = %v", err)
	}
	if got.Name != "Coach" {
		t.Errorf("name = %q, want Coach", got.Name)
	}
}

func TestService_UpdateProfileName_updateError(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{updateDisplayNameErr: pgx.ErrNoRows})
	_, err := svc.UpdateProfileName(context.Background(), uuid.New(), "Coach")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("UpdateProfileName() error = %v, want ErrNoRows", err)
	}
}

func TestService_UpdateProfileName_profileError(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{profileErr: errors.New("db down")})
	_, err := svc.UpdateProfileName(context.Background(), uuid.New(), "Coach")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_RegisterTrainer_storeError(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{registerErr: errors.New("db down")})
	_, err := svc.RegisterTrainer(context.Background(), "trainer@test.com", "password123", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_RegisterTrainer_insertSessionError(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{insertSession: errors.New("db down")})
	_, err := svc.RegisterTrainer(context.Background(), "trainer@test.com", "password123", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_Login_insertSessionError(t *testing.T) {
	hash := mustHash(t, "password123")
	svc := testAuthService(&fakeAuthStore{
		identity:      emailIdentityRow{UserID: uuid.New(), PasswordHash: hash},
		insertSession: errors.New("db down"),
	})
	_, err := svc.Login(context.Background(), "trainer@test.com", "password123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_Login_identityLoadError(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{identityErr: errors.New("db down")})
	_, err := svc.Login(context.Background(), "trainer@test.com", "password123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	h, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return h
}

type fakeIDTokenVerifier struct {
	claims IDTokenClaims
	err    error
	tokens []string
}

func (f *fakeIDTokenVerifier) Verify(_ context.Context, rawToken string) (IDTokenClaims, error) {
	f.tokens = append(f.tokens, rawToken)
	return f.claims, f.err
}

func socialTestService(store authStore, verifiers map[string]IDTokenVerifier) *Service {
	svc := testAuthService(store)
	svc.verifiers = verifiers
	return svc
}

func TestService_SocialLogin_success(t *testing.T) {
	tests := []struct {
		name        string
		store       *fakeAuthStore
		wantCreated bool
	}{
		{name: "known identity", store: &fakeAuthStore{primaryEmail: "known@test.com"}, wantCreated: false},
		{name: "new identity", store: &fakeAuthStore{socialCreated: true, primaryEmail: "new@test.com"}, wantCreated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub-1", Email: "x@test.com", EmailVerified: true}}
			svc := socialTestService(tt.store, map[string]IDTokenVerifier{ProviderGoogle: ver})
			issued, created, err := svc.SocialLogin(context.Background(), ProviderGoogle, "raw-token", "")
			if err != nil {
				t.Fatalf("SocialLogin() error = %v", err)
			}
			if created != tt.wantCreated {
				t.Errorf("created = %v, want %v", created, tt.wantCreated)
			}
			if issued.AccessToken == "" || issued.RefreshToken == "" {
				t.Fatal("expected access and refresh tokens")
			}
			if issued.UserID != tt.store.socialUserID {
				t.Errorf("user id = %v, want %v", issued.UserID, tt.store.socialUserID)
			}
			if issued.Email != tt.store.primaryEmail {
				t.Errorf("email = %q, want primary email %q", issued.Email, tt.store.primaryEmail)
			}
			if len(ver.tokens) != 1 || ver.tokens[0] != "raw-token" {
				t.Errorf("verifier tokens = %v", ver.tokens)
			}
		})
	}
}

func TestService_SocialLogin_emailEmptyWhenUserHasNone(t *testing.T) {
	store := &fakeAuthStore{socialCreated: true}
	svc := socialTestService(&noEmailStore{fakeAuthStore: store}, map[string]IDTokenVerifier{
		ProviderApple: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub-1"}},
	})
	issued, _, err := svc.SocialLogin(context.Background(), ProviderApple, "raw", "")
	if err != nil {
		t.Fatalf("SocialLogin() error = %v", err)
	}
	if issued.Email != "" {
		t.Errorf("email = %q, want empty", issued.Email)
	}
}

// noEmailStore reports a user without a primary email.
type noEmailStore struct{ *fakeAuthStore }

func (noEmailStore) UserPrimaryEmail(context.Context, uuid.UUID) (string, error) { return "", nil }

func TestService_SocialLogin_passesClaimsAndName(t *testing.T) {
	claims := IDTokenClaims{Subject: "sub-1", Email: "x@test.com", EmailVerified: true, Name: "Token Name"}
	tests := []struct {
		name     string
		given    string
		wantName string
	}{
		{name: "request name wins and is trimmed", given: "  Given Name ", wantName: "Given Name"},
		{name: "token name is the fallback", given: "   ", wantName: "Token Name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := socialTestService(store, map[string]IDTokenVerifier{ProviderApple: &fakeIDTokenVerifier{claims: claims}})
			if _, _, err := svc.SocialLogin(context.Background(), ProviderApple, "raw", tt.given); err != nil {
				t.Fatalf("SocialLogin() error = %v", err)
			}
			if len(store.socialCalls) != 1 {
				t.Fatalf("store calls = %d, want 1", len(store.socialCalls))
			}
			got := store.socialCalls[0]
			if got.provider != ProviderApple || got.claims != claims || got.displayName != tt.wantName {
				t.Errorf("store call = %+v", got)
			}
		})
	}
}

func TestService_SocialLogin_errors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name         string
		provider     string
		verifiers    map[string]IDTokenVerifier
		store        *fakeAuthStore
		wantErr      error
		wantStoreHit bool
	}{
		{
			name:      "provider not configured",
			provider:  ProviderApple,
			verifiers: map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{}},
			store:     &fakeAuthStore{},
			wantErr:   ErrProviderNotConfigured,
		},
		{
			name:     "no verifiers at all",
			provider: ProviderGoogle,
			store:    &fakeAuthStore{},
			wantErr:  ErrProviderNotConfigured,
		},
		{
			name:      "unknown provider",
			provider:  "facebook",
			verifiers: map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{}},
			store:     &fakeAuthStore{},
			wantErr:   ErrProviderNotConfigured,
		},
		{
			name:      "provider unavailable",
			provider:  ProviderGoogle,
			verifiers: map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{err: ErrIDTokenProviderUnavailable}},
			store:     &fakeAuthStore{},
			wantErr:   ErrIDTokenProviderUnavailable,
		},
		{
			name:      "invalid token",
			provider:  ProviderGoogle,
			verifiers: map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{err: ErrInvalidIDToken}},
			store:     &fakeAuthStore{},
			wantErr:   ErrInvalidIDToken,
		},
		{
			name:         "email belongs to another account",
			provider:     ProviderGoogle,
			verifiers:    map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}}},
			store:        &fakeAuthStore{socialErr: ErrEmailBelongsToAnotherAccount},
			wantErr:      ErrEmailBelongsToAnotherAccount,
			wantStoreHit: true,
		},
		{
			name:         "store failure",
			provider:     ProviderGoogle,
			verifiers:    map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}}},
			store:        &fakeAuthStore{socialErr: boom},
			wantErr:      boom,
			wantStoreHit: true,
		},
		{
			name:         "session insert failure",
			provider:     ProviderGoogle,
			verifiers:    map[string]IDTokenVerifier{ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}}},
			store:        &fakeAuthStore{insertSession: boom},
			wantErr:      boom,
			wantStoreHit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := socialTestService(tt.store, tt.verifiers)
			_, _, err := svc.SocialLogin(context.Background(), tt.provider, "raw", "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SocialLogin() error = %v, want %v", err, tt.wantErr)
			}
			if hit := len(tt.store.socialCalls) > 0; hit != tt.wantStoreHit {
				t.Errorf("store called = %v, want %v", hit, tt.wantStoreHit)
			}
		})
	}
}

func TestService_AddRole(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{}
	svc := testAuthService(store)
	if err := svc.AddRole(context.Background(), userID, RoleTrainer); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}
	if len(store.addRoleCalls) != 1 || store.addRoleCalls[0] != (addRoleCall{userID: userID, role: RoleTrainer}) {
		t.Errorf("store calls = %+v", store.addRoleCalls)
	}

	store.addRoleErr = ErrRoleConflict
	if err := svc.AddRole(context.Background(), userID, RoleClient); !errors.Is(err, ErrRoleConflict) {
		t.Errorf("AddRole() error = %v, want ErrRoleConflict", err)
	}
}

func TestNewService_withIDTokenVerifiers(t *testing.T) {
	ver := &fakeIDTokenVerifier{}
	svc := NewService(nil, "secret", time.Minute, time.Hour, WithIDTokenVerifiers(map[string]IDTokenVerifier{ProviderApple: ver}))
	if svc.verifiers[ProviderApple] != ver {
		t.Fatal("verifier was not registered")
	}
}

func TestService_SocialLogin_truncatesTokenNameToLimit(t *testing.T) {
	long := strings.Repeat("Ж", maxSocialNameRunes+50)
	store := &fakeAuthStore{}
	svc := socialTestService(store, map[string]IDTokenVerifier{
		ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "  " + long + "  "}},
	})
	if _, _, err := svc.SocialLogin(context.Background(), ProviderGoogle, "raw", ""); err != nil {
		t.Fatalf("SocialLogin() error = %v", err)
	}
	got := store.socialCalls[0].displayName
	if n := utf8.RuneCountInString(got); n != maxSocialNameRunes {
		t.Errorf("display name has %d runes, want %d", n, maxSocialNameRunes)
	}
	if !utf8.ValidString(got) {
		t.Error("truncation split a multibyte rune")
	}
}

func TestService_SocialLogin_shortTokenNameIsKept(t *testing.T) {
	store := &fakeAuthStore{}
	svc := socialTestService(store, map[string]IDTokenVerifier{
		ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "Анна"}},
	})
	if _, _, err := svc.SocialLogin(context.Background(), ProviderGoogle, "raw", ""); err != nil {
		t.Fatalf("SocialLogin() error = %v", err)
	}
	if got := store.socialCalls[0].displayName; got != "Анна" {
		t.Errorf("display name = %q", got)
	}
}

func TestService_SocialLogin_dropsUnusableTokenName(t *testing.T) {
	for name, tokenName := range map[string]string{
		"NUL rune":      "Ann\x00a",
		"invalid UTF-8": "An\xffna",
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := socialTestService(store, map[string]IDTokenVerifier{
				ProviderGoogle: &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: tokenName}},
			})
			if _, _, err := svc.SocialLogin(context.Background(), ProviderGoogle, "raw", ""); err != nil {
				t.Fatalf("SocialLogin() error = %v", err)
			}
			if got := store.socialCalls[0].displayName; got != "" {
				t.Errorf("display name = %q, want empty", got)
			}
		})
	}
}

func TestUsableName(t *testing.T) {
	tests := map[string]bool{
		"":         true,
		"Anna":     true,
		"Анна 😀":   true,
		"An\x00na": false,
		"\x00":     false,
		"An\xffna": false,
	}
	for in, want := range tests {
		if got := usableName(in); got != want {
			t.Errorf("usableName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestService_AttachIdentity_success(t *testing.T) {
	store := &fakeAuthStore{}
	ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub-attach", Email: "ignored@test.com", EmailVerified: true}}
	svc := socialTestService(store, map[string]IDTokenVerifier{ProviderApple: ver})
	userID := uuid.New()

	if err := svc.AttachIdentity(context.Background(), userID, ProviderApple, "raw-token", ""); err != nil {
		t.Fatalf("AttachIdentity() error = %v", err)
	}

	want := attachIdentityCall{userID: userID, provider: ProviderApple, subject: "sub-attach"}
	if len(store.attachCalls) != 1 || store.attachCalls[0] != want {
		t.Errorf("store calls = %+v, want [%+v]", store.attachCalls, want)
	}
	if len(ver.tokens) != 1 || ver.tokens[0] != "raw-token" {
		t.Errorf("verifier tokens = %v", ver.tokens)
	}
}

func TestService_AttachIdentity_errorsPassThrough(t *testing.T) {
	tests := []struct {
		name        string
		verifierErr error
		storeErr    error
		want        error
		wantStore   int
	}{
		{name: "invalid token", verifierErr: ErrInvalidIDToken, want: ErrInvalidIDToken},
		{name: "provider unavailable", verifierErr: ErrIDTokenProviderUnavailable, want: ErrIDTokenProviderUnavailable},
		{name: "identity of another account", storeErr: ErrIdentityBelongsToAnotherAccount, want: ErrIdentityBelongsToAnotherAccount, wantStore: 1},
		{name: "user not found", storeErr: pgx.ErrNoRows, want: pgx.ErrNoRows, wantStore: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{attachErr: tt.storeErr}
			ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}, err: tt.verifierErr}
			svc := socialTestService(store, map[string]IDTokenVerifier{ProviderGoogle: ver})

			err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "")

			if !errors.Is(err, tt.want) {
				t.Errorf("AttachIdentity() error = %v, want %v", err, tt.want)
			}
			if len(store.attachCalls) != tt.wantStore {
				t.Errorf("store calls = %d, want %d", len(store.attachCalls), tt.wantStore)
			}
		})
	}
}

func TestService_AttachIdentity_providerNotConfigured(t *testing.T) {
	for name, verifiers := range map[string]map[string]IDTokenVerifier{
		"missing":   {},
		"nil entry": {ProviderGoogle: nil},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := socialTestService(store, verifiers)

			err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "")

			if !errors.Is(err, ErrProviderNotConfigured) {
				t.Errorf("AttachIdentity() error = %v, want ErrProviderNotConfigured", err)
			}
			if len(store.attachCalls) != 0 {
				t.Errorf("store called for an unconfigured provider: %+v", store.attachCalls)
			}
		})
	}
}

func TestService_AttachIdentity_passwordStepUp(t *testing.T) {
	hash := mustHash(t, "correct-password")
	tests := []struct {
		name         string
		store        *fakeAuthStore
		password     string
		wantErr      error
		wantVerified bool
		wantAttached bool
	}{
		{name: "password account, correct password", store: &fakeAuthStore{passwordHash: hash}, password: "correct-password", wantVerified: true, wantAttached: true},
		{name: "password account, missing password", store: &fakeAuthStore{passwordHash: hash}, wantErr: ErrPasswordRequired},
		{name: "password account, wrong password", store: &fakeAuthStore{passwordHash: hash}, password: "wrong-password", wantErr: ErrPasswordIncorrect},
		{name: "social-only account, no password", store: &fakeAuthStore{}, wantVerified: true, wantAttached: true},
		{name: "social-only account, stray password is ignored", store: &fakeAuthStore{}, password: "anything", wantVerified: true, wantAttached: true},
		{name: "user not found", store: &fakeAuthStore{passwordErr: pgx.ErrNoRows}, password: "correct-password", wantErr: pgx.ErrNoRows},
		{name: "admin account", store: &fakeAuthStore{passwordHash: hash, isAdmin: true}, password: "correct-password", wantErr: ErrAdminCannotAttachIdentity},
		{name: "admin account without password hash", store: &fakeAuthStore{isAdmin: true}, wantErr: ErrAdminCannotAttachIdentity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}}
			svc := socialTestService(tt.store, map[string]IDTokenVerifier{ProviderGoogle: ver})

			err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", tt.password)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AttachIdentity() error = %v, want %v", err, tt.wantErr)
			}
			if verified := len(ver.tokens) > 0; verified != tt.wantVerified {
				t.Errorf("verifier called = %v, want %v", verified, tt.wantVerified)
			}
			if attached := len(tt.store.attachCalls) > 0; attached != tt.wantAttached {
				t.Errorf("store attach called = %v, want %v", attached, tt.wantAttached)
			}
			if err != nil && tt.password != "" && strings.Contains(err.Error(), tt.password) {
				t.Error("error text contains the password")
			}
		})
	}
}

func TestService_AttachIdentity_lookupFailuresStopBeforeVerifying(t *testing.T) {
	tests := []struct {
		name  string
		store *fakeAuthStore
	}{
		{name: "password lookup fails", store: &fakeAuthStore{passwordErr: errors.New("db down")}},
		{name: "admin lookup fails", store: &fakeAuthStore{adminErr: errors.New("db down")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}}
			svc := socialTestService(tt.store, map[string]IDTokenVerifier{ProviderGoogle: ver})

			err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "")

			if err == nil {
				t.Fatal("AttachIdentity() succeeded, want an error")
			}
			if len(ver.tokens) != 0 || len(tt.store.attachCalls) != 0 {
				t.Errorf("verifier calls = %d, attach calls = %d; want none", len(ver.tokens), len(tt.store.attachCalls))
			}
		})
	}
}

func TestService_AttachIdentity_providerNotConfiguredBeatsStoreLookups(t *testing.T) {
	store := &fakeAuthStore{passwordErr: errors.New("db down"), adminErr: errors.New("db down")}
	svc := socialTestService(store, map[string]IDTokenVerifier{})

	err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "")

	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("AttachIdentity() error = %v, want ErrProviderNotConfigured", err)
	}
}

func TestService_AttachIdentity_adminBeatsPasswordCheck(t *testing.T) {
	hash := mustHash(t, "correct-password")
	for name, password := range map[string]string{"missing": "", "wrong": "wrong-password"} {
		t.Run(name, func(t *testing.T) {
			store := &fakeAuthStore{passwordHash: hash, isAdmin: true}
			ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}}
			svc := socialTestService(store, map[string]IDTokenVerifier{ProviderGoogle: ver})

			err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", password)

			if !errors.Is(err, ErrAdminCannotAttachIdentity) {
				t.Errorf("AttachIdentity() error = %v, want ErrAdminCannotAttachIdentity", err)
			}
		})
	}
}

func TestService_AttachIdentity_malformedStoredHashFailsClosed(t *testing.T) {
	store := &fakeAuthStore{passwordHash: "not-an-argon2-hash"}
	ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}}
	svc := socialTestService(store, map[string]IDTokenVerifier{ProviderGoogle: ver})

	err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "some-password")

	if err == nil {
		t.Fatal("AttachIdentity() succeeded with a malformed stored hash")
	}
	if errors.Is(err, ErrPasswordIncorrect) || errors.Is(err, ErrPasswordRequired) {
		t.Errorf("error = %v, want an internal error", err)
	}
	if len(ver.tokens) != 0 || len(store.attachCalls) != 0 {
		t.Errorf("verifier calls = %d, attach calls = %d; want none", len(ver.tokens), len(store.attachCalls))
	}
}

func TestService_AttachIdentity_whitespaceOnlyPasswordIsIncorrect(t *testing.T) {
	store := &fakeAuthStore{passwordHash: mustHash(t, "correct-password")}
	ver := &fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub"}}
	svc := socialTestService(store, map[string]IDTokenVerifier{ProviderGoogle: ver})

	err := svc.AttachIdentity(context.Background(), uuid.New(), ProviderGoogle, "raw", "   ")

	if !errors.Is(err, ErrPasswordIncorrect) {
		t.Errorf("AttachIdentity() error = %v, want ErrPasswordIncorrect", err)
	}
	if len(ver.tokens) != 0 {
		t.Error("verifier called after a failed password check")
	}
}
