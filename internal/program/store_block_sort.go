package program

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func dayBlockAudit(userID uuid.UUID) (time.Time, pgtype.UUID) {
	now := time.Now().UTC()
	if userID == uuid.Nil {
		return now, pgtype.UUID{}
	}
	return now, pgconv.ToPGUUID(userID)
}

func insertDayBlockParams(dayID pgtype.UUID, blockType, instruction string, sortOrder int32, userID uuid.UUID) sqlc.InsertDayBlockParams {
	modifiedAt, modifiedBy := dayBlockAudit(userID)
	return sqlc.InsertDayBlockParams{
		ProgramWeekDayID: dayID,
		BlockType:        blockType,
		Instruction:      instruction,
		SortOrder:        sortOrder,
		ModifiedAt:       modifiedAt,
		ModifiedBy:       modifiedBy,
	}
}

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

func applyDayBlockOrder(ctx context.Context, q *sqlc.Queries, dayID uuid.UUID, blockIDs []uuid.UUID, userID uuid.UUID) error {
	dayPG := pgconv.ToPGUUID(dayID)
	modifiedAt, modifiedBy := dayBlockAudit(userID)
	for i, blockID := range blockIDs {
		pos := int32(i + 1)
		if err := q.UpdateDayBlockPlacement(ctx, sqlc.UpdateDayBlockPlacementParams{
			ID:               pgconv.ToPGUUID(blockID),
			ProgramWeekDayID: dayPG,
			SortOrder:        pos,
			ModifiedAt:       modifiedAt,
			ModifiedBy:       modifiedBy,
		}); err != nil {
			return fmt.Errorf("update block placement: %w", err)
		}
	}
	return nil
}

func insertBlockIntoDayOrder(ctx context.Context, q *sqlc.Queries, dayID, blockID uuid.UUID, position int, userID uuid.UUID) error {
	ids, err := listDayBlockUUIDs(ctx, q, dayID)
	if err != nil {
		return err
	}
	ids = removeUUID(ids, blockID)
	position = clampInsertPosition(position, len(ids)+1)
	ids = insertUUIDAt(ids, blockID, position)
	return applyDayBlockOrder(ctx, q, dayID, ids, userID)
}

func normalizeDayBlockSort(ctx context.Context, q *sqlc.Queries, dayID uuid.UUID) error {
	ids, err := listDayBlockUUIDs(ctx, q, dayID)
	if err != nil {
		return err
	}
	return applyDayBlockOrder(ctx, q, dayID, ids, uuid.Nil)
}

func normalizeBlockExerciseSort(ctx context.Context, q *sqlc.Queries, blockID uuid.UUID) error {
	rows, err := q.ListBlockExercises(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		return fmt.Errorf("list block exercises: %w", err)
	}
	blockPG := pgconv.ToPGUUID(blockID)
	now := time.Now().UTC()
	for i, row := range rows {
		if err := q.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
			ID:                    row.ID,
			ProgramWeekDayBlockID: blockPG,
			SortOrder:             int32(i + 1),
			ModifiedAt:            now,
			ModifiedBy:            pgtype.UUID{},
		}); err != nil {
			return fmt.Errorf("normalize exercise sort: %w", err)
		}
	}
	return nil
}

func normalizeProgramWeekSort(ctx context.Context, q *sqlc.Queries, programID uuid.UUID) error {
	rows, err := q.ListProgramWeeks(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list program weeks: %w", err)
	}
	programPG := pgconv.ToPGUUID(programID)
	for i, row := range rows {
		if err := q.UpdateProgramWeekOrder(ctx, sqlc.UpdateProgramWeekOrderParams{
			ID:         row.ID,
			ProgramID:  programPG,
			SortOrder:  int32(i + 1),
			WeekNumber: row.WeekNumber,
		}); err != nil {
			return fmt.Errorf("normalize week sort: %w", err)
		}
	}
	return nil
}

func normalizeProgramDaySort(ctx context.Context, q *sqlc.Queries, weekID uuid.UUID) error {
	rows, err := q.ListProgramDaysForWeek(ctx, pgconv.ToPGUUID(weekID))
	if err != nil {
		return fmt.Errorf("list program days: %w", err)
	}
	weekPG := pgconv.ToPGUUID(weekID)
	for i, row := range rows {
		if err := q.UpdateProgramDayOrder(ctx, sqlc.UpdateProgramDayOrderParams{
			ID:        row.ID,
			WeekID:    weekPG,
			SortOrder: int32(i + 1),
			DayNumber: row.DayNumber,
		}); err != nil {
			return fmt.Errorf("normalize day sort: %w", err)
		}
	}
	return nil
}

func sortProgramDetail(d *Detail) {
	sort.Slice(d.Weeks, func(i, j int) bool {
		if d.Weeks[i].SortOrder == d.Weeks[j].SortOrder {
			return d.Weeks[i].ID.String() < d.Weeks[j].ID.String()
		}
		return d.Weeks[i].SortOrder < d.Weeks[j].SortOrder
	})
	for wi := range d.Weeks {
		sort.Slice(d.Weeks[wi].Days, func(i, j int) bool {
			if d.Weeks[wi].Days[i].SortOrder == d.Weeks[wi].Days[j].SortOrder {
				return d.Weeks[wi].Days[i].ID.String() < d.Weeks[wi].Days[j].ID.String()
			}
			return d.Weeks[wi].Days[i].SortOrder < d.Weeks[wi].Days[j].SortOrder
		})
		for di := range d.Weeks[wi].Days {
			sort.Slice(d.Weeks[wi].Days[di].Blocks, func(i, j int) bool {
				if d.Weeks[wi].Days[di].Blocks[i].SortOrder == d.Weeks[wi].Days[di].Blocks[j].SortOrder {
					return d.Weeks[wi].Days[di].Blocks[i].ID.String() < d.Weeks[wi].Days[di].Blocks[j].ID.String()
				}
				return d.Weeks[wi].Days[di].Blocks[i].SortOrder < d.Weeks[wi].Days[di].Blocks[j].SortOrder
			})
			for bi := range d.Weeks[wi].Days[di].Blocks {
				ex := d.Weeks[wi].Days[di].Blocks[bi].Exercises
				sort.Slice(ex, func(i, j int) bool {
					if ex[i].SortOrder == ex[j].SortOrder {
						return ex[i].ID.String() < ex[j].ID.String()
					}
					return ex[i].SortOrder < ex[j].SortOrder
				})
				d.Weeks[wi].Days[di].Blocks[bi].Exercises = ex
			}
		}
	}
}
