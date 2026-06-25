package program

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db"
)

const programSelectColumns = `
	id, created_by, modified_by, status, name, description, category, difficulty,
	preview_image_url, created_at, modified_at, deleted_at`

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+db.Table("user_roles")+` WHERE user_id = $1 AND role = $2)`,
		userID, auth.RoleAdmin,
	).Scan(&ok)
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

	now := time.Now().UTC()
	var programID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO `+db.Table("programs")+` (
			created_by, modified_by, status, modified_at
		) VALUES ($1, $1, $2, $3)
		RETURNING id`,
		userID, string(StatusDraft), now,
	).Scan(&programID)
	if err != nil {
		return Detail{}, fmt.Errorf("insert program: %w", err)
	}

	var dayID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO `+db.Table("program_days")+` (program_id, day_number, sort_order)
		VALUES ($1, 1, 1)
		RETURNING id`, programID,
	).Scan(&dayID)
	if err != nil {
		return Detail{}, fmt.Errorf("insert program day: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) List(ctx context.Context, params ListParams) (ListResult, error) {
	where, args := buildListWhere(params)
	total, err := s.countPrograms(ctx, where, args)
	if err != nil {
		return ListResult{}, err
	}

	listArgs := append(append([]any{}, args...), params.Limit, params.Offset())
	query := fmt.Sprintf(`
		SELECT %s
		FROM `+db.Table("programs")+`
		%s
		ORDER BY %s %s NULLS LAST
		LIMIT $%d OFFSET $%d`,
		programSelectColumns,
		where,
		params.SortBy,
		params.SortOrder,
		len(args)+1,
		len(args)+2,
	)

	rows, err := s.pool.Query(ctx, query, listArgs...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list programs: %w", err)
	}
	defer rows.Close()

	items, err := scanPrograms(rows)
	if err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, total),
	}, nil
}

func buildListWhere(params ListParams) (string, []any) {
	var conditions []string
	var args []any

	conditions = append(conditions, "deleted_at IS NULL")
	if params.CreatedBy != nil {
		args = append(args, *params.CreatedBy)
		conditions = append(conditions, fmt.Sprintf("created_by = $%d", len(args)))
	}
	if params.Query != "" {
		pattern := "%" + escapeLike(params.Query) + "%"
		args = append(args, pattern)
		n := len(args)
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d ESCAPE '\\'", n))
	}
	if len(params.Statuses) > 0 {
		statuses := make([]string, len(params.Statuses))
		for i, st := range params.Statuses {
			statuses[i] = string(st)
		}
		args = append(args, statuses)
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if params.Category != nil {
		args = append(args, string(*params.Category))
		conditions = append(conditions, fmt.Sprintf("category = $%d", len(args)))
	}
	if params.Difficulty != nil {
		args = append(args, string(*params.Difficulty))
		conditions = append(conditions, fmt.Sprintf("difficulty = $%d", len(args)))
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func (s *Store) countPrograms(ctx context.Context, where string, args []any) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s %s`, db.Table("programs"), where)
	var total int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count programs: %w", err)
	}
	return total, nil
}

func (s *Store) GetProgramRow(ctx context.Context, id uuid.UUID) (Program, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM `+db.Table("programs")+` WHERE id = $1`, programSelectColumns), id)
	p, err := scanProgram(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Program{}, pgx.ErrNoRows
		}
		return Program{}, fmt.Errorf("get program: %w", err)
	}
	return p, nil
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
	dayRows, err := s.pool.Query(ctx, `
		SELECT id, day_number, sort_order, created_at
		FROM `+db.Table("program_days")+`
		WHERE program_id = $1
		ORDER BY sort_order ASC, day_number ASC`, programID)
	if err != nil {
		return nil, fmt.Errorf("list program days: %w", err)
	}
	defer dayRows.Close()

	var days []Day
	for dayRows.Next() {
		var d Day
		if err := dayRows.Scan(&d.ID, &d.DayNumber, &d.SortOrder, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan program day: %w", err)
		}
		d.CreatedAt = d.CreatedAt.UTC()
		days = append(days, d)
	}
	if err := dayRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate program days: %w", err)
	}
	if days == nil {
		days = []Day{}
	}

	for i := range days {
		exercises, err := s.listDayExercises(ctx, days[i].ID)
		if err != nil {
			return nil, err
		}
		days[i].Exercises = exercises
	}
	return days, nil
}

