package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeAuthStore struct {
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
	clientUserID         uuid.UUID
	clientErr            error
	clientCalls          []ClientIdentity
	clientCreated        bool
	fillCalls            []fillDisplayNameCall
	fillErr              error
	linkRemaining        uuid.UUID
	linkErr              error
	linkCalls            []linkTelegramCall
	deletedTelegramID    string
	deleteErr            error
	deleteCalls          []uuid.UUID
	deleteSubjects       []string
	deleteContexts       []deleteContextState
	roles                []string
	rolesErr             error
	appleSubjects        []string
	appleSubjectsErr     error
	activation           storeActivation
	businessData         bool
	businessDataErr      error
	businessDataCalls    int
	insertedExpiry       time.Time
	rotatedExpiry        time.Time
}

type fillDisplayNameCall struct {
	userID uuid.UUID
	name   string
}

type linkTelegramCall struct {
	appUserID      uuid.UUID
	telegramUserID string
}

func (f *fakeAuthStore) LinkTelegram(_ context.Context, appUserID uuid.UUID, telegramUserID string) (uuid.UUID, error) {
	f.linkCalls = append(f.linkCalls, linkTelegramCall{appUserID, telegramUserID})
	if f.linkErr != nil {
		return uuid.Nil, f.linkErr
	}
	return f.linkRemaining, nil
}

func (f *fakeAuthStore) FindOrCreateClientByIdentity(_ context.Context, id ClientIdentity) (uuid.UUID, bool, error) {
	f.clientCalls = append(f.clientCalls, id)
	if f.clientErr != nil {
		return uuid.Nil, false, f.clientErr
	}
	if f.clientUserID == uuid.Nil {
		f.clientUserID = uuid.New()
	}
	return f.clientUserID, f.clientCreated, nil
}

// FillUserDisplayNameIfEmpty mirrors the SQL guard: only a blank stored name is replaced.
func (f *fakeAuthStore) FillUserDisplayNameIfEmpty(_ context.Context, userID uuid.UUID, name string) (bool, error) {
	f.fillCalls = append(f.fillCalls, fillDisplayNameCall{userID, name})
	if f.fillErr != nil {
		return false, f.fillErr
	}
	if strings.TrimSpace(f.profile.Name) != "" {
		return false, nil
	}
	f.profile.Name = name
	return true, nil
}

func (f *fakeAuthStore) InsertRefreshSession(_ context.Context, _ uuid.UUID, _ []byte, expiresAt time.Time) error {
	f.insertedExpiry = expiresAt
	return f.insertSession
}

func (f *fakeAuthStore) RotateRefreshSession(_ context.Context, _, _ []byte, newExpiresAt time.Time) (uuid.UUID, string, error) {
	f.rotatedExpiry = newExpiresAt
	if f.rotateErr != nil {
		return uuid.Nil, "", f.rotateErr
	}
	// The email is read inside the rotation transaction: a failure rolls it back.
	if f.primaryErr != nil {
		return uuid.Nil, "", f.primaryErr
	}
	email := f.primaryEmail
	if email == "" {
		email = "user@test.com"
	}
	if f.rotateUserID == uuid.Nil {
		f.rotateUserID = uuid.New()
	}
	return f.rotateUserID, email, nil
}

func (f *fakeAuthStore) RevokeRefreshSession(context.Context, []byte) error {
	return f.revokeErr
}

func (f *fakeAuthStore) RevokeAllUserRefreshSessions(context.Context, uuid.UUID) error {
	return f.revokeAllErr
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

func TestService_Refresh_rotationErrorIsPropagatedWithoutTokens(t *testing.T) {
	store := &fakeAuthStore{primaryErr: errors.New("db down")}
	svc := testAuthService(store)
	issued, err := svc.Refresh(context.Background(), "refresh-plain-token")
	if !errors.Is(err, store.primaryErr) {
		t.Fatalf("Refresh() error = %v, want %v", err, store.primaryErr)
	}
	if issued.AccessToken != "" || issued.RefreshToken != "" {
		t.Error("Refresh() issued tokens although rotation failed")
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

func TestNormalizeEmail(t *testing.T) {
	got := NormalizeEmail("  User@Example.COM ")
	if got != "user@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
}
