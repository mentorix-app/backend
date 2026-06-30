package program

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) CreateDraft(ctx context.Context, userID uuid.UUID) (Detail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	userPG := pgconv.ToPGUUID(userID)

	programID, err := qtx.InsertProgramDraft(ctx, sqlc.InsertProgramDraftParams{
		CreatedBy:  userPG,
		Status:     string(StatusDraft),
		ModifiedAt: now,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("insert program: %w", err)
	}

	weekID, err := qtx.InsertProgramWeek(ctx, sqlc.InsertProgramWeekParams{
		ProgramID:  programID,
		WeekNumber: 1,
		SortOrder:  1,
		ModifiedAt: now,
		ModifiedBy: userPG,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("insert program week: %w", err)
	}

	if err := insertDaysForWeek(ctx, qtx, programID, weekID, 1, DefaultWeekDays); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, pgconv.FromPGUUID(programID))
}

func insertDaysForWeek(ctx context.Context, q *sqlc.Queries, programPG, weekID pgtype.UUID, startDayNumber, count int) error {
	for i := 0; i < count; i++ {
		dayNum := int32(startDayNumber + i)
		if _, err := q.InsertProgramDay(ctx, sqlc.InsertProgramDayParams{
			ProgramID: programPG,
			WeekID:    weekID,
			DayNumber: dayNum,
			SortOrder: dayNum,
		}); err != nil {
			return fmt.Errorf("insert program day: %w", err)
		}
	}
	return nil
}

func (s *Store) List(ctx context.Context, params ListParams) (ListResult, error) {
	filter := listFilterParams(params)

	total, err := s.q.CountPrograms(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("count programs: %w", err)
	}

	rows, err := s.q.ListPrograms(ctx, sqlc.ListProgramsParams{
		FilterCreatedBy:  filter.FilterCreatedBy,
		QPattern:         filter.QPattern,
		FilterStatuses:   filter.FilterStatuses,
		FilterCategory:   filter.FilterCategory,
		FilterDifficulty: filter.FilterDifficulty,
		SortBy:           params.SortBy,
		SortOrder:        params.SortOrder,
		Offset:           int32(params.Offset()),
		Limit:            int32(params.Limit),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list programs: %w", err)
	}

	items := make([]Program, 0, len(rows))
	for _, row := range rows {
		p := programFromListRow(row)
		p, err = s.enrichProgram(ctx, p, nil)
		if err != nil {
			return ListResult{}, err
		}
		items = append(items, p)
	}

	return ListResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, int(total)),
	}, nil
}

func (s *Store) GetProgramRow(ctx context.Context, id uuid.UUID) (Program, error) {
	row, err := s.q.GetProgramByID(ctx, pgconv.ToPGUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Program{}, pgx.ErrNoRows
		}
		return Program{}, fmt.Errorf("get program: %w", err)
	}
	return programFromGetRow(row), nil
}

func (s *Store) GetDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	d, err := s.loadDetail(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	p, err := s.enrichProgram(ctx, d.Program, &d)
	if err != nil {
		return Detail{}, err
	}
	d.Program = p
	return d, nil
}

func (s *Store) loadDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	p, err := s.GetProgramRow(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	weeks, err := s.listWeeksWithDaysAndExercises(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Program: p, Weeks: weeks}, nil
}

func (s *Store) listWeeksWithDaysAndExercises(ctx context.Context, programID uuid.UUID) ([]Week, error) {
	programPG := pgconv.ToPGUUID(programID)
	weekRows, err := s.q.ListProgramWeeks(ctx, programPG)
	if err != nil {
		return nil, fmt.Errorf("list program weeks: %w", err)
	}

	weeks := make([]Week, 0, len(weekRows))
	for _, w := range weekRows {
		weekID := pgconv.FromPGUUID(w.ID)
		days, err := s.listDaysWithExercises(ctx, weekID)
		if err != nil {
			return nil, err
		}
		weeks = append(weeks, Week{
			ID:         weekID,
			WeekNumber: int(w.WeekNumber),
			SortOrder:  int(w.SortOrder),
			Days:       days,
			CreatedAt:  w.CreatedAt.UTC(),
		})
	}
	return weeks, nil
}

