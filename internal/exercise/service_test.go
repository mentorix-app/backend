package exercise

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type deleteAllStore struct {
	count int64
	err   error
}

func (s *deleteAllStore) List(context.Context, ListParams) (ListResult, error) {
	panic("not implemented")
}

func (s *deleteAllStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteAllStore) Create(context.Context, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteAllStore) Update(context.Context, uuid.UUID, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (s *deleteAllStore) Delete(context.Context, uuid.UUID) error {
	panic("not implemented")
}

func (s *deleteAllStore) DeleteAll(context.Context) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.count, nil
}

func TestService_DeleteAll(t *testing.T) {
	t.Run("returns count", func(t *testing.T) {
		svc := &Service{store: &deleteAllStore{count: 5}}
		count, err := svc.DeleteAll(context.Background())
		if err != nil {
			t.Fatalf("DeleteAll: %v", err)
		}
		if count != 5 {
			t.Fatalf("count = %d, want 5", count)
		}
	})

	t.Run("propagates error", func(t *testing.T) {
		want := errors.New("delete failed")
		svc := &Service{store: &deleteAllStore{err: want}}
		_, err := svc.DeleteAll(context.Background())
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}
