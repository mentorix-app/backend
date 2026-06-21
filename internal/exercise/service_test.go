package exercise

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type deleteManyStore struct {
	count int64
	err   error
}

func (s *deleteManyStore) List(context.Context, ListParams) (ListResult, error) {
	panic("not implemented")
}

func (s *deleteManyStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteManyStore) Create(context.Context, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteManyStore) Update(context.Context, uuid.UUID, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteManyStore) DeleteMany(context.Context, []uuid.UUID) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.count, nil
}

func TestService_DeleteMany(t *testing.T) {
	t.Run("returns count", func(t *testing.T) {
		svc := &Service{store: &deleteManyStore{count: 5}}
		count, err := svc.DeleteMany(context.Background(), []uuid.UUID{uuid.New()})
		if err != nil {
			t.Fatalf("DeleteMany: %v", err)
		}
		if count != 5 {
			t.Fatalf("count = %d, want 5", count)
		}
	})

	t.Run("propagates error", func(t *testing.T) {
		want := errors.New("delete failed")
		svc := &Service{store: &deleteManyStore{err: want}}
		_, err := svc.DeleteMany(context.Background(), []uuid.UUID{uuid.New()})
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}
