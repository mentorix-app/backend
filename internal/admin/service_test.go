package admin

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/auth"
)

type fakeRoleStore struct {
	profiles map[uuid.UUID]auth.UserProfile
	roles    map[uuid.UUID][]string
}

func (f *fakeRoleStore) UserProfile(_ context.Context, userID uuid.UUID) (auth.UserProfile, error) {
	profile, ok := f.profiles[userID]
	if !ok {
		return auth.UserProfile{}, pgx.ErrNoRows
	}
	roles := append([]string(nil), f.roles[userID]...)
	slices.Sort(roles)
	profile.Roles = roles
	return profile, nil
}

func (f *fakeRoleStore) GrantRole(_ context.Context, userID uuid.UUID, role string) error {
	for _, r := range f.roles[userID] {
		if r == role {
			return nil
		}
	}
	f.roles[userID] = append(f.roles[userID], role)
	return nil
}

func TestService_GrantAdmin(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	createdAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	store := &fakeRoleStore{
		profiles: map[uuid.UUID]auth.UserProfile{
			userID: {Email: "trainer@test.com", CreatedAt: createdAt},
		},
		roles: map[uuid.UUID][]string{
			userID: {auth.RoleTrainer},
		},
	}
	svc := &Service{store: store}

	t.Run("user not found", func(t *testing.T) {
		_, err := svc.GrantAdmin(context.Background(), uuid.New())
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("grant success", func(t *testing.T) {
		profile, err := svc.GrantAdmin(context.Background(), userID)
		if err != nil {
			t.Fatalf("GrantAdmin: %v", err)
		}
		if len(profile.Roles) != 2 {
			t.Fatalf("roles = %v, want 2 entries", profile.Roles)
		}
		if profile.Roles[0] != auth.RoleAdmin || profile.Roles[1] != auth.RoleTrainer {
			t.Fatalf("roles = %v, want [admin trainer]", profile.Roles)
		}
	})

	t.Run("idempotent grant", func(t *testing.T) {
		profile, err := svc.GrantAdmin(context.Background(), userID)
		if err != nil {
			t.Fatalf("GrantAdmin: %v", err)
		}
		if len(profile.Roles) != 2 {
			t.Fatalf("roles = %v, want 2 entries after repeat grant", profile.Roles)
		}
	})
}
