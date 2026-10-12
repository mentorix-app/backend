package auth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDeletionError(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		wantSentinel bool
	}{
		{"foreign key violation", "23503", true},
		{"check violation", "23514", true},
		{"not null violation", "23502", false},
		{"serialization failure", "40001", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{Code: tt.code, ConstraintName: "some_constraint"}

			err := deletionError("delete user", fmt.Errorf("exec: %w", pgErr))

			if got := errors.Is(err, ErrAccountDeletionUnavailable); got != tt.wantSentinel {
				t.Errorf("errors.Is(ErrAccountDeletionUnavailable) = %v, want %v (error: %v)", got, tt.wantSentinel, err)
			}
			var found *pgconn.PgError
			if !errors.As(err, &found) || found != pgErr {
				t.Errorf("the PgError is not in the chain of %v", err)
			}
		})
	}

	t.Run("not a database error", func(t *testing.T) {
		cause := errors.New("connection reset")

		err := deletionError("delete user", cause)

		if errors.Is(err, ErrAccountDeletionUnavailable) {
			t.Errorf("a plain error became the refusal: %v", err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("the cause is not in the chain of %v", err)
		}
	})
}

func TestOnlyClientRole(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  bool
	}{
		{"client", []string{RoleClient}, true},
		{"no role rows", nil, false},
		{"empty list", []string{}, false},
		{"trainer", []string{RoleTrainer}, false},
		{"admin", []string{RoleAdmin}, false},
		{"client and trainer", []string{RoleClient, RoleTrainer}, false},
		{"unknown role", []string{"owner"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := onlyClientRole(tt.roles); got != tt.want {
				t.Errorf("onlyClientRole(%v) = %v, want %v", tt.roles, got, tt.want)
			}
		})
	}
}