func (s *Store) listDaysWithExercises(ctx context.Context, weekID uuid.UUID) ([]Day, error) {
	weekPG := pgconv.ToPGUUID(weekID)
	dayRows, err := s.q.ListProgramDaysForWeek(ctx, weekPG)
	if err != nil {
		return nil, fmt.Errorf("list program days: %w", err)
	}

	days := make([]Day, 0, len(dayRows))
	for _, d := range dayRows {
		dayID := pgconv.FromPGUUID(d.ID)
		exercises, err := s.listDayExercises(ctx, dayID)
		if err != nil {
			return nil, err
		}
		days = append(days, Day{
			ID:        dayID,
			DayNumber: int(d.DayNumber),
			SortOrder: int(d.SortOrder),
			Exercises: exercises,
			CreatedAt: d.CreatedAt.UTC(),
		})
	}
	return days, nil
}

func (s *Store) listDayExercises(ctx context.Context, dayID uuid.UUID) ([]DayExercise, error) {
	rows, err := s.q.ListDayExercises(ctx, pgconv.ToPGUUID(dayID))
	if err != nil {
		return nil, fmt.Errorf("list day exercises: %w", err)
	}

	out := make([]DayExercise, 0, len(rows))
	for _, row := range rows {
		out = append(out, dayExerciseFromRow(row))
	}
	return out, nil
}

