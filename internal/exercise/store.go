package exercise

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) List(ctx context.Context) ([]Exercise, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, name_ru, added_by, modified_by, modified_at, created_at,
		       equipment, type, muscle_group, description, description_ru,
		       difficulty, video_url, preview_image_url
		FROM mentorix.exercises
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	defer rows.Close()
	return scanExercises(rows)
}

func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (Exercise, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, name_ru, added_by, modified_by, modified_at, created_at,
		       equipment, type, muscle_group, description, description_ru,
		       difficulty, video_url, preview_image_url
		FROM mentorix.exercises
		WHERE id = $1`, id)
	ex, err := scanExercise(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exercise{}, pgx.ErrNoRows
		}
		return Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return ex, nil
}

func (s *Store) Create(ctx context.Context, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	var equipment *string
	if in.Equipment != nil {
		v := string(*in.Equipment)
		equipment = &v
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mentorix.exercises (
			name, name_ru, added_by, modified_by, modified_at,
			equipment, type, muscle_group, description, description_ru,
			difficulty, video_url, preview_image_url
		) VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		in.Name, in.NameRu, userID, now, equipment, string(in.Type), string(in.MuscleGroup),
		in.Description, in.DescriptionRu, string(in.Difficulty), in.VideoURL, in.PreviewImageURL,
	).Scan(&id)
	if err != nil {
		return Exercise{}, fmt.Errorf("insert exercise: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *Store) Update(ctx context.Context, id, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	var equipment *string
	if in.Equipment != nil {
		v := string(*in.Equipment)
		equipment = &v
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE mentorix.exercises SET
			name = $2, name_ru = $3, modified_by = $4, modified_at = $5,
			equipment = $6, type = $7, muscle_group = $8, description = $9, description_ru = $10,
			difficulty = $11, video_url = $12, preview_image_url = $13
		WHERE id = $1`,
		id, in.Name, in.NameRu, userID, now, equipment, string(in.Type), string(in.MuscleGroup),
		in.Description, in.DescriptionRu, string(in.Difficulty), in.VideoURL, in.PreviewImageURL,
	)
	if err != nil {
		return Exercise{}, fmt.Errorf("update exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Exercise{}, pgx.ErrNoRows
	}
	return s.GetByID(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mentorix.exercises WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanExercise(row scannable) (Exercise, error) {
	var ex Exercise
	var equipment *string
	var muscleGroup, typ, difficulty string
	err := row.Scan(
		&ex.ID, &ex.Name, &ex.NameRu, &ex.AddedBy, &ex.ModifiedBy, &ex.ModifiedAt, &ex.CreatedAt,
		&equipment, &typ, &muscleGroup, &ex.Description, &ex.DescriptionRu,
		&difficulty, &ex.VideoURL, &ex.PreviewImageURL,
	)
	if err != nil {
		return Exercise{}, err
	}
	if equipment != nil {
		e := Equipment(*equipment)
		ex.Equipment = &e
	}
	ex.Type = ExerciseType(typ)
	ex.MuscleGroup = MuscleGroup(muscleGroup)
	ex.Difficulty = Difficulty(difficulty)
	ex.ModifiedAt = ex.ModifiedAt.UTC()
	ex.CreatedAt = ex.CreatedAt.UTC()
	return ex, nil
}

func scanExercises(rows pgx.Rows) ([]Exercise, error) {
	var out []Exercise
	for rows.Next() {
		ex, err := scanExercise(rows)
		if err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		out = append(out, ex)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercises: %w", err)
	}
	if out == nil {
		out = []Exercise{}
	}
	return out, nil
}
