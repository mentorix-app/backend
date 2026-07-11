package program

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func (s *Store) TrainerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		return uuid.Nil, err
	}
	return pgconv.FromPGUUID(id), nil
}

func (s *Store) GetClientProgramAssignment(ctx context.Context, trainerID, clientUserID uuid.UUID) (*Assignment, error) {
	if err := s.ensureTrainerClientActive(ctx, trainerID, clientUserID); err != nil {
		return nil, err
	}

	row, err := s.q.GetProgramAssignmentByTrainerClient(ctx, sqlc.GetProgramAssignmentByTrainerClientParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get assignment: %w", err)
	}
	return s.assignmentFromDB(ctx, row)
}

func (s *Store) SetClientProgramAssignment(ctx context.Context, trainerUserID, trainerID, clientUserID uuid.UUID, programID *uuid.UUID) (*Assignment, error) {
	if err := s.ensureTrainerClientActive(ctx, trainerID, clientUserID); err != nil {
		return nil, err
	}
	if _, err := s.q.GetUserByID(ctx, pgconv.ToPGUUID(clientUserID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClientNotFound
		}
		return nil, fmt.Errorf("get client user: %w", err)
	}

	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	trainerPG := pgconv.ToPGUUID(trainerID)
	clientPG := pgconv.ToPGUUID(clientUserID)
	trainerUserPG := pgconv.ToPGUUID(trainerUserID)

	existing, err := qtx.GetProgramAssignmentByTrainerClient(ctx, sqlc.GetProgramAssignmentByTrainerClientParams{
		TrainerID:    trainerPG,
		ClientUserID: clientPG,
	})
	var previousProgramID *uuid.UUID
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get assignment: %w", err)
		}
	} else {
		pid := pgconv.FromPGUUID(existing.ProgramID)
		previousProgramID = &pid
	}

	if programID == nil {
		if previousProgramID != nil {
			if _, err := qtx.DeleteProgramAssignmentByTrainerClient(ctx, sqlc.DeleteProgramAssignmentByTrainerClientParams{
				TrainerID:    trainerPG,
				ClientUserID: clientPG,
			}); err != nil {
				return nil, fmt.Errorf("delete assignment: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		if previousProgramID != nil {
			s.cleanupUnusedVersionsBestEffort(ctx, *previousProgramID)
		}
		return nil, nil
	}

	p, err := s.GetProgramRow(ctx, *programID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if p.CreatedBy != trainerUserID {
		return nil, ErrForbidden
	}
	if p.Status != StatusPublished {
		return nil, ErrProgramNotPublished
	}

	latest, err := qtx.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(*programID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProgramNotPublished
		}
		return nil, fmt.Errorf("latest program version: %w", err)
	}

	var row sqlc.MentorixProgramAssignment
	if previousProgramID == nil {
		row, err = qtx.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
			ProgramID:        pgconv.ToPGUUID(*programID),
			ProgramVersionID: latest.ID,
			TrainerID:        trainerPG,
			ClientUserID:     clientPG,
			Status:           string(AssignmentStatusActive),
			AssignedAt:       now,
			CreatedBy:        trainerUserPG,
			ModifiedAt:       now,
			ModifiedBy:       trainerUserPG,
		})
		if err != nil {
			return nil, fmt.Errorf("insert program assignment: %w", err)
		}
	} else {
		row, err = qtx.UpdateProgramAssignment(ctx, sqlc.UpdateProgramAssignmentParams{
			TrainerID:        trainerPG,
			ClientUserID:     clientPG,
			ProgramID:        pgconv.ToPGUUID(*programID),
			ProgramVersionID: latest.ID,
			AssignedAt:       now,
			ModifiedAt:       now,
			ModifiedBy:       trainerUserPG,
		})
		if err != nil {
			return nil, fmt.Errorf("update program assignment: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	if previousProgramID != nil && *previousProgramID != *programID {
		s.cleanupUnusedVersionsBestEffort(ctx, *previousProgramID)
	}

	return s.assignmentFromDBWithVersion(ctx, row, latest)
}

func (s *Store) DeleteProgramAssignments(ctx context.Context, programID uuid.UUID) error {
	if _, err := s.q.DeleteProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(programID)); err != nil {
		return fmt.Errorf("delete program assignments: %w", err)
	}
	return nil
}

func (s *Store) validateProgramForAssignment(ctx context.Context, trainerUserID, programID uuid.UUID) error {
	p, err := s.GetProgramRow(ctx, programID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if p.DeletedAt != nil {
		return ErrNotFound
	}
	if p.CreatedBy != trainerUserID {
		return ErrForbidden
	}
	if p.Status != StatusPublished {
		return ErrProgramNotPublished
	}
	if _, err := s.q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(programID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProgramNotPublished
		}
		return fmt.Errorf("latest program version: %w", err)
	}
	return nil
}

func (s *Store) ensureTrainerClientActive(ctx context.Context, trainerID, clientUserID uuid.UUID) error {
	status, err := s.q.TrainerClientLinkActive(ctx, sqlc.TrainerClientLinkActiveParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrClientNotLinked
		}
		return fmt.Errorf("trainer client link: %w", err)
	}
	if status == "blocked" {
		return ErrClientBlocked
	}
	return nil
}

func (s *Store) assignmentFromDB(ctx context.Context, row sqlc.MentorixProgramAssignment) (*Assignment, error) {
	version, err := s.q.GetProgramVersionByID(ctx, row.ProgramVersionID)
	if err != nil {
		return nil, fmt.Errorf("get program version: %w", err)
	}
	return s.assignmentFromDBWithVersion(ctx, row, version)
}