func (s *Store) listDayExercises(ctx context.Context, dayID uuid.UUID) ([]DayExercise, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			pde.id, pde.exercise_id, e.name, e.name_ru, pde.sort_order,
			pde.sets, pde.reps, pde.weight_kg, pde.instruction, pde.created_at
		FROM `+db.Table("program_day_exercises")+` pde
		JOIN `+db.Table("exercises")+` e ON e.id = pde.exercise_id
		WHERE pde.program_day_id = $1
		ORDER BY pde.sort_order ASC, pde.created_at ASC`, dayID)
	if err != nil {
		return nil, fmt.Errorf("list day exercises: %w", err)
	}
	defer rows.Close()

	var out []DayExercise
	for rows.Next() {
		var ex DayExercise
		if err := rows.Scan(
			&ex.ID, &ex.ExerciseID, &ex.ExerciseName, &ex.ExerciseNameRu, &ex.SortOrder,
			&ex.Sets, &ex.Reps, &ex.WeightKg, &ex.Instruction, &ex.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan day exercise: %w", err)
		}
		ex.CreatedAt = ex.CreatedAt.UTC()
		out = append(out, ex)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate day exercises: %w", err)
	}
	if out == nil {
		out = []DayExercise{}
	}
	return out, nil
}

func (s *Store) Update(ctx context.Context, id, userID uuid.UUID, in UpdateInput) (Detail, error) {
	if err := in.Validate(); err != nil {
		return Detail{}, err
	}

	sets := []string{"modified_by = $2", "modified_at = $3"}
	args := []any{id, userID, time.Now().UTC()}
	argN := 4

	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argN))
		args = append(args, *in.Name)
		argN++
	}
	if in.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argN))
		args = append(args, *in.Description)
		argN++
	}
	if in.Category != nil {
		sets = append(sets, fmt.Sprintf("category = $%d", argN))
		args = append(args, string(*in.Category))
		argN++
	}
	if in.Difficulty != nil {
		sets = append(sets, fmt.Sprintf("difficulty = $%d", argN))
		args = append(args, string(*in.Difficulty))
		argN++
	}
	if in.PreviewImageURL != nil {
		sets = append(sets, fmt.Sprintf("preview_image_url = $%d", argN))
		args = append(args, *in.PreviewImageURL)
	}

	query := fmt.Sprintf(`
		UPDATE %s SET %s
		WHERE id = $1 AND deleted_at IS NULL`,
		db.Table("programs"), strings.Join(sets, ", "),
	)
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return Detail{}, fmt.Errorf("update program: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, id)
}

func (s *Store) SetStatus(ctx context.Context, id, userID uuid.UUID, status Status) (Detail, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE `+db.Table("programs")+` SET
			status = $2, modified_by = $3, modified_at = $4
		WHERE id = $1 AND deleted_at IS NULL`,
		id, string(status), userID, time.Now().UTC(),
	)
	if err != nil {
		return Detail{}, fmt.Errorf("set program status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, id)
}

func (s *Store) SoftDelete(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE `+db.Table("programs")+` SET
			deleted_at = $2, modified_by = $3, modified_at = $2
		WHERE id = $1 AND deleted_at IS NULL`,
		id, now, userID,
	)
	if err != nil {
		return fmt.Errorf("soft delete program: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) AddDay(ctx context.Context, programID uuid.UUID) (Detail, error) {
	var nextDay, nextSort int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(day_number), 0) + 1, COALESCE(MAX(sort_order), 0) + 1
		FROM `+db.Table("program_days")+`
		WHERE program_id = $1`, programID,
	).Scan(&nextDay, &nextSort)
	if err != nil {
		return Detail{}, fmt.Errorf("next day number: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO `+db.Table("program_days")+` (program_id, day_number, sort_order)
		VALUES ($1, $2, $3)`, programID, nextDay, nextSort)
	if err != nil {
		return Detail{}, fmt.Errorf("insert program day: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DeleteDay(ctx context.Context, programID, dayID uuid.UUID) (Detail, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM `+db.Table("program_days")+`
		WHERE id = $1 AND program_id = $2`, dayID, programID)
	if err != nil {
		return Detail{}, fmt.Errorf("delete program day: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

func (s *Store) DayBelongsToProgram(ctx context.Context, programID, dayID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM `+db.Table("program_days")+`
			WHERE id = $1 AND program_id = $2
		)`, dayID, programID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check program day: %w", err)
	}
	return ok, nil
}

func (s *Store) ExerciseExists(ctx context.Context, exerciseID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+db.Table("exercises")+` WHERE id = $1)`, exerciseID,
	).Scan(&ok)
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

	var nextSort int
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), 0) + 1
		FROM `+db.Table("program_day_exercises")+`
		WHERE program_day_id = $1`, dayID,
	).Scan(&nextSort)
	if err != nil {
		return Detail{}, fmt.Errorf("next exercise sort: %w", err)
	}

	instruction := ""
	if in.Instruction != nil {
		instruction = *in.Instruction
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO `+db.Table("program_day_exercises")+` (
			program_day_id, exercise_id, sort_order, sets, reps, weight_kg, instruction
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		dayID, in.ExerciseID, nextSort, in.Sets, in.Reps, in.WeightKg, instruction,
	)
	if err != nil {
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

	instruction := ""
	if in.Instruction != nil {
		instruction = *in.Instruction
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE `+db.Table("program_day_exercises")+` SET
			exercise_id = $3, sets = $4, reps = $5, weight_kg = $6, instruction = $7
		WHERE id = $1 AND program_day_id = $2`,
		itemID, dayID, in.ExerciseID, in.Sets, in.Reps, in.WeightKg, instruction,
	)
	if err != nil {
		return Detail{}, fmt.Errorf("update day exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
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

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM `+db.Table("program_day_exercises")+`
		WHERE id = $1 AND program_day_id = $2`, itemID, dayID)
	if err != nil {
		return Detail{}, fmt.Errorf("delete day exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Detail{}, pgx.ErrNoRows
	}
	return s.GetDetail(ctx, programID)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanProgram(row scannable) (Program, error) {
	var p Program
	var status string
	var category, difficulty *string
	err := row.Scan(
		&p.ID, &p.CreatedBy, &p.ModifiedBy, &status, &p.Name, &p.Description,
		&category, &difficulty, &p.PreviewImageURL,
		&p.CreatedAt, &p.ModifiedAt, &p.DeletedAt,
	)
	if err != nil {
		return Program{}, err
	}
	p.Status = Status(status)
	if category != nil {
		c := Category(*category)
		p.Category = &c
	}
	if difficulty != nil {
		d := Difficulty(*difficulty)
		p.Difficulty = &d
	}
	p.CreatedAt = p.CreatedAt.UTC()
	p.ModifiedAt = p.ModifiedAt.UTC()
	if p.DeletedAt != nil {
		t := p.DeletedAt.UTC()
		p.DeletedAt = &t
	}
	return p, nil
}

func scanPrograms(rows pgx.Rows) ([]Program, error) {
	var out []Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate programs: %w", err)
	}
	if out == nil {
		out = []Program{}
	}
	return out, nil
}
