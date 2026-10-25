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

// DuplicateBlockExercise copies one exercise. In a group block the copy sits right after
// the original. A single block has one exercise, so the copy becomes a new single block
// right after it, with the same client rules (as separate rows).
func (s *Store) DuplicateBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}
	itemBlockID, err := s.exerciseBlockID(ctx, itemID)
	if err != nil {
		return Detail{}, err
	}
	if itemBlockID != blockID {
		return Detail{}, pgx.ErrNoRows
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	// Lock before reading the block and its client rules, as Extract and Ungroup do:
	// a concurrent SetBlockClients must not slip between the read and the copy.
	if err := qtx.LockProgramForUpdate(ctx, pgconv.ToPGUUID(programID)); err != nil {
		return Detail{}, fmt.Errorf("lock program: %w", err)
	}
	block, err := qtx.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	exercises, err := qtx.ListBlockExercises(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		return Detail{}, fmt.Errorf("list block exercises: %w", err)
	}
	itemIdx := -1
	for i, ex := range exercises {
		if pgconv.FromPGUUID(ex.ID) == itemID {
			itemIdx = i
			break
		}
	}
	if itemIdx < 0 {
		return Detail{}, pgx.ErrNoRows
	}

	if BlockType(block.BlockType) == BlockTypeSingle {
		if err := duplicateSingleBlock(ctx, qtx, userID, programID, block, itemID); err != nil {
			return Detail{}, err
		}
	} else {
		modifiedAt, modifiedBy := dayBlockAudit(userID)
		newItemID, err := qtx.InsertBlockExerciseCopy(ctx, sqlc.InsertBlockExerciseCopyParams{
			ProgramWeekDayBlockID: block.ID,
			SortOrder:             int32(len(exercises) + 1),
			ModifiedAt:            modifiedAt,
			ModifiedBy:            modifiedBy,
			SourceItemID:          pgconv.ToPGUUID(itemID),
			SourceBlockID:         block.ID,
		})
		if err != nil {
			return Detail{}, copyInsertError("copy block exercise", err)
		}
		ids := make([]uuid.UUID, 0, len(exercises)+1)
		for i, ex := range exercises {
			ids = append(ids, pgconv.FromPGUUID(ex.ID))
			if i == itemIdx {
				ids = append(ids, pgconv.FromPGUUID(newItemID))
			}
		}
		if err := applyBlockExerciseOrder(ctx, qtx, blockID, ids, userID); err != nil {
			return Detail{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

// duplicateSingleBlock inserts a copy of a single block, with the copy of its only exercise,
// right after the original in the day.
func duplicateSingleBlock(ctx context.Context, qtx *sqlc.Queries, userID, programID uuid.UUID, block sqlc.GetDayBlockByIDRow, itemID uuid.UUID) error {
	dayID := pgconv.FromPGUUID(block.ProgramWeekDayID)
	ids, err := listDayBlockUUIDs(ctx, qtx, dayID)
	if err != nil {
		return err
	}
	blockIdx := -1
	for i, id := range ids {
		if id == pgconv.FromPGUUID(block.ID) {
			blockIdx = i
			break
		}
	}
	if blockIdx < 0 {
		return pgx.ErrNoRows
	}

	modifiedAt, modifiedBy := dayBlockAudit(userID)
	row, err := qtx.InsertDayBlockCopy(ctx, sqlc.InsertDayBlockCopyParams{
		ProgramWeekDayID: block.ProgramWeekDayID,
		SortOrder:        int32(len(ids) + 1),
		ModifiedAt:       modifiedAt,
		ModifiedBy:       modifiedBy,
		ProgramID:        pgconv.ToPGUUID(programID),
		SourceBlockID:    block.ID,
	})
	if err != nil {
		return copyInsertError("copy single block", err)
	}
	if _, err := qtx.InsertBlockExerciseCopy(ctx, sqlc.InsertBlockExerciseCopyParams{
		ProgramWeekDayBlockID: row.ID,
		SortOrder:             1,
		ModifiedAt:            modifiedAt,
		ModifiedBy:            modifiedBy,
		SourceItemID:          pgconv.ToPGUUID(itemID),
		SourceBlockID:         block.ID,
	}); err != nil {
		return copyInsertError("copy block exercise", err)
	}
	if err := qtx.CopyProgramBlockClients(ctx, sqlc.CopyProgramBlockClientsParams{
		ProgramID:      pgconv.ToPGUUID(programID),
		TargetBlockKey: row.BlockKey,
		SourceBlockKey: block.BlockKey,
	}); err != nil {
		return fmt.Errorf("copy block clients: %w", err)
	}
	return applyDayBlockOrder(ctx, qtx, dayID, insertUUIDAt(ids, pgconv.FromPGUUID(row.ID), blockIdx+2), userID)
}

// CopyDayBlock copies a block of any type, with its exercises and client rules, to the end
// of targetDayID. The target may be any day of the program, the source day included. A
// limited copy may land in a day without a shared block: publishing validates that.
func (s *Store) CopyDayBlock(ctx context.Context, userID, programID, weekID, blockID, targetDayID uuid.UUID) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	// Lock before reading the source rules, for the reason given in DuplicateBlockExercise.
	if err := qtx.LockProgramForUpdate(ctx, pgconv.ToPGUUID(programID)); err != nil {
		return Detail{}, fmt.Errorf("lock program: %w", err)
	}
	inProgram, err := qtx.DayBelongsToProgram(ctx, sqlc.DayBelongsToProgramParams{
		ID:        pgconv.ToPGUUID(targetDayID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("check target day: %w", err)
	}
	if !inProgram {
		return Detail{}, pgx.ErrNoRows
	}
	source, err := qtx.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	nextSort, err := qtx.NextDayBlockSort(ctx, pgconv.ToPGUUID(targetDayID))
	if err != nil {
		return Detail{}, fmt.Errorf("next block sort: %w", err)
	}

	modifiedAt, modifiedBy := dayBlockAudit(userID)
	row, err := qtx.InsertDayBlockCopy(ctx, sqlc.InsertDayBlockCopyParams{
		ProgramWeekDayID: pgconv.ToPGUUID(targetDayID),
		SortOrder:        nextSort,
		ModifiedAt:       modifiedAt,
		ModifiedBy:       modifiedBy,
		ProgramID:        pgconv.ToPGUUID(programID),
		SourceBlockID:    source.ID,
	})
	if err != nil {
		return Detail{}, copyInsertError("copy block", err)
	}
	if err := qtx.InsertBlockExercisesCopy(ctx, sqlc.InsertBlockExercisesCopyParams{
		ProgramWeekDayBlockID: row.ID,
		ModifiedAt:            modifiedAt,
		ModifiedBy:            modifiedBy,
		SourceBlockID:         source.ID,
	}); err != nil {
		return Detail{}, fmt.Errorf("copy block exercises: %w", err)
	}
	if err := qtx.CopyProgramBlockClients(ctx, sqlc.CopyProgramBlockClientsParams{
		ProgramID:      pgconv.ToPGUUID(programID),
		TargetBlockKey: row.BlockKey,
		SourceBlockKey: source.BlockKey,
	}); err != nil {
		return Detail{}, fmt.Errorf("copy block clients: %w", err)
	}
	if err := normalizeDayBlockSort(ctx, qtx, targetDayID, userID); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

// copyInsertError maps a copy insert that matched no source row to not found: the copy
// queries scope the source to its parent, so a wrong id inserts nothing.
func copyInsertError(what string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	return fmt.Errorf("%s: %w", what, err)
}

// applyBlockExerciseOrder writes sort_order 1..N in the given order.
func applyBlockExerciseOrder(ctx context.Context, q *sqlc.Queries, blockID uuid.UUID, itemIDs []uuid.UUID, userID uuid.UUID) error {
	now := time.Now().UTC()
	blockPG := pgconv.ToPGUUID(blockID)
	userPG := pgconv.ToPGUUID(userID)
	for i, id := range itemIDs {
		if err := q.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
			ID:                    pgconv.ToPGUUID(id),
			ProgramWeekDayBlockID: blockPG,
			SortOrder:             int32(i + 1),
			ModifiedAt:            now,
			ModifiedBy:            userPG,
		}); err != nil {
			return fmt.Errorf("update exercise placement: %w", err)
		}
	}
	return nil
}
