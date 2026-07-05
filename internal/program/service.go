package program

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/sqlc"
)

type programStore interface {
	CreateDraft(ctx context.Context, userID uuid.UUID) (Detail, error)
	List(ctx context.Context, params ListParams) (ListResult, error)
	GetProgramRow(ctx context.Context, id uuid.UUID) (Program, error)
	GetDetail(ctx context.Context, id uuid.UUID) (Detail, error)
	Update(ctx context.Context, id, userID uuid.UUID, in UpdateInput) (Detail, error)
	SetStatus(ctx context.Context, id, userID uuid.UUID, status Status) (Detail, error)
	PublishFromDraft(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error)
	FreezePublishedVersion(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error)
	SoftDelete(ctx context.Context, id, userID uuid.UUID) error
	AddWeek(ctx context.Context, programID uuid.UUID) (Detail, error)
	DeleteWeek(ctx context.Context, programID, weekID uuid.UUID) (Detail, error)
	AddDay(ctx context.Context, programID, weekID uuid.UUID) (Detail, error)
	DeleteDay(ctx context.Context, programID, weekID, dayID uuid.UUID) (Detail, error)
	CreateDayBlock(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, in CreateDayBlockInput) (Detail, error)
	AddBlockExercise(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error)
	UpdateBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, in DayExerciseInput) (Detail, error)
	DeleteBlockExercise(ctx context.Context, programID, weekID, blockID, itemID uuid.UUID) (Detail, error)
	PatchDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID, in BlockPatchInput) (Detail, error)
	MergeDayBlocks(ctx context.Context, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error)
	UngroupDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID) (Detail, error)
	MoveDayBlock(ctx context.Context, programID, weekID, blockID, targetDayID uuid.UUID, insertSort int) (Detail, error)
	ExtractBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, insertSort int) (Detail, error)
	MoveExerciseToBlock(ctx context.Context, userID, programID, weekID, blockID, itemID, targetBlockID uuid.UUID) (Detail, error)
	DeleteDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID) (Detail, error)
	ReorderWeeks(ctx context.Context, programID uuid.UUID, weekIDs []uuid.UUID) (Detail, error)
	ReorderDays(ctx context.Context, programID, weekID uuid.UUID, dayIDs []uuid.UUID) (Detail, error)
	ReorderDayBlocks(ctx context.Context, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error)
	ReorderBlockExercises(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, itemIDs []uuid.UUID) (Detail, error)
	TrainerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	GetClientProgramAssignment(ctx context.Context, trainerID, clientUserID uuid.UUID) (*Assignment, error)
	SetClientProgramAssignment(ctx context.Context, trainerUserID, trainerID, clientUserID uuid.UUID, programID *uuid.UUID) (*Assignment, error)
	ListProgramAssignments(ctx context.Context, programID uuid.UUID) (AssignmentListResult, error)
	SyncProgramAssignments(ctx context.Context, programID uuid.UUID, req AssignmentSyncRequest) (AssignmentSyncResult, error)
	ListProgramVersions(ctx context.Context, programID uuid.UUID) (VersionListResult, error)
	DeleteProgramVersion(ctx context.Context, programID, versionID uuid.UUID) error
	CleanupProgramVersions(ctx context.Context, programID uuid.UUID) (VersionCleanupResult, error)
}

type Service struct {
	store programStore
	roles auth.RoleQuerier
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		store: NewStore(pool),
		roles: sqlc.New(pool),
	}
}

