package auth

import (
	"context"
	"errors"
	"testing"
	"time"

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
	primaryEmail         string
	primaryErr           error
	profile              UserProfile
	profileErr           error
	updateDisplayNameErr error
	revokeErr            error
	revokeAllErr         error
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

func (f *fakeAuthStore) RotateRefreshSession(context.Context, []byte, []byte, time.Time) (uuid.UUID, error) {
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

func TestService_Logout(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	if err := svc.Logout(context.Background(), ""); err != nil {
		t.Errorf("Logout empty: %v", err)
	}
	if err := svc.Logout(context.Background(), "token"); err != nil {
		t.Errorf("Logout: %v", err)
	}
}

func TestService_LogoutAll(t *testing.T) {
	svc := testAuthService(&fakeAuthStore{})
	if err := svc.LogoutAll(context.Background(), uuid.New()); err != nil {
		t.Errorf("LogoutAll: %v", err)
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