func (s *Store) assignmentFromDBWithVersion(ctx context.Context, row sqlc.MentorixProgramAssignment, version sqlc.MentorixProgramVersion) (*Assignment, error) {
	programID := pgconv.FromPGUUID(row.ProgramID)
	latest, err := s.q.GetLatestProgramVersionByProgramID(ctx, row.ProgramID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("latest program version: %w", err)
	}

	a := Assignment{
		ID:               pgconv.FromPGUUID(row.ID),
		ProgramID:        programID,
		ProgramVersionID: pgconv.FromPGUUID(row.ProgramVersionID),
		TrainerID:        pgconv.FromPGUUID(row.TrainerID),
		ClientUserID:     pgconv.FromPGUUID(row.ClientUserID),
		Status:           AssignmentStatus(row.Status),
		AssignedAt:       row.AssignedAt.UTC(),
		CreatedAt:        row.CreatedAt.UTC(),
	}
	publishedAt := version.PublishedAt.UTC()
	a.ClientPlanAt = &publishedAt

	if err == nil {
		behind := pgconv.FromPGUUID(latest.ID) != a.ProgramVersionID
		a.IsBehindLatest = &behind
	}
	return &a, nil
}

func (s *Store) ListProgramAssignments(ctx context.Context, programID uuid.UUID) (AssignmentListResult, error) {
	rows, err := s.q.ListActiveProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return AssignmentListResult{}, fmt.Errorf("list active assignments: %w", err)
	}

	var latestID *uuid.UUID
	latest, err := s.q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err == nil {
		id := pgconv.FromPGUUID(latest.ID)
		latestID = &id
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return AssignmentListResult{}, fmt.Errorf("latest program version: %w", err)
	}

	items := make([]Assignment, 0, len(rows))
	for _, row := range rows {
		a, err := s.assignmentFromDB(ctx, row)
		if err != nil {
			return AssignmentListResult{}, err
		}
		items = append(items, *a)
	}
	return AssignmentListResult{Items: items, LatestProgramVersionID: latestID}, nil
}

func (s *Store) SyncProgramAssignments(ctx context.Context, userID, programID uuid.UUID, req AssignmentSyncRequest) (AssignmentSyncResult, error) {
	latest, err := s.q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssignmentSyncResult{}, ErrProgramNotPublished
		}
		return AssignmentSyncResult{}, fmt.Errorf("latest program version: %w", err)
	}

	now := time.Now().UTC()
	latestVersionID := pgconv.FromPGUUID(latest.ID)
	result := AssignmentSyncResult{
		Synced:  make([]Assignment, 0),
		Skipped: make([]AssignmentSyncSkipped, 0),
	}

	syncOne := func(row sqlc.MentorixProgramAssignment) error {
		assignmentID := pgconv.FromPGUUID(row.ID)
		if pgconv.FromPGUUID(row.ProgramID) != programID {
			result.Skipped = append(result.Skipped, AssignmentSyncSkipped{
				AssignmentID: assignmentID,
				Reason:       "wrong_program",
			})
			return nil
		}
		if row.Status != string(AssignmentStatusActive) {
			result.Skipped = append(result.Skipped, AssignmentSyncSkipped{
				AssignmentID: assignmentID,
				Reason:       "not_active",
			})
			return nil
		}
		if pgconv.FromPGUUID(row.ProgramVersionID) == latestVersionID {
			result.Skipped = append(result.Skipped, AssignmentSyncSkipped{
				AssignmentID: assignmentID,
				Reason:       "already_on_latest",
			})
			return nil
		}

		updated, err := s.q.UpdateProgramAssignmentVersion(ctx, sqlc.UpdateProgramAssignmentVersionParams{
			ID:               row.ID,
			ProgramVersionID: latest.ID,
			ModifiedAt:       now,
			ModifiedBy:       pgconv.ToPGUUID(userID),
		})
		if err != nil {
			return fmt.Errorf("update assignment version: %w", err)
		}
		if updated == 0 {
			result.Skipped = append(result.Skipped, AssignmentSyncSkipped{
				AssignmentID: assignmentID,
				Reason:       "not_active",
			})
			return nil
		}

		updatedRow, err := s.q.GetProgramAssignmentByID(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("get updated assignment: %w", err)
		}
		a, err := s.assignmentFromDBWithVersion(ctx, updatedRow, latest)
		if err != nil {
			return err
		}
		result.Synced = append(result.Synced, *a)
		return nil
	}

	if req.AllActive != nil && *req.AllActive {
		rows, err := s.q.ListActiveProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(programID))
		if err != nil {
			return AssignmentSyncResult{}, fmt.Errorf("list active assignments: %w", err)
		}
		for _, row := range rows {
			if err := syncOne(row); err != nil {
				return AssignmentSyncResult{}, err
			}
		}
	} else {
		for _, id := range req.AssignmentIDs {
			row, err := s.q.GetProgramAssignmentByID(ctx, pgconv.ToPGUUID(id))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					result.Skipped = append(result.Skipped, AssignmentSyncSkipped{
						AssignmentID: id,
						Reason:       "not_found",
					})
					continue
				}
				return AssignmentSyncResult{}, fmt.Errorf("get assignment: %w", err)
			}
			if err := syncOne(row); err != nil {
				return AssignmentSyncResult{}, err
			}
		}
	}

	if len(result.Synced) > 0 {
		s.cleanupUnusedVersionsBestEffort(ctx, programID)
	}
	return result, nil
}
