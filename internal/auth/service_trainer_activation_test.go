package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// storeActivation holds the ActivateTrainer calls the fake store saw and the error it answers with.
type storeActivation struct {
	err   error
	users []uuid.UUID
	terms []string
}

func (f *fakeAuthStore) ActivateTrainer(_ context.Context, userID uuid.UUID, termsVersion string) error {
	f.activation.users = append(f.activation.users, userID)
	f.activation.terms = append(f.activation.terms, termsVersion)
	return f.activation.err
}

func TestService_ActivateTrainer_returnsTheProfileAfterTheActivation(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{profile: UserProfile{Email: "client@test.com", Roles: []string{RoleClient, RoleTrainer}}}
	svc := testAuthService(store)

	profile, err := svc.ActivateTrainer(context.Background(), userID, "2026-10-01")

	if err != nil {
		t.Fatalf("ActivateTrainer() error = %v", err)
	}
	if len(store.activation.users) != 1 || store.activation.users[0] != userID || store.activation.terms[0] != "2026-10-01" {
		t.Errorf("store called with %v, %q, want [%v], [2026-10-01]", store.activation.users, store.activation.terms, userID)
	}
	if profile.Email != "client@test.com" || len(profile.Roles) != 2 {
		t.Errorf("profile = %+v, want the stored one", profile)
	}
}

func TestService_ActivateTrainer_storeErrorsAreReturnedWithoutAProfile(t *testing.T) {
	tests := []struct {
		name     string
		storeErr error
	}{
		{"admin", ErrAdminCannotActivate},
		{"account gone", pgx.ErrNoRows},
		{"database failure", errors.New("db down")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{activation: storeActivation{err: tt.storeErr}, profile: UserProfile{Email: "x@test.com"}}
			svc := testAuthService(store)

			profile, err := svc.ActivateTrainer(context.Background(), uuid.New(), "2026-10-01")

			if !errors.Is(err, tt.storeErr) {
				t.Fatalf("error = %v, want %v", err, tt.storeErr)
			}
			if profile.Email != "" {
				t.Errorf("profile = %+v, want the zero value", profile)
			}
		})
	}
}

func TestService_ActivateTrainer_profileFailureIsReturned(t *testing.T) {
	boom := errors.New("profile down")
	store := &fakeAuthStore{profileErr: boom}
	svc := testAuthService(store)

	_, err := svc.ActivateTrainer(context.Background(), uuid.New(), "2026-10-01")

	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}
