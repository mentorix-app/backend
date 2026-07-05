package program

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func clampInsertPosition(position, max int) int {
	if position <= 0 || position > max {
		return max
	}
	return position
}

func removeUUID(ids []uuid.UUID, remove uuid.UUID) []uuid.UUID {
	if remove == uuid.Nil {
		return ids
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != remove {
			out = append(out, id)
		}
	}
	return out
}

func insertUUIDAt(ids []uuid.UUID, id uuid.UUID, position int) []uuid.UUID {
	position = clampInsertPosition(position, len(ids)+1)
	out := make([]uuid.UUID, 0, len(ids)+1)
	out = append(out, ids[:position-1]...)
	out = append(out, id)
	out = append(out, ids[position-1:]...)
	return out
}

func listDayBlockUUIDs(ctx context.Context, q *sqlc.Queries, dayID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.ListDayBlockIDsForDay(ctx, pgconv.ToPGUUID(dayID))
	if err != nil {
		return nil, fmt.Errorf("list day block ids: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, pgconv.FromPGUUID(row))
	}
	return out, nil
}

func applyDayBlockOrder(ctx context.Context, q *sqlc.Queries, dayID uuid.UUID, blockIDs []uuid.UUID) error {
	dayPG := pgconv.ToPGUUID(dayID)
	for i, blockID := range blockIDs {
		pos := int32(i + 1)
		if err := q.UpdateDayBlockPlacement(ctx, sqlc.UpdateDayBlockPlacementParams{
			ID:           pgconv.ToPGUUID(blockID),
			ProgramWeekDayID: dayPG,
			SortOrder:    pos,
		}); err != nil {
			return fmt.Errorf("update block placement: %w", err)
		}
	}
	return nil
}

func insertBlockIntoDayOrder(ctx context.Context, q *sqlc.Queries, dayID, blockID uuid.UUID, position int) error {
	ids, err := listDayBlockUUIDs(ctx, q, dayID)
	if err != nil {
		return err
	}
	ids = removeUUID(ids, blockID)
	position = clampInsertPosition(position, len(ids)+1)
	ids = insertUUIDAt(ids, blockID, position)
	return applyDayBlockOrder(ctx, q, dayID, ids)
}

func normalizeDayBlockSort(ctx context.Context, q *sqlc.Queries, dayID uuid.UUID) error {
	ids, err := listDayBlockUUIDs(ctx, q, dayID)
	if err != nil {
		return err
	}
	return applyDayBlockOrder(ctx, q, dayID, ids)
}

func normalizeBlockExerciseSort(ctx context.Context, q *sqlc.Queries, blockID uuid.UUID) error {
	rows, err := q.ListBlockExercises(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		return fmt.Errorf("list block exercises: %w", err)
	}
	blockPG := pgconv.ToPGUUID(blockID)
	for i, row := range rows {
		if int(row.SortOrder) == i+1 {
			continue
		}
		if err := q.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
			ID:                row.ID,
			ProgramWeekDayBlockID: blockPG,
			SortOrder:         int32(i + 1),
			ModifiedAt:        time.Now().UTC(),
			ModifiedBy:        pgtype.UUID{},
		}); err != nil {
			return fmt.Errorf("normalize exercise sort: %w", err)
		}
	}
	return nil
}
