package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// ErrForbidden is returned when the caller is neither the resource owner nor an admin.
var ErrForbidden = errors.New("forbidden")

// RoleQuerier checks role membership in the database.
type RoleQuerier interface {
	UserHasAnyRole(ctx context.Context, arg sqlc.UserHasAnyRoleParams) (bool, error)
}

// UserIsAdmin reports whether userID has the admin role.
func UserIsAdmin(ctx context.Context, q RoleQuerier, userID uuid.UUID) (bool, error) {
	ok, err := q.UserHasAnyRole(ctx, sqlc.UserHasAnyRoleParams{
		UserID: pgconv.ToPGUUID(userID),
		Roles:  []string{RoleAdmin},
	})
	if err != nil {
		return false, fmt.Errorf("check admin role: %w", err)
	}
	return ok, nil
}

// EnsureOwnerOrAdmin allows access when userID is ownerID or has the admin role.
func EnsureOwnerOrAdmin(ctx context.Context, q RoleQuerier, userID, ownerID uuid.UUID) error {
	if userID == ownerID {
		return nil
	}
	isAdmin, err := UserIsAdmin(ctx, q, userID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
