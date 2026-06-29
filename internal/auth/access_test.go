package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/sqlc"
)

type stubRoleQuerier struct {
	isAdmin bool
	err     error
}

func (s *stubRoleQuerier) UserHasAnyRole(context.Context, sqlc.UserHasAnyRoleParams) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.isAdmin, nil
}

func TestUserIsAdmin(t *testing.T) {
	userID := uuid.New()
	q := &stubRoleQuerier{isAdmin: true}
	ok, err := UserIsAdmin(context.Background(), q, userID)
	if err != nil || !ok {
		t.Fatalf("UserIsAdmin() = %v, %v, want true", ok, err)
	}
}

func TestEnsureOwnerOrAdmin_owner(t *testing.T) {
	userID := uuid.New()
	if err := EnsureOwnerOrAdmin(context.Background(), &stubRoleQuerier{}, userID, userID); err != nil {
		t.Fatalf("EnsureOwnerOrAdmin() error = %v, want nil", err)
	}
}

func TestEnsureOwnerOrAdmin_admin(t *testing.T) {
	ownerID := uuid.New()
	adminID := uuid.New()
	if err := EnsureOwnerOrAdmin(context.Background(), &stubRoleQuerier{isAdmin: true}, adminID, ownerID); err != nil {
		t.Fatalf("EnsureOwnerOrAdmin() error = %v, want nil", err)
	}
}

func TestEnsureOwnerOrAdmin_forbidden(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	err := EnsureOwnerOrAdmin(context.Background(), &stubRoleQuerier{isAdmin: false}, otherID, ownerID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("EnsureOwnerOrAdmin() error = %v, want ErrForbidden", err)
	}
}

func TestEnsureOwnerOrAdmin_roleCheckError(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	want := errors.New("db down")
	err := EnsureOwnerOrAdmin(context.Background(), &stubRoleQuerier{err: want}, otherID, ownerID)
	if !errors.Is(err, want) {
		t.Fatalf("EnsureOwnerOrAdmin() error = %v, want %v", err, want)
	}
}