// NewServiceWithStore wires a Service with test or custom store implementations.
func NewServiceWithStore(store programStore, roles auth.RoleQuerier) *Service {
	return &Service{store: store, roles: roles}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID) (Detail, error) {
	return s.store.CreateDraft(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, params ListParams) (ListResult, error) {
	isAdmin, err := auth.UserIsAdmin(ctx, s.roles, userID)
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
	if err := s.ensureAccess(ctx, userID, id); err != nil {
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
	var out Detail
	switch d.Status {
	case StatusDraft:
		out, err = s.store.PublishFromDraft(ctx, id, userID, d)
	case StatusArchived:
		out, err = s.store.SetStatus(ctx, id, userID, StatusPublished)
	default:
		return Detail{}, ErrInvalidStatusTransition
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return out, nil
}

func (s *Service) PublishUpdate(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
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
	if d.Status != StatusPublished {
		return Detail{}, ErrInvalidStatusTransition
	}
	if !d.HasUnpublishedChanges {
		return Detail{}, ErrNoUnpublishedChanges
	}
	if err := validatePublishDetail(d); err != nil {
		return Detail{}, err
	}
	out, err := s.store.FreezePublishedVersion(ctx, id, userID, d)
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

func (s *Service) AddWeek(ctx context.Context, userID, programID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	return s.store.AddWeek(ctx, programID)
}

func (s *Service) DeleteWeek(ctx context.Context, userID, programID, weekID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteWeek(ctx, programID, weekID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) AddDay(ctx context.Context, userID, programID, weekID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.AddDay(ctx, programID, weekID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) DeleteDay(ctx context.Context, userID, programID, weekID, dayID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteDay(ctx, programID, weekID, dayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) CreateDayBlock(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, in CreateDayBlockInput) (Detail, error) {
	if err := in.ValidateDraft(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.CreateDayBlock(ctx, userID, programID, weekID, dayID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) UpdateBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, in DayExerciseInput) (Detail, error) {
	if err := in.ValidateDraft(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.UpdateBlockExercise(ctx, userID, programID, weekID, blockID, itemID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) DeleteBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteBlockExercise(ctx, programID, weekID, blockID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ReorderWeeks(ctx context.Context, userID, programID uuid.UUID, weekIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.ReorderWeeks(ctx, programID, weekIDs)
	if err != nil {
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ReorderDays(ctx context.Context, userID, programID, weekID uuid.UUID, dayIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.ReorderDays(ctx, programID, weekID, dayIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ReorderDayBlocks(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.ReorderDayBlocks(ctx, programID, weekID, dayID, blockIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ReorderBlockExercises(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, itemIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.ReorderBlockExercises(ctx, userID, programID, weekID, blockID, itemIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) AddBlockExercise(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error) {
	if err := in.ValidateDraft(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.AddBlockExercise(ctx, userID, programID, weekID, blockID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) PatchDayBlock(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in BlockPatchInput) (Detail, error) {
	if err := in.Validate(); err != nil {
		return Detail{}, err
	}
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.PatchDayBlock(ctx, programID, weekID, blockID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) MergeDayBlocks(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.MergeDayBlocks(ctx, programID, weekID, dayID, blockIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) UngroupDayBlock(ctx context.Context, userID, programID, weekID, blockID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.UngroupDayBlock(ctx, programID, weekID, blockID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) MoveDayBlock(ctx context.Context, userID, programID, weekID, blockID, targetDayID uuid.UUID, insertSort int) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.MoveDayBlock(ctx, programID, weekID, blockID, targetDayID, insertSort)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) ExtractBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, insertSort int) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.ExtractBlockExercise(ctx, userID, programID, weekID, blockID, itemID, insertSort)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) MoveExerciseToBlock(ctx context.Context, userID, programID, weekID, blockID, itemID, targetBlockID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.MoveExerciseToBlock(ctx, userID, programID, weekID, blockID, itemID, targetBlockID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) DeleteDayBlock(ctx context.Context, userID, programID, weekID, blockID uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	d, err := s.store.DeleteDayBlock(ctx, programID, weekID, blockID)
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
	if err := s.ensureOwnerOrAdmin(ctx, userID, p.CreatedBy); err != nil {
		return err
	}
	if p.Status == StatusArchived {
		return ErrReadOnly
	}
	return nil
}

func (s *Service) ensureOwnerOrAdmin(ctx context.Context, userID, ownerID uuid.UUID) error {
	err := auth.EnsureOwnerOrAdmin(ctx, s.roles, userID, ownerID)
	if errors.Is(err, auth.ErrForbidden) {
		return ErrForbidden
	}
	return err
}
