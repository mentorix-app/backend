package program

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func (s *Store) AddBlockExercise(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	block, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	if block.BlockType == string(BlockTypeSingle) {
		return Detail{}, fmt.Errorf("%w: cannot add exercise to single block", ErrValidation)
	}

	exists, err := s.ExerciseExists(ctx, in.ExerciseID)
	if err != nil {
		return Detail{}, err
	}
	if !exists {
		return Detail{}, fmt.Errorf("%w: exercise not found", ErrValidation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	blockPG := pgconv.ToPGUUID(blockID)
	nextSort, err := qtx.NextBlockExerciseSort(ctx, blockPG)
	if err != nil {
		return Detail{}, fmt.Errorf("next exercise sort: %w", err)
	}
	if err := qtx.InsertBlockExercise(ctx, blockExerciseInsertParams(blockPG, nextSort, userID, in)); err != nil {
		return Detail{}, fmt.Errorf("insert block exercise: %w", err)
	}
	if err := normalizeBlockExerciseSort(ctx, qtx, blockID); err != nil {
		return Detail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) PatchDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID, in BlockPatchInput) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	block, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	var blockType *string
	if in.BlockType != nil {
		v := string(*in.BlockType)
		blockType = &v
	}
	rows, err := s.q.UpdateDayBlock(ctx, sqlc.UpdateDayBlockParams{
		ID:           pgconv.ToPGUUID(blockID),
		ProgramWeekDayID: block.ProgramWeekDayID,
		BlockType:    blockType,
		Instruction:  in.Instruction,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("update day block: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) MergeDayBlocks(ctx context.Context, programID, weekID, dayID uuid.UUID, blockIDs []uuid.UUID) (Detail, error) {
	if len(blockIDs) < 2 {
		return Detail{}, fmt.Errorf("%w: at least two blocks required", ErrValidation)
	}
	ok, err := s.DayBelongsToWeek(ctx, programID, weekID, dayID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	blocks, err := s.loadMergeBlocks(ctx, dayID, blockIDs)
	if err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	primary := blocks[0]
	mergedInstruction := mergeBlockInstructions(blocks)

	blockType := string(BlockTypeComplex)
	if _, err := qtx.UpdateDayBlock(ctx, sqlc.UpdateDayBlockParams{
		ID:           pgconv.ToPGUUID(primary.ID),
		ProgramWeekDayID: pgconv.ToPGUUID(dayID),
		BlockType:    &blockType,
		Instruction:  &mergedInstruction,
	}); err != nil {
		return Detail{}, fmt.Errorf("update primary block: %w", err)
	}

	sortOrder := int32(1)
	for _, block := range blocks {
		exercises, err := qtx.ListBlockExercises(ctx, pgconv.ToPGUUID(block.ID))
		if err != nil {
			return Detail{}, fmt.Errorf("list block exercises: %w", err)
		}
		for _, ex := range exercises {
			if err := qtx.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
				ID:                ex.ID,
				ProgramWeekDayBlockID: pgconv.ToPGUUID(primary.ID),
				SortOrder:         sortOrder,
				ModifiedAt:        time.Now().UTC(),
				ModifiedBy:        pgtype.UUID{},
			}); err != nil {
				return Detail{}, fmt.Errorf("move exercise to merged block: %w", err)
			}
			sortOrder++
		}
	}

	for _, block := range blocks[1:] {
		if _, err := qtx.DeleteDayBlock(ctx, sqlc.DeleteDayBlockParams{
			ID:           pgconv.ToPGUUID(block.ID),
			ProgramWeekDayID: pgconv.ToPGUUID(dayID),
		}); err != nil {
			return Detail{}, fmt.Errorf("delete merged block: %w", err)
		}
	}

	if err := normalizeDayBlockSort(ctx, qtx, dayID); err != nil {
		return Detail{}, err
	}
	if err := normalizeBlockExerciseSort(ctx, qtx, primary.ID); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) UngroupDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	block, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	dayID := pgconv.FromPGUUID(block.ProgramWeekDayID)
	if block.BlockType == string(BlockTypeSingle) {
		return Detail{}, fmt.Errorf("%w: block is already single", ErrValidation)
	}

	exercises, err := s.q.ListBlockExercises(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		return Detail{}, fmt.Errorf("list block exercises: %w", err)
	}
	if len(exercises) == 0 {
		return Detail{}, fmt.Errorf("%w: cannot ungroup empty block", ErrValidation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	dayPG := pgconv.ToPGUUID(dayID)

	ids, err := listDayBlockUUIDs(ctx, qtx, dayID)
	if err != nil {
		return Detail{}, err
	}
	groupIdx := -1
	for i, id := range ids {
		if id == blockID {
			groupIdx = i
			break
		}
	}
	if groupIdx < 0 {
		return Detail{}, pgx.ErrNoRows
	}

	newBlockIDs := make([]uuid.UUID, 0, len(exercises))
	for _, ex := range exercises {
		newBlockID, err := qtx.InsertDayBlock(ctx, sqlc.InsertDayBlockParams{
			ProgramWeekDayID: dayPG,
			BlockType:    string(BlockTypeSingle),
			Instruction:  "",
			SortOrder:    1,
		})
		if err != nil {
			return Detail{}, fmt.Errorf("insert single block: %w", err)
		}
		newBlockIDs = append(newBlockIDs, pgconv.FromPGUUID(newBlockID))
		if err := qtx.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
			ID:                ex.ID,
			ProgramWeekDayBlockID: newBlockID,
			SortOrder:         1,
			ModifiedAt:        time.Now().UTC(),
			ModifiedBy:        pgtype.UUID{},
		}); err != nil {
			return Detail{}, fmt.Errorf("move exercise to single block: %w", err)
		}
	}

	ids = append(ids[:groupIdx], append(newBlockIDs, ids[groupIdx+1:]...)...)
	if err := applyDayBlockOrder(ctx, qtx, dayID, ids); err != nil {
		return Detail{}, err
	}

	if _, err := qtx.DeleteDayBlock(ctx, sqlc.DeleteDayBlockParams{
		ID:           pgconv.ToPGUUID(blockID),
		ProgramWeekDayID: dayPG,
	}); err != nil {
		return Detail{}, fmt.Errorf("delete ungrouped block: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) MoveDayBlock(ctx context.Context, programID, weekID, blockID, targetDayID uuid.UUID, insertSort int) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}
	ok, err = s.DayBelongsToWeek(ctx, programID, weekID, targetDayID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	block, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	sourceDayID := pgconv.FromPGUUID(block.ProgramWeekDayID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if sourceDayID != targetDayID {
		sourceIDs, err := listDayBlockUUIDs(ctx, qtx, sourceDayID)
		if err != nil {
			return Detail{}, err
		}
		sourceIDs = removeUUID(sourceIDs, blockID)
		if err := applyDayBlockOrder(ctx, qtx, sourceDayID, sourceIDs); err != nil {
			return Detail{}, err
		}
	}
	if err := insertBlockIntoDayOrder(ctx, qtx, targetDayID, blockID, insertSort); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) ExtractBlockExercise(ctx context.Context, userID, programID, weekID, blockID, itemID uuid.UUID, insertSort int) (Detail, error) {
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

	meta, err := s.q.GetBlockExerciseMeta(ctx, pgconv.ToPGUUID(itemID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get exercise meta: %w", err)
	}
	if meta.BlockType == string(BlockTypeSingle) {
		return Detail{}, fmt.Errorf("%w: exercise is already in a single block", ErrValidation)
	}
	dayID := pgconv.FromPGUUID(meta.ProgramWeekDayID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	dayPG := pgconv.ToPGUUID(dayID)
	newBlockID, err := qtx.InsertDayBlock(ctx, sqlc.InsertDayBlockParams{
		ProgramWeekDayID: dayPG,
		BlockType:    string(BlockTypeSingle),
		Instruction:  "",
		SortOrder:    1,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("insert single block: %w", err)
	}

	now := time.Now().UTC()
	if err := qtx.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
		ID:                pgconv.ToPGUUID(itemID),
		ProgramWeekDayBlockID: newBlockID,
		SortOrder:         1,
		ModifiedAt:        now,
		ModifiedBy:        pgconv.ToPGUUID(userID),
	}); err != nil {
		return Detail{}, fmt.Errorf("extract exercise: %w", err)
	}

	if err := insertBlockIntoDayOrder(ctx, qtx, dayID, pgconv.FromPGUUID(newBlockID), insertSort); err != nil {
		return Detail{}, err
	}
	if err := normalizeBlockExerciseSort(ctx, qtx, pgconv.FromPGUUID(meta.ProgramWeekDayBlockID)); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) MoveExerciseToBlock(ctx context.Context, userID, programID, weekID, blockID, itemID, targetBlockID uuid.UUID) (Detail, error) {
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

	ok, err = s.blockBelongsToWeek(ctx, programID, weekID, targetBlockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	target, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(targetBlockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get target block: %w", err)
	}
	if target.BlockType == string(BlockTypeSingle) {
		return Detail{}, fmt.Errorf("%w: cannot move exercise into single block", ErrValidation)
	}

	meta, err := s.q.GetBlockExerciseMeta(ctx, pgconv.ToPGUUID(itemID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get exercise meta: %w", err)
	}
	if pgconv.FromPGUUID(meta.ProgramWeekDayBlockID) == targetBlockID {
		return s.GetDetail(ctx, programID)
	}

	nextSort, err := s.q.NextBlockExerciseSort(ctx, pgconv.ToPGUUID(targetBlockID))
	if err != nil {
		return Detail{}, fmt.Errorf("next exercise sort: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	if err := qtx.UpdateBlockExercisePlacement(ctx, sqlc.UpdateBlockExercisePlacementParams{
		ID:                pgconv.ToPGUUID(itemID),
		ProgramWeekDayBlockID: pgconv.ToPGUUID(targetBlockID),
		SortOrder:         nextSort,
		ModifiedAt:        now,
		ModifiedBy:        pgconv.ToPGUUID(userID),
	}); err != nil {
		return Detail{}, fmt.Errorf("move exercise to block: %w", err)
	}

	sourceBlockID := pgconv.FromPGUUID(meta.ProgramWeekDayBlockID)
	if meta.BlockType == string(BlockTypeSingle) {
		if _, err := qtx.DeleteDayBlock(ctx, sqlc.DeleteDayBlockParams{
			ID:           meta.ProgramWeekDayBlockID,
			ProgramWeekDayID: meta.ProgramWeekDayID,
		}); err != nil {
			return Detail{}, fmt.Errorf("delete empty single block: %w", err)
		}
	} else if err := normalizeBlockExerciseSort(ctx, qtx, sourceBlockID); err != nil {
		return Detail{}, err
	}
	if err := normalizeBlockExerciseSort(ctx, qtx, targetBlockID); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDayBlock(ctx context.Context, programID, weekID, blockID uuid.UUID) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	block, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	dayID := pgconv.FromPGUUID(block.ProgramWeekDayID)
	if block.BlockType == string(BlockTypeSingle) {
		return Detail{}, fmt.Errorf("%w: use delete exercise for single blocks", ErrValidation)
	}

	rows, err := s.q.DeleteDayBlock(ctx, sqlc.DeleteDayBlockParams{
		ID:           pgconv.ToPGUUID(blockID),
		ProgramWeekDayID: pgconv.ToPGUUID(dayID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delete day block: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	if err := normalizeDayBlockSort(ctx, s.q, dayID); err != nil {
		return Detail{}, err
	}
	return s.GetDetail(ctx, programID)
}

type mergeBlock struct {
	ID          uuid.UUID
	BlockType   BlockType
	Instruction string
	SortOrder   int
}

func (s *Store) loadMergeBlocks(ctx context.Context, dayID uuid.UUID, blockIDs []uuid.UUID) ([]mergeBlock, error) {
	rows, err := s.q.ListDayBlocks(ctx, pgconv.ToPGUUID(dayID))
	if err != nil {
		return nil, fmt.Errorf("list day blocks: %w", err)
	}

	byID := make(map[uuid.UUID]mergeBlock, len(rows))
	for _, row := range rows {
		id := pgconv.FromPGUUID(row.ID)
		byID[id] = mergeBlock{
			ID:          id,
			BlockType:   BlockType(row.BlockType),
			Instruction: row.Instruction,
			SortOrder:   int(row.SortOrder),
		}
	}

	out := make([]mergeBlock, 0, len(blockIDs))
	for _, id := range blockIDs {
		b, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: unknown block", ErrValidation)
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out, nil
}

func mergeBlockInstructions(blocks []mergeBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if !b.BlockType.isGroup() {
			continue
		}
		text := strings.TrimSpace(b.Instruction)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
