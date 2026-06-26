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

	"mentorix-backend/internal/auth"
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

func (s *Store) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	ok, err := s.q.UserHasAnyRole(ctx, sqlc.UserHasAnyRoleParams{
		UserID: pgconv.ToPGUUID(userID),
		Roles:  []string{auth.RoleAdmin},
	})
	if err != nil {
		return false, fmt.Errorf("check admin role: %w", err)
	}
	return ok, nil
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

	_, err = qtx.InsertProgramDay(ctx, sqlc.InsertProgramDayParams{
		ProgramID: programID,
		DayNumber: 1,
		SortOrder: 1,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("insert program day: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, pgconv.FromPGUUID(programID))
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
		items = append(items, programFromRow(row))
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
	return programFromRow(row), nil
}

func (s *Store) GetDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	p, err := s.GetProgramRow(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	days, err := s.listDaysWithExercises(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Program: p, Days: days}, nil
}

func (s *Store) listDaysWithExercises(ctx context.Context, programID uuid.UUID) ([]Day, error) {
	programPG := pgconv.ToPGUUID(programID)
	dayRows, err := s.q.ListProgramDays(ctx, programPG)
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
		Description:     in.Description,
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

func (s *Store) AddDay(ctx context.Context, programID uuid.UUID) (Detail, error) {
	programPG := pgconv.ToPGUUID(programID)
	nums, err := s.q.NextProgramDayNumbers(ctx, programPG)
	if err != nil {
		return Detail{}, fmt.Errorf("next day number: %w", err)
	}

	if err := s.q.InsertProgramDayAuto(ctx, sqlc.InsertProgramDayAutoParams{
		ProgramID: programPG,
		DayNumber: nums.NextDayNumber,
		SortOrder: nums.NextSortOrder,
	}); err != nil {
		return Detail{}, fmt.Errorf("insert program day: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDay(ctx context.Context, programID, dayID uuid.UUID) (Detail, error) {
	rows, err := s.q.DeleteProgramDay(ctx, sqlc.DeleteProgramDayParams{
		ID:        pgconv.ToPGUUID(dayID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delete program day: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DayBelongsToProgram(ctx context.Context, programID, dayID uuid.UUID) (bool, error) {
	ok, err := s.q.DayBelongsToProgram(ctx, sqlc.DayBelongsToProgramParams{
		ID:        pgconv.ToPGUUID(dayID),
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

func (s *Store) AddDayExercise(ctx context.Context, programID, dayID uuid.UUID, in DayExerciseInput) (Detail, error) {
	ok, err := s.DayBelongsToProgram(ctx, programID, dayID)
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

	if err := s.q.InsertDayExercise(ctx, dayExerciseInsertParams(dayPG, nextSort, in)); err != nil {
		return Detail{}, fmt.Errorf("insert day exercise: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) UpdateDayExercise(ctx context.Context, programID, dayID, itemID uuid.UUID, in DayExerciseInput) (Detail, error) {
	ok, err := s.DayBelongsToProgram(ctx, programID, dayID)
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

	rows, err := s.q.UpdateDayExercise(ctx, sqlc.UpdateDayExerciseParams{
		ID:           pgconv.ToPGUUID(itemID),
		ProgramDayID: pgconv.ToPGUUID(dayID),
		ExerciseID:   pgconv.ToPGUUID(in.ExerciseID),
		Sets:         intPtrToInt32(in.Sets),
		Reps:         intPtrToInt32(in.Reps),
		WeightKg:     pgconv.ToNumeric(in.WeightKg),
		Instruction:  instructionString(in.Instruction),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("update day exercise: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDayExercise(ctx context.Context, programID, dayID, itemID uuid.UUID) (Detail, error) {
	ok, err := s.DayBelongsToProgram(ctx, programID, dayID)
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

func programFromRow(row sqlc.MentorixProgram) Program {
	var category *Category
	if row.Category != nil {
		c := Category(*row.Category)
		category = &c
	}
	var difficulty *Difficulty
	if row.Difficulty != nil {
		d := Difficulty(*row.Difficulty)
		difficulty = &d
	}
	var deletedAt *time.Time
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time.UTC()
		deletedAt = &t
	}
	return Program{
		ID:              pgconv.FromPGUUID(row.ID),
		CreatedBy:       pgconv.FromPGUUID(row.CreatedBy),
		ModifiedBy:      pgconv.FromPGUUID(row.ModifiedBy),
		Status:          Status(row.Status),
		Name:            row.Name,
		Description:     row.Description,
		Category:        category,
		Difficulty:      difficulty,
		PreviewImageURL: row.PreviewImageUrl,
		CreatedAt:       row.CreatedAt.UTC(),
		ModifiedAt:      row.ModifiedAt.UTC(),
		DeletedAt:       deletedAt,
	}
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

func dayExerciseInsertParams(dayPG pgtype.UUID, sort int32, in DayExerciseInput) sqlc.InsertDayExerciseParams {
	return sqlc.InsertDayExerciseParams{
		ProgramDayID: dayPG,
		ExerciseID:   pgconv.ToPGUUID(in.ExerciseID),
		SortOrder:    sort,
		Sets:         intPtrToInt32(in.Sets),
		Reps:         intPtrToInt32(in.Reps),
		WeightKg:     pgconv.ToNumeric(in.WeightKg),
		Instruction:  instructionString(in.Instruction),
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
