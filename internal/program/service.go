package program

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/subscription"
)

type programStore interface {
	CreateDraft(ctx context.Context, userID uuid.UUID) (Detail, error)
	List(ctx context.Context, params ListParams) (ListResult, error)
	GetProgramRow(ctx context.Context, id uuid.UUID) (Program, error)
	GetDetail(ctx context.Context, id uuid.UUID) (Detail, error)
	Update(ctx context.Context, id, userID uuid.UUID, in UpdateInput) (Detail, error)
	SetStatus(ctx context.Context, id, userID uuid.UUID, status Status) (Detail, error)
	PublishFromDraft(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error)
	Republish(ctx context.Context, id, userID uuid.UUID) (Detail, error)
	FreezePublishedVersion(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error)
	RestoreWorkingTreeFromLatestVersion(ctx context.Context, id, userID uuid.UUID) (Detail, error)
	SoftDelete(ctx context.Context, id, userID uuid.UUID) error
	DeleteProgramAssignments(ctx context.Context, programID uuid.UUID) error
	AddWeek(ctx context.Context, programID uuid.UUID) (Detail, error)
	DeleteWeek(ctx context.Context, programID, weekID uuid.UUID) (Detail, error)
	AddDay(ctx context.Context, programID, weekID uuid.UUID) (Detail, error)
	DeleteDay(ctx context.Context, programID, weekID, dayID uuid.UUID) (Detail, error)
	CreateDayBlock(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, in CreateDayBlockInput) (Detail, error)
	AddBlockExercise(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error)
	UpdateBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, in DayExerciseInput) (Detail, error)
	DeleteBlockExercise(ctx context.Context, programID, weekID, blockID, itemID uuid.UUID) (Detail, error)
	PatchDayBlock(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in BlockPatchInput) (Detail, error)
	MergeDayBlocks(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error)
	UngroupDayBlock(ctx context.Context, userID, programID, weekID, blockID uuid.UUID) (Detail, error)
	MoveDayBlock(ctx context.Context, userID, programID, weekID, blockID, targetDayID uuid.UUID, insertSort int) (Detail, error)
	ExtractBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, insertSort int) (Detail, error)
	MoveExerciseToBlock(ctx context.Context, userID, programID, weekID, blockID, itemID, targetBlockID uuid.UUID) (Detail, error)
	DeleteDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID) (Detail, error)
	ReorderWeeks(ctx context.Context, programID uuid.UUID, weekIDs []uuid.UUID) (Detail, error)
	ReorderDays(ctx context.Context, programID, weekID uuid.UUID, dayIDs []uuid.UUID) (Detail, error)
	ReorderDayBlocks(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error)
	ReorderBlockExercises(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, itemIDs []uuid.UUID) (Detail, error)
	TrainerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	GetClientProgramAssignment(ctx context.Context, trainerID, clientUserID uuid.UUID) (*Assignment, error)
	SetClientProgramAssignment(ctx context.Context, trainerUserID, trainerID, clientUserID uuid.UUID, programID *uuid.UUID) (*Assignment, error)
	validateProgramForAssignment(ctx context.Context, trainerUserID, programID uuid.UUID) error
	ListProgramAssignments(ctx context.Context, programID uuid.UUID) (AssignmentListResult, error)
	SyncProgramAssignments(ctx context.Context, userID, programID uuid.UUID, req AssignmentSyncRequest) (AssignmentSyncResult, error)
	ListProgramVersions(ctx context.Context, programID uuid.UUID) (VersionListResult, error)
	DeleteProgramVersion(ctx context.Context, programID, versionID uuid.UUID) error
	CleanupProgramVersions(ctx context.Context, programID uuid.UUID) (VersionCleanupResult, error)
	GetVersionDetail(ctx context.Context, versionID uuid.UUID) (Detail, error)
	GetVersionDetailForClient(ctx context.Context, versionID, clientUserID uuid.UUID) (Detail, error)
	SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error)
}

// QuotaChecker verifies plan quotas for a trainer user; nil disables checks (tests).
type QuotaChecker interface {
	CheckQuota(ctx context.Context, trainerUserID uuid.UUID, resource subscription.Resource, op subscription.Op) error
}

type Service struct {
	store    programStore
	roles    auth.RoleQuerier
	notifier ProgramNotifier
	quota    QuotaChecker
}