func (s *Store) Update(ctx context.Context, id, userID uuid.UUID, in UpdateInput) (Detail, error) {
	if err := in.Validate(); err != nil {
		return Detail{}, err
	}

	var category, difficulty *string
	if in.Category != nil {
		v := string(*in.Category)
		category = &v
	}
	if in.Difficulty != nil {
		v := string(*in.Difficulty)
		difficulty = &v
	}

	rows, err := s.q.UpdateProgram(ctx, sqlc.UpdateProgramParams{
		ID:              pgconv.ToPGUUID(id),
		ModifiedBy:      pgconv.ToPGUUID(userID),
		ModifiedAt:      time.Now().UTC(),
		Name:            in.Name,
		NameRu:          in.NameRu,
		Description:     in.Description,
		DescriptionRu:   in.DescriptionRu,
		Category:        category,
		Difficulty:      difficulty,
		PreviewImageUrl: in.PreviewImageURL,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("update program: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, id)
}

func (s *Store) SetStatus(ctx context.Context, id, userID uuid.UUID, status Status) (Detail, error) {
	rows, err := s.q.SetProgramStatus(ctx, sqlc.SetProgramStatusParams{
		ID:         pgconv.ToPGUUID(id),
		Status:     string(status),
		ModifiedBy: pgconv.ToPGUUID(userID),
		ModifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("set program status: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, id)
}

func (s *Store) SoftDelete(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now().UTC()
	rows, err := s.q.SoftDeleteProgram(ctx, sqlc.SoftDeleteProgramParams{
		ID:         pgconv.ToPGUUID(id),
		DeletedAt:  pgtype.Timestamptz{Time: now, Valid: true},
		ModifiedBy: pgconv.ToPGUUID(userID),
	})
	if err != nil {
		return fmt.Errorf("soft delete program: %w", err)
	}
	if rows == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) AddWeek(ctx context.Context, programID uuid.UUID) (Detail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	programPG := pgconv.ToPGUUID(programID)
	nums, err := qtx.NextProgramWeekNumbers(ctx, programPG)
	if err != nil {
		return Detail{}, fmt.Errorf("next week number: %w", err)
	}

	now := time.Now().UTC()
	weekID, err := qtx.InsertProgramWeek(ctx, sqlc.InsertProgramWeekParams{
		ProgramID:  programPG,
		WeekNumber: nums.NextWeekNumber,
		SortOrder:  nums.NextSortOrder,
		ModifiedAt: now,
		ModifiedBy: pgtype.UUID{},
	})
	if err != nil {
		return Detail{}, fmt.Errorf("insert program week: %w", err)
	}

	if err := insertDaysForWeek(ctx, qtx, programPG, weekID, 1, DefaultWeekDays); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteWeek(ctx context.Context, programID, weekID uuid.UUID) (Detail, error) {
	count, err := s.q.CountProgramWeeks(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return Detail{}, fmt.Errorf("count program weeks: %w", err)
	}
	if count <= 1 {
		return Detail{}, ErrLastWeek
	}

	rows, err := s.q.DeleteProgramWeek(ctx, sqlc.DeleteProgramWeekParams{
		ID:        pgconv.ToPGUUID(weekID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delete program week: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) AddDay(ctx context.Context, programID, weekID uuid.UUID) (Detail, error) {
	ok, err := s.WeekBelongsToProgram(ctx, programID, weekID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	weekPG := pgconv.ToPGUUID(weekID)
	count, err := s.q.CountProgramDaysForWeek(ctx, weekPG)
	if err != nil {
		return Detail{}, fmt.Errorf("count program days: %w", err)
	}
	if count >= DefaultWeekDays {
		return Detail{}, ErrMaxDaysPerWeek
	}

	programPG := pgconv.ToPGUUID(programID)
	nums, err := s.q.NextProgramDayNumbersForWeek(ctx, weekPG)
	if err != nil {
		return Detail{}, fmt.Errorf("next day number: %w", err)
	}

	if err := s.q.InsertProgramDayAuto(ctx, sqlc.InsertProgramDayAutoParams{
		ProgramID: programPG,
		WeekID:    weekPG,
		DayNumber: nums.NextDayNumber,
		SortOrder: nums.NextSortOrder,
	}); err != nil {
		return Detail{}, fmt.Errorf("insert program day: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDay(ctx context.Context, programID, weekID, dayID uuid.UUID) (Detail, error) {
	count, err := s.q.CountProgramDaysForWeek(ctx, pgconv.ToPGUUID(weekID))
	if err != nil {
		return Detail{}, fmt.Errorf("count program days: %w", err)
	}
	if count <= 1 {
		return Detail{}, ErrLastDay
	}

	rows, err := s.q.DeleteProgramDay(ctx, sqlc.DeleteProgramDayParams{
		ID:        pgconv.ToPGUUID(dayID),
		ProgramID: pgconv.ToPGUUID(programID),
		WeekID:    pgconv.ToPGUUID(weekID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delete program day: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) WeekBelongsToProgram(ctx context.Context, programID, weekID uuid.UUID) (bool, error) {
	ok, err := s.q.WeekBelongsToProgram(ctx, sqlc.WeekBelongsToProgramParams{
		ID:        pgconv.ToPGUUID(weekID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return false, fmt.Errorf("check program week: %w", err)
	}
	return ok, nil
}

func (s *Store) DayBelongsToWeek(ctx context.Context, programID, weekID, dayID uuid.UUID) (bool, error) {
	ok, err := s.q.DayBelongsToWeek(ctx, sqlc.DayBelongsToWeekParams{
		ID:        pgconv.ToPGUUID(dayID),
		WeekID:    pgconv.ToPGUUID(weekID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return false, fmt.Errorf("check program day: %w", err)
	}
	return ok, nil
}

func (s *Store) ExerciseExists(ctx context.Context, exerciseID uuid.UUID) (bool, error) {
	ok, err := s.q.ExerciseExists(ctx, pgconv.ToPGUUID(exerciseID))
	if err != nil {
		return false, fmt.Errorf("check exercise: %w", err)
	}
	return ok, nil
}

func (s *Store) AddDayExercise(ctx context.Context, userID, programID, weekID, dayID uuid.UUID, in DayExerciseInput) (Detail, error) {
	ok, err := s.DayBelongsToWeek(ctx, programID, weekID, dayID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	exists, err := s.ExerciseExists(ctx, in.ExerciseID)
	if err != nil {
		return Detail{}, err
	}
	if !exists {
		return Detail{}, fmt.Errorf("%w: exercise not found", ErrValidation)
	}

	dayPG := pgconv.ToPGUUID(dayID)
	nextSort, err := s.q.NextDayExerciseSort(ctx, dayPG)
	if err != nil {
		return Detail{}, fmt.Errorf("next exercise sort: %w", err)
	}

	if err := s.q.InsertDayExercise(ctx, dayExerciseInsertParams(dayPG, nextSort, userID, in)); err != nil {
		return Detail{}, fmt.Errorf("insert day exercise: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) UpdateDayExercise(ctx context.Context, userID, programID, weekID, dayID, itemID uuid.UUID, in DayExerciseInput) (Detail, error) {
	ok, err := s.DayBelongsToWeek(ctx, programID, weekID, dayID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	exists, err := s.ExerciseExists(ctx, in.ExerciseID)
	if err != nil {
		return Detail{}, err
	}
	if !exists {
		return Detail{}, fmt.Errorf("%w: exercise not found", ErrValidation)
	}

	now := time.Now().UTC()
	rows, err := s.q.UpdateDayExercise(ctx, sqlc.UpdateDayExerciseParams{
		ID:           pgconv.ToPGUUID(itemID),
		ProgramDayID: pgconv.ToPGUUID(dayID),
		ExerciseID:   pgconv.ToPGUUID(in.ExerciseID),
		Sets:         intPtrToInt32(in.Sets),
		Reps:         intPtrToInt32(in.Reps),
		WeightKg:     pgconv.ToNumeric(in.WeightKg),
		Instruction:  instructionString(in.Instruction),
		ModifiedAt:   now,
		ModifiedBy:   pgconv.ToPGUUID(userID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("update day exercise: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDayExercise(ctx context.Context, programID, weekID, dayID, itemID uuid.UUID) (Detail, error) {
	ok, err := s.DayBelongsToWeek(ctx, programID, weekID, dayID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	rows, err := s.q.DeleteDayExercise(ctx, sqlc.DeleteDayExerciseParams{
		ID:           pgconv.ToPGUUID(itemID),
		ProgramDayID: pgconv.ToPGUUID(dayID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delete day exercise: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) ReorderWeeks(ctx context.Context, programID uuid.UUID, weekIDs []uuid.UUID) (Detail, error) {
	if err := validateWeekReorder(ctx, s.q, programID, weekIDs); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	programPG := pgconv.ToPGUUID(programID)
	for i, weekID := range weekIDs {
		temp := int32(10000 + i)
		if err := qtx.UpdateProgramWeekOrder(ctx, sqlc.UpdateProgramWeekOrderParams{
			ID:         pgconv.ToPGUUID(weekID),
			ProgramID:  programPG,
			SortOrder:  temp,
			WeekNumber: temp,
		}); err != nil {
			return Detail{}, fmt.Errorf("stage week order: %w", err)
		}
	}
	for i, weekID := range weekIDs {
		pos := int32(i + 1)
		if err := qtx.UpdateProgramWeekOrder(ctx, sqlc.UpdateProgramWeekOrderParams{
			ID:         pgconv.ToPGUUID(weekID),
			ProgramID:  programPG,
			SortOrder:  pos,
			WeekNumber: pos,
		}); err != nil {
			return Detail{}, fmt.Errorf("update week order: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) ReorderDays(ctx context.Context, programID, weekID uuid.UUID, dayIDs []uuid.UUID) (Detail, error) {
	ok, err := s.WeekBelongsToProgram(ctx, programID, weekID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	if err := validateDayReorder(ctx, s.q, weekID, dayIDs); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	weekPG := pgconv.ToPGUUID(weekID)
	for i, dayID := range dayIDs {
		temp := int32(10000 + i)
		if err := qtx.UpdateProgramDayOrder(ctx, sqlc.UpdateProgramDayOrderParams{
			ID:        pgconv.ToPGUUID(dayID),
			WeekID:    weekPG,
			SortOrder: temp,
			DayNumber: temp,
		}); err != nil {
			return Detail{}, fmt.Errorf("stage day order: %w", err)
		}
	}
	for i, dayID := range dayIDs {
		pos := int32(i + 1)
		if err := qtx.UpdateProgramDayOrder(ctx, sqlc.UpdateProgramDayOrderParams{
			ID:        pgconv.ToPGUUID(dayID),
			WeekID:    weekPG,
			SortOrder: pos,
			DayNumber: pos,
		}); err != nil {
			return Detail{}, fmt.Errorf("update day order: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) ReorderWeekExercises(ctx context.Context, userID, programID, weekID uuid.UUID, days []WeekExerciseReorderDay) (Detail, error) {
	ok, err := s.WeekBelongsToProgram(ctx, programID, weekID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	if err := validateExerciseReorder(ctx, s.q, programID, weekID, days); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	userPG := pgconv.ToPGUUID(userID)
	for _, day := range days {
		for i, itemID := range day.ExerciseItemIDs {
			if err := qtx.UpdateDayExercisePlacement(ctx, sqlc.UpdateDayExercisePlacementParams{
				ID:           pgconv.ToPGUUID(itemID),
				ProgramDayID: pgconv.ToPGUUID(day.DayID),
				SortOrder:    int32(i + 1),
				ModifiedAt:   now,
				ModifiedBy:   userPG,
			}); err != nil {
				return Detail{}, fmt.Errorf("update exercise placement: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func validateWeekReorder(ctx context.Context, q *sqlc.Queries, programID uuid.UUID, weekIDs []uuid.UUID) error {
	existing, err := q.ListProgramWeekIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list program weeks: %w", err)
	}
	return validateReorderIDs(existing, weekIDs, "week")
}

func validateDayReorder(ctx context.Context, q *sqlc.Queries, weekID uuid.UUID, dayIDs []uuid.UUID) error {
	existing, err := q.ListProgramDayIDsForWeek(ctx, pgconv.ToPGUUID(weekID))
	if err != nil {
		return fmt.Errorf("list program days: %w", err)
	}
	return validateReorderIDs(existing, dayIDs, "day")
}

func validateExerciseReorder(ctx context.Context, q *sqlc.Queries, programID, weekID uuid.UUID, days []WeekExerciseReorderDay) error {
	items, err := q.ListExerciseItemsByWeek(ctx, pgconv.ToPGUUID(weekID))
	if err != nil {
		return fmt.Errorf("list week exercises: %w", err)
	}

	expected := make(map[uuid.UUID]uuid.UUID, len(items))
	for _, item := range items {
		expected[pgconv.FromPGUUID(item.ID)] = pgconv.FromPGUUID(item.ProgramDayID)
	}

	if len(expected) == 0 && len(days) == 0 {
		return nil
	}

	seen := make(map[uuid.UUID]struct{}, len(expected))
	for _, day := range days {
		ok, err := q.DayBelongsToWeek(ctx, sqlc.DayBelongsToWeekParams{
			ID:        pgconv.ToPGUUID(day.DayID),
			WeekID:    pgconv.ToPGUUID(weekID),
			ProgramID: pgconv.ToPGUUID(programID),
		})
		if err != nil {
			return fmt.Errorf("check program day: %w", err)
		}
		if !ok {
			return fmt.Errorf("%w: day does not belong to week", ErrInvalidReorder)
		}

		for _, itemID := range day.ExerciseItemIDs {
			if _, exists := expected[itemID]; !exists {
				return fmt.Errorf("%w: unknown exercise item", ErrInvalidReorder)
			}
			if _, dup := seen[itemID]; dup {
				return fmt.Errorf("%w: duplicate exercise item", ErrInvalidReorder)
			}
			seen[itemID] = struct{}{}
		}
	}

	if len(seen) != len(expected) {
		return fmt.Errorf("%w: exercise items must include all items in week", ErrInvalidReorder)
	}
	return nil
}

func validateReorderIDs(existing []pgtype.UUID, got []uuid.UUID, label string) error {
	if len(got) != len(existing) {
		return fmt.Errorf("%w: %s count mismatch", ErrInvalidReorder, label)
	}

	expected := make(map[uuid.UUID]struct{}, len(existing))
	for _, id := range existing {
		expected[pgconv.FromPGUUID(id)] = struct{}{}
	}

	seen := make(map[uuid.UUID]struct{}, len(got))
	for _, id := range got {
		if _, ok := expected[id]; !ok {
			return fmt.Errorf("%w: unknown %s", ErrInvalidReorder, label)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: duplicate %s", ErrInvalidReorder, label)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func listFilterParams(params ListParams) sqlc.CountProgramsParams {
	var filterCreatedBy pgtype.UUID
	if params.CreatedBy != nil {
		filterCreatedBy = pgconv.ToPGUUID(*params.CreatedBy)
	}

	var qPattern *string
	if params.Query != "" {
		p := "%" + escapeLike(params.Query) + "%"
		qPattern = &p
	}

	var filterStatuses []string
	if len(params.Statuses) > 0 {
		filterStatuses = make([]string, len(params.Statuses))
		for i, st := range params.Statuses {
			filterStatuses[i] = string(st)
		}
	}

	var filterCategory *string
	if params.Category != nil {
		v := string(*params.Category)
		filterCategory = &v
	}

	var filterDifficulty *string
	if params.Difficulty != nil {
		v := string(*params.Difficulty)
		filterDifficulty = &v
	}

	return sqlc.CountProgramsParams{
		FilterCreatedBy:  filterCreatedBy,
		QPattern:         qPattern,
		FilterStatuses:   filterStatuses,
		FilterCategory:   filterCategory,
		FilterDifficulty: filterDifficulty,
	}
}

func programFromFields(
	id, createdBy, modifiedBy pgtype.UUID,
	createdByName, status, name, nameRu, description, descriptionRu, previewImageURL string,
	category, difficulty *string,
	createdAt, modifiedAt time.Time,
	deletedAt pgtype.Timestamptz,
) Program {
	var cat *Category
	if category != nil {
		c := Category(*category)
		cat = &c
	}
	var diff *Difficulty
	if difficulty != nil {
		d := Difficulty(*difficulty)
		diff = &d
	}
	var deleted *time.Time
	if deletedAt.Valid {
		t := deletedAt.Time.UTC()
		deleted = &t
	}
	return Program{
		ID:              pgconv.FromPGUUID(id),
		CreatedBy:       pgconv.FromPGUUID(createdBy),
		CreatedByName:   createdByName,
		ModifiedBy:      pgconv.FromPGUUID(modifiedBy),
		Status:          Status(status),
		Name:            name,
		NameRu:          nameRu,
		Description:     description,
		DescriptionRu:   descriptionRu,
		Category:        cat,
		Difficulty:      diff,
		PreviewImageURL: previewImageURL,
		CreatedAt:       createdAt.UTC(),
		ModifiedAt:      modifiedAt.UTC(),
		DeletedAt:       deleted,
	}
}

func programFromGetRow(row sqlc.GetProgramByIDRow) Program {
	return programFromFields(
		row.ID, row.CreatedBy, row.ModifiedBy,
		row.CreatedByName, row.Status, row.Name, row.NameRu, row.Description, row.DescriptionRu, row.PreviewImageUrl,
		row.Category, row.Difficulty,
		row.CreatedAt, row.ModifiedAt,
		row.DeletedAt,
	)
}

func programFromListRow(row sqlc.ListProgramsRow) Program {
	return programFromFields(
		row.ID, row.CreatedBy, row.ModifiedBy,
		row.CreatedByName, row.Status, row.Name, row.NameRu, row.Description, row.DescriptionRu, row.PreviewImageUrl,
		row.Category, row.Difficulty,
		row.CreatedAt, row.ModifiedAt,
		row.DeletedAt,
	)
}

func dayExerciseFromRow(row sqlc.ListDayExercisesRow) DayExercise {
	return DayExercise{
		ID:             pgconv.FromPGUUID(row.ID),
		ExerciseID:     pgconv.FromPGUUID(row.ExerciseID),
		ExerciseName:   row.Name,
		ExerciseNameRu: row.NameRu,
		SortOrder:      int(row.SortOrder),
		Sets:           int32PtrToInt(row.Sets),
		Reps:           int32PtrToInt(row.Reps),
		WeightKg:       pgconv.FromNumeric(row.WeightKg),
		Instruction:    row.Instruction,
		CreatedAt:      row.CreatedAt.UTC(),
	}
}

func dayExerciseInsertParams(dayPG pgtype.UUID, sort int32, userID uuid.UUID, in DayExerciseInput) sqlc.InsertDayExerciseParams {
	now := time.Now().UTC()
	return sqlc.InsertDayExerciseParams{
		ProgramDayID: dayPG,
		ExerciseID:   pgconv.ToPGUUID(in.ExerciseID),
		SortOrder:    sort,
		Sets:         intPtrToInt32(in.Sets),
		Reps:         intPtrToInt32(in.Reps),
		WeightKg:     pgconv.ToNumeric(in.WeightKg),
		Instruction:  instructionString(in.Instruction),
		ModifiedAt:   now,
		ModifiedBy:   pgconv.ToPGUUID(userID),
	}
}

func instructionString(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func intPtrToInt32(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

func int32PtrToInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}
