package exercise

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

func (s *Store) List(ctx context.Context, params ListParams) (ListResult, error) {
	filter := listFilterParams(params)

	total, err := s.q.CountExercises(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("count exercises: %w", err)
	}

	rows, err := s.q.ListExercises(ctx, sqlc.ListExercisesParams{
		QPattern:          filter.QPattern,
		FilterType:        filter.FilterType,
		FilterMuscleGroup: filter.FilterMuscleGroup,
		FilterDifficulty:  filter.FilterDifficulty,
		EquipmentIsNull:   filter.EquipmentIsNull,
		FilterEquipment:   filter.FilterEquipment,
		SortBy:            params.SortBy,
		SortOrder:         params.SortOrder,
		Offset:            int32(params.Offset()),
		Limit:             int32(params.Limit),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list exercises: %w", err)
	}

	items := make([]Exercise, 0, len(rows))
	for _, row := range rows {
		items = append(items, exerciseFromListRow(row))
	}

	return ListResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, int(total)),
	}, nil
}

func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (Exercise, error) {
	row, err := s.q.GetExerciseByID(ctx, pgconv.ToPGUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exercise{}, pgx.ErrNoRows
		}
		return Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return exerciseFromGetRow(row), nil
}

func (s *Store) Create(ctx context.Context, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	row, err := s.q.CreateExercise(ctx, upsertParams(userID, now, in))
	if err != nil {
		return Exercise{}, fmt.Errorf("insert exercise: %w", err)
	}
	return s.GetByID(ctx, pgconv.FromPGUUID(row.ID))
}

func (s *Store) Update(ctx context.Context, id, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	params := upsertParams(userID, now, in)
	rows, err := s.q.UpdateExercise(ctx, sqlc.UpdateExerciseParams{
		ID:              pgconv.ToPGUUID(id),
		Name:            params.Name,
		NameRu:          params.NameRu,
		ModifiedBy:      params.ModifiedBy,
		ModifiedAt:      params.ModifiedAt,
		Equipment:       params.Equipment,
		ExerciseType:    params.ExerciseType,
		MuscleGroup:     params.MuscleGroup,
		Description:     params.Description,
		DescriptionRu:   params.DescriptionRu,
		Difficulty:      params.Difficulty,
		VideoUrl:        params.VideoUrl,
		PreviewImageUrl: params.PreviewImageUrl,
	})
	if err != nil {
		return Exercise{}, fmt.Errorf("update exercise: %w", err)
	}
	if rows == 0 {
		return Exercise{}, pgx.ErrNoRows
	}
	return s.GetByID(ctx, id)
}

func (s *Store) DeleteMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	now := time.Now().UTC()
	n, err := s.q.SoftDeleteExercises(ctx, sqlc.SoftDeleteExercisesParams{
		Column1:    pgconv.UUIDSlice(ids),
		DeletedAt:  pgtype.Timestamptz{Time: now, Valid: true},
		ModifiedBy: pgconv.ToPGUUID(userID),
	})
	if err != nil {
		return 0, fmt.Errorf("soft delete exercises: %w", err)
	}
	return n, nil
}

func listFilterParams(params ListParams) sqlc.CountExercisesParams {
	var qPattern *string
	if params.Query != "" {
		p := "%" + escapeLike(params.Query) + "%"
		qPattern = &p
	}

	var filterType *string
	if params.Type != nil {
		v := string(*params.Type)
		filterType = &v
	}

	var filterMuscleGroup *string
	if params.MuscleGroup != nil {
		v := string(*params.MuscleGroup)
		filterMuscleGroup = &v
	}

	var filterDifficulty *string
	if params.Difficulty != nil {
		v := string(*params.Difficulty)
		filterDifficulty = &v
	}

	var equipmentIsNull *bool
	if params.EquipmentIsNull {
		t := true
		equipmentIsNull = &t
	}

	var filterEquipment *string
	if params.Equipment != nil {
		v := string(*params.Equipment)
		filterEquipment = &v
	}

	return sqlc.CountExercisesParams{
		QPattern:          qPattern,
		FilterType:        filterType,
		FilterMuscleGroup: filterMuscleGroup,
		FilterDifficulty:  filterDifficulty,
		EquipmentIsNull:   equipmentIsNull,
		FilterEquipment:   filterEquipment,
	}
}

func upsertParams(userID uuid.UUID, now time.Time, in UpsertInput) sqlc.CreateExerciseParams {
	var equipment *string
	if in.Equipment != nil {
		v := string(*in.Equipment)
		equipment = &v
	}
	userPG := pgconv.ToPGUUID(userID)
	return sqlc.CreateExerciseParams{
		Name:            in.Name,
		NameRu:          in.NameRu,
		CreatedBy:       userPG,
		ModifiedBy:      userPG,
		ModifiedAt:      now,
		Equipment:       equipment,
		ExerciseType:    string(in.Type),
		MuscleGroup:     string(in.MuscleGroup),
		Description:     in.Description,
		DescriptionRu:   in.DescriptionRu,
		Difficulty:      string(in.Difficulty),
		VideoUrl:        in.VideoURL,
		PreviewImageUrl: in.PreviewImageURL,
	}
}

func exerciseFromFields(
	id, createdBy, modifiedBy pgtype.UUID,
	name, nameRu, createdByName string,
	modifiedAt, createdAt time.Time,
	equipment *string,
	exerciseType, muscleGroup, description, descriptionRu, difficulty, videoURL, previewImageURL string,
) Exercise {
	var eq *Equipment
	if equipment != nil {
		e := Equipment(*equipment)
		eq = &e
	}
	return Exercise{
		ID:              pgconv.FromPGUUID(id),
		Name:            name,
		NameRu:          nameRu,
		CreatedBy:       pgconv.FromPGUUID(createdBy),
		CreatedByName:   createdByName,
		ModifiedBy:      pgconv.FromPGUUID(modifiedBy),
		ModifiedAt:      modifiedAt.UTC(),
		CreatedAt:       createdAt.UTC(),
		Equipment:       eq,
		Type:            ExerciseType(exerciseType),
		MuscleGroup:     MuscleGroup(muscleGroup),
		Description:     description,
		DescriptionRu:   descriptionRu,
		Difficulty:      Difficulty(difficulty),
		VideoURL:        videoURL,
		PreviewImageURL: previewImageURL,
	}
}

func exerciseFromGetRow(row sqlc.GetExerciseByIDRow) Exercise {
	return exerciseFromFields(
		row.ID, row.CreatedBy, row.ModifiedBy,
		row.Name, row.NameRu, row.CreatedByName,
		row.ModifiedAt, row.CreatedAt,
		row.Equipment, row.ExerciseType, row.MuscleGroup,
		row.Description, row.DescriptionRu, row.Difficulty,
		row.VideoUrl, row.PreviewImageUrl,
	)
}

func exerciseFromListRow(row sqlc.ListExercisesRow) Exercise {
	return exerciseFromFields(
		row.ID, row.CreatedBy, row.ModifiedBy,
		row.Name, row.NameRu, row.CreatedByName,
		row.ModifiedAt, row.CreatedAt,
		row.Equipment, row.ExerciseType, row.MuscleGroup,
		row.Description, row.DescriptionRu, row.Difficulty,
		row.VideoUrl, row.PreviewImageUrl,
	)
}