type ProgramNotifier interface {
	NotifyProgramSynced(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error
}

type ServiceOption func(*Service)

func WithProgramNotifier(n ProgramNotifier) ServiceOption {
	return func(s *Service) { s.notifier = n }
}

func WithQuotaChecker(q QuotaChecker) ServiceOption {
	return func(s *Service) { s.quota = q }
}

func NewService(pool *pgxpool.Pool, opts ...ServiceOption) *Service {
	s := &Service{
		store: NewStore(pool),
		roles: sqlc.New(pool),
		quota: subscription.NewChecker(pool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewServiceWithStore wires a Service with test or custom store implementations.
func NewServiceWithStore(store programStore, roles auth.RoleQuerier, opts ...ServiceOption) *Service {
	s := &Service{store: store, roles: roles}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	if err := s.ensureOwner(ctx, userID, id); err != nil {
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
		// A draft already counts as active: block only when over the limit.
		if err := s.checkQuota(ctx, userID, subscription.ResourcePrograms, subscription.OpMutate); err != nil {
			return Detail{}, err
		}
		out, err = s.store.PublishFromDraft(ctx, id, userID, d)
	case StatusArchived:
		// Reactivation increases the active count; the store enforces the quota under lock.
		out, err = s.store.Republish(ctx, id, userID)
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

func (s *Service) DiscardUnpublished(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
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
	out, err := s.store.RestoreWorkingTreeFromLatestVersion(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return out, nil
}

func (s *Service) Archive(ctx context.Context, userID, id uuid.UUID) (Detail, error) {
	// Archiving frees quota, so it stays allowed even in read-only mode.
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
	if userID != d.CreatedBy {
		return Detail{}, ErrForbidden
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
	if userID != p.CreatedBy {
		return ErrForbidden
	}
	if p.DeletedAt != nil {
		return nil
	}
	if err := s.store.DeleteProgramAssignments(ctx, id); err != nil {
		return err
	}
	if _, err := s.store.CleanupProgramVersions(ctx, id); err != nil {
		return err
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
	d, err := s.store.ReorderDayBlocks(ctx, userID, programID, weekID, dayID, blockIDs)
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
	d, err := s.store.PatchDayBlock(ctx, userID, programID, weekID, blockID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	seen := make(map[uuid.UUID]struct{}, len(clientUserIDs))
	for _, id := range clientUserIDs {
		if id == uuid.Nil {
			return Detail{}, fmt.Errorf("%w: client_user_ids must not contain a nil uuid", ErrValidation)
		}
		if _, dup := seen[id]; dup {
			return Detail{}, fmt.Errorf("%w: client_user_ids must not contain duplicates", ErrValidation)
		}
		seen[id] = struct{}{}
	}
	d, err := s.store.SetBlockClients(ctx, userID, programID, weekID, blockID, clientUserIDs)
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
	d, err := s.store.MergeDayBlocks(ctx, userID, programID, weekID, dayID, blockIDs)
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
	d, err := s.store.UngroupDayBlock(ctx, userID, programID, weekID, blockID)
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
	d, err := s.store.MoveDayBlock(ctx, userID, programID, weekID, blockID, targetDayID, insertSort)
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

func (s *Service) ensureOwner(ctx context.Context, userID, programID uuid.UUID) error {
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
	if userID != p.CreatedBy {
		return ErrForbidden
	}
	return nil
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
	if userID != p.CreatedBy {
		return ErrForbidden
	}
	if p.Status == StatusArchived {
		return ErrReadOnly
	}
	// Read-only mode when the trainer holds more active programs than the plan allows.
	return s.checkQuota(ctx, userID, subscription.ResourcePrograms, subscription.OpMutate)
}

func (s *Service) checkQuota(ctx context.Context, userID uuid.UUID, resource subscription.Resource, op subscription.Op) error {
	if s.quota == nil {
		return nil
	}
	return s.quota.CheckQuota(ctx, userID, resource, op)
}

func (s *Service) ensureOwnerOrAdmin(ctx context.Context, userID, ownerID uuid.UUID) error {
	err := auth.EnsureOwnerOrAdmin(ctx, s.roles, userID, ownerID)
	if errors.Is(err, auth.ErrForbidden) {
		return ErrForbidden
	}
	return err
}
