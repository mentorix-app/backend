package program

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type programStore interface {
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
	CreateDraft(ctx context.Context, userID uuid.UUID) (Detail, error)
	List(ctx context.Context, params ListParams) (ListResult, error)
	GetProgramRow(ctx context.Context, id uuid.UUID) (Program, error)
	GetDetail(ctx context.Context, id uuid.UUID) (Detail, error)
	Update(ctx context.Context, id, userID uuid.UUID, in UpdateInput) (Detail, error)
	SetStatus(ctx context.Context, id, userID uuid.UUID, status Status) (Detail, error)
	SoftDelete(ctx context.Context, id, userID uuid.UUID) error
	AddDay(ctx context.Context, programID uuid.UUID) (Detail, error)
	DeleteDay(ctx context.Context, programID, dayID uuid.UUID) (Detail, error)
	AddDayExercise(ctx context.Context, programID, dayID uuid.UUID, in DayExerciseInput) (Detail, error)
	UpdateDayExercise(ctx context.Context, programID, dayID, itemID uuid.UUID, in DayExerciseInput) (Detail, error)
	DeleteDayExercise(ctx context.Context, programID, dayID, itemID uuid.UUID) (Detail, error)
}

type Service struct {
	store programStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{store: NewStore(pool)}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID) (Detail, error) {
	return s.store.CreateDraft(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, params ListParams) (ListResult, error) {
	isAdmin, err := s.store.IsAdmin(ctx, userID)
	if err != nil {
		return ListResult{}, err
	}
	if !isAdmin {
		params.CreatedBy = &userID
	}
	return s.store.List(ctx, params)
}

func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
	if err := s.ensureAccess(ctx, userID, id); err != nil {
		return Detail{}, err
	}
	d, err := s.store.GetDetail(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if d.DeletedAt != nil {
		return Detail{}, ErrNotFound
	}
	return d, nil
}

func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, in UpdateInput) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, id); err != nil {
		return Detail{}, err
	}
	d, err := s.store.Update(ctx, id, userID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) Publish(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, id); err != nil {
		return Detail{}, err
	}
	d, err := s.store.GetDetail(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	if d.DeletedAt != nil {
		return Detail{}, ErrNotFound
	}
	switch d.Status {
	case StatusDraft, StatusArchived:
	default:
		return Detail{}, ErrInvalidStatusTransition
	}
	if err := validatePublishDetail(d); err != nil {
		return Detail{}, err
	}
	out, err := s.store.SetStatus(ctx, id, userID, StatusPublished)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return out, nil
}

func (s *Service) Archive(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, id); err != nil {
		return Detail{}, err
	}
	d, err := s.store.GetProgramRow(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	if d.DeletedAt != nil {
		return Detail{}, ErrNotFound
	}
	if d.Status != StatusPublished {
		return Detail{}, ErrInvalidStatusTransition
	}
	out, err := s.store.SetStatus(ctx, id, userID, StatusArchived)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	p, err := s.store.GetProgramRow(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := s.ensureOwnerOrAdmin(ctx, userID, p.CreatedBy); err != nil {
		return err
	}
	if p.DeletedAt != nil {
		return nil
	}
	if err := s.store.SoftDelete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) AddDay(ctx context.Context, userID, programID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.AddDay(ctx, programID)
	if err != nil {
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) DeleteDay(ctx context.Context, userID, programID, dayID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteDay(ctx, programID, dayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) AddDayExercise(ctx context.Context, userID, programID, dayID uuid.UUID, in DayExerciseInput) (Detail, error) {
	if err := in.ValidateDraft(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.AddDayExercise(ctx, programID, dayID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) UpdateDayExercise(ctx context.Context, userID, programID, dayID, itemID uuid.UUID, in DayExerciseInput) (Detail, error) {
	if err := in.ValidateDraft(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.UpdateDayExercise(ctx, programID, dayID, itemID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) DeleteDayExercise(ctx context.Context, userID, programID, dayID, itemID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteDayExercise(ctx, programID, dayID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ensureAccess(ctx context.Context, userID, programID uuid.UUID) error {
	p, err := s.store.GetProgramRow(ctx, programID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if p.DeletedAt != nil {
		return ErrNotFound
	}
	return s.ensureOwnerOrAdmin(ctx, userID, p.CreatedBy)
}

func (s *Service) ensureMutable(ctx context.Context, userID, programID uuid.UUID) error {
	return s.ensureAccess(ctx, userID, programID)
}

func (s *Service) ensureOwnerOrAdmin(ctx context.Context, userID, ownerID uuid.UUID) error {
	if userID == ownerID {
		return nil
	}
	isAdmin, err := s.store.IsAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
