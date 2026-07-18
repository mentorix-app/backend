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
	"mentorix-backend/internal/subscription"
)

// Viewer describes catalog visibility for the acting user.
type Viewer struct {
	// TrainerID is set for users with a trainer profile: they see global
	// exercises plus their own private ones.
	TrainerID *uuid.UUID
	// IncludeAll is set for admins: they see every exercise.
	IncludeAll bool
}

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) TrainerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		return uuid.Nil, err
	}
	return pgconv.FromPGUUID(id), nil
}

func (s *Store) List(ctx context.Context, viewer Viewer, params ListParams) (ListResult, error) {
	filter := listFilterParams(viewer, params)

	total, err := s.q.CountExercises(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("count exercises: %w", err)
	}

	rows, err := s.q.ListExercises(ctx, sqlc.ListExercisesParams{
		IncludeAll:        filter.IncludeAll,
		ViewerTrainerID:   filter.ViewerTrainerID,
		FilterScope:       filter.FilterScope,
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

// Create inserts a global exercise (ownerTrainerID nil) without quota checks,
// or a trainer-private exercise under the exercise quota with a trainer lock.
func (s *Store) Create(ctx context.Context, userID uuid.UUID, ownerTrainerID *uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	params := upsertParams(userID, now, in)
	if ownerTrainerID == nil {
		row, err := s.q.CreateExercise(ctx, params)
		if err != nil {
			return Exercise{}, fmt.Errorf("insert exercise: %w", err)
		}
		return s.GetByID(ctx, pgconv.FromPGUUID(row.ID))
	}

	params.OwnerTrainerID = pgconv.ToPGUUID(*ownerTrainerID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Exercise{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := subscription.LockTrainer(ctx, qtx, *ownerTrainerID); err != nil {
		return Exercise{}, err
	}
	if err := subscription.CheckQuota(ctx, qtx, *ownerTrainerID, subscription.ResourceExercises, subscription.OpCreate); err != nil {
		return Exercise{}, err
	}
	row, err := qtx.CreateExercise(ctx, params)
	if err != nil {
		return Exercise{}, fmt.Errorf("insert exercise: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Exercise{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetByID(ctx, pgconv.FromPGUUID(row.ID))
}

// Update mutates an exercise scoped by owner: nil targets global exercises,
// a trainer id targets that trainer's private exercises only.
func (s *Store) Update(ctx context.Context, id, userID uuid.UUID, ownerTrainerID *uuid.UUID, in UpsertInput) (Exercise, error) {
	now := time.Now().UTC()
	params := upsertParams(userID, now, in)
	var ownerPG pgtype.UUID
	if ownerTrainerID != nil {
		ownerPG = pgconv.ToPGUUID(*ownerTrainerID)
	}
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
		OwnerTrainerID:  ownerPG,
	})
	if err != nil {
		return Exercise{}, fmt.Errorf("update exercise: %w", err)
	}
	if rows == 0 {
		return Exercise{}, pgx.ErrNoRows
	}
	return s.GetByID(ctx, id)
}

// DeleteMany soft-deletes exercises within the given owner scope.
func (s *Store) DeleteMany(ctx context.Context, userID uuid.UUID, ownerTrainerID *uuid.UUID, ids []uuid.UUID) (int64, error) {
	now := time.Now().UTC()
	var ownerPG pgtype.UUID
	if ownerTrainerID != nil {
		ownerPG = pgconv.ToPGUUID(*ownerTrainerID)
	}
	n, err := s.q.SoftDeleteExercises(ctx, sqlc.SoftDeleteExercisesParams{
		Column1:        pgconv.UUIDSlice(ids),
		DeletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
		ModifiedBy:     pgconv.ToPGUUID(userID),
		OwnerTrainerID: ownerPG,
	})
	if err != nil {
		return 0, fmt.Errorf("soft delete exercises: %w", err)
	}
	return n, nil
}

func listFilterParams(viewer Viewer, params ListParams) sqlc.CountExercisesParams {
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

	var filterScope *string
	if params.Scope != nil {
		v := string(*params.Scope)
		filterScope = &v
	}

	var includeAll *bool
	if viewer.IncludeAll {
		t := true
		includeAll = &t
	}
	var viewerTrainerPG pgtype.UUID
	if viewer.TrainerID != nil {
		viewerTrainerPG = pgconv.ToPGUUID(*viewer.TrainerID)
	}

	return sqlc.CountExercisesParams{
		IncludeAll:        includeAll,
		ViewerTrainerID:   viewerTrainerPG,
		FilterScope:       filterScope,
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
	ownerTrainerID, ownerUserID pgtype.UUID,
) Exercise {
	var eq *Equipment
	if equipment != nil {
		e := Equipment(*equipment)
		eq = &e
	}
	ex := Exercise{
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
		Scope:           ScopeGlobal,
	}
	if ownerTrainerID.Valid {
		ex.Scope = ScopePrivate
		trainerID := pgconv.FromPGUUID(ownerTrainerID)
		ex.ownerTrainerID = &trainerID
		if ownerUserID.Valid {
			ownerUser := pgconv.FromPGUUID(ownerUserID)
			ex.OwnerUserID = &ownerUser
		}
	}
	return ex
}

func exerciseFromGetRow(row sqlc.GetExerciseByIDRow) Exercise {
	return exerciseFromFields(
		row.ID, row.CreatedBy, row.ModifiedBy,
		row.Name, row.NameRu, row.CreatedByName,
		row.ModifiedAt, row.CreatedAt,
		row.Equipment, row.ExerciseType, row.MuscleGroup,
		row.Description, row.DescriptionRu, row.Difficulty,
		row.VideoUrl, row.PreviewImageUrl,
		row.OwnerTrainerID, row.OwnerUserID,
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
		row.OwnerTrainerID, row.OwnerUserID,
	)
}
