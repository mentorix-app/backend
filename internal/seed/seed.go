package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

type Result struct {
	UserCreated      bool
	ExercisesCreated int
	ProgramsCreated  int
	ExerciseTotal    int
	ProgramTotal     int
}

func Run(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	authStore := auth.NewStore(pool)
	exStore := exercise.NewStore(pool)
	progSvc := program.NewService(pool)
	q := sqlc.New(pool)

	userID, created, err := ensureUser(ctx, authStore, q)
	if err != nil {
		return Result{}, err
	}

	exCreated, exByName, exTotal, err := ensureExercises(ctx, exStore, userID)
	if err != nil {
		return Result{}, fmt.Errorf("exercises: %w", err)
	}

	progCreated, progTotal, err := ensurePrograms(ctx, progSvc, userID, exByName)
	if err != nil {
		return Result{}, fmt.Errorf("programs: %w", err)
	}

	return Result{
		UserCreated:      created,
		ExercisesCreated: exCreated,
		ProgramsCreated:  progCreated,
		ExerciseTotal:    exTotal,
		ProgramTotal:     progTotal,
	}, nil
}

func ensureUser(ctx context.Context, authStore *auth.Store, q *sqlc.Queries) (uuid.UUID, bool, error) {
	hash, err := auth.HashPassword(DevPassword)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("hash password: %w", err)
	}

	userID, err := authStore.RegisterTrainerEmailPassword(ctx, DevEmail, hash, DevDisplayName)
	if err == nil {
		if err := authStore.GrantRole(ctx, userID, auth.RoleAdmin); err != nil {
			return uuid.Nil, false, fmt.Errorf("grant admin: %w", err)
		}
		return userID, true, nil
	}
	if !errors.Is(err, auth.ErrEmailTaken) {
		return uuid.Nil, false, fmt.Errorf("register: %w", err)
	}

	row, err := q.GetEmailPasswordIdentity(ctx, sqlc.GetEmailPasswordIdentityParams{
		Provider: auth.ProviderEmailPassword,
		Subject:  DevEmail,
	})
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("lookup user: %w", err)
	}
	userID = pgconv.FromPGUUID(row.UserID)
	if err := authStore.UpdateUserDisplayName(ctx, userID, DevDisplayName); err != nil {
		return uuid.Nil, false, fmt.Errorf("update display name: %w", err)
	}
	if err := authStore.GrantRole(ctx, userID, auth.RoleAdmin); err != nil {
		return uuid.Nil, false, fmt.Errorf("grant admin: %w", err)
	}
	return userID, false, nil
}

func ensureExercises(ctx context.Context, store *exercise.Store, userID uuid.UUID) (created int, byName map[string]uuid.UUID, total int, err error) {
	byName = make(map[string]uuid.UUID)
	catalog := exerciseCatalog()

	for _, in := range catalog {
		exists, id, err := exerciseExistsByName(ctx, store, in.Name)
		if err != nil {
			return 0, nil, 0, err
		}
		if exists {
			byName[in.Name] = id
			continue
		}
		ex, err := store.Create(ctx, userID, in)
		if err != nil {
			return 0, nil, 0, fmt.Errorf("create %q: %w", in.Name, err)
		}
		byName[in.Name] = ex.ID
		created++
	}

	params, err := exercise.ParseListParams("1", "100", "name", "asc", "", "", "", "", "")
	if err != nil {
		return 0, nil, 0, err
	}
	list, err := store.List(ctx, params)
	if err != nil {
		return 0, nil, 0, err
	}
	total = list.Pagination.Total
	return created, byName, total, nil
}

func exerciseExistsByName(ctx context.Context, store *exercise.Store, name string) (bool, uuid.UUID, error) {
	params, err := exercise.ParseListParams("1", "100", "name", "asc", name, "", "", "", "")
	if err != nil {
		return false, uuid.Nil, err
	}
	result, err := store.List(ctx, params)
	if err != nil {
		return false, uuid.Nil, err
	}
	for _, item := range result.Items {
		if item.Name == name {
			return true, item.ID, nil
		}
	}
	return false, uuid.Nil, nil
}

func ensurePrograms(ctx context.Context, svc *program.Service, userID uuid.UUID, exByName map[string]uuid.UUID) (created int, total int, err error) {
	for _, spec := range programCatalog() {
		var exists bool
		var err error
		switch {
		case spec.SkipMetadata:
			exists, err = emptyDraftExists(ctx, svc, userID)
		case spec.Name != "":
			exists, err = programExistsByName(ctx, svc, userID, spec.Name)
		default:
			exists, err = emptyDraftExists(ctx, svc, userID)
		}
		if err != nil {
			return 0, 0, err
		}
		if exists {
			continue
		}

		if err := createProgram(ctx, svc, userID, spec, exByName); err != nil {
			return 0, 0, fmt.Errorf("create program %q: %w", spec.Name, err)
		}
		created++
	}

	params, err := program.ParseListParams("1", "100", "created_at", "desc", "", "", "", "")
	if err != nil {
		return 0, 0, err
	}
	list, err := svc.List(ctx, userID, params)
	if err != nil {
		return 0, 0, err
	}
	return created, list.Pagination.Total, nil
}

func programExistsByName(ctx context.Context, svc *program.Service, userID uuid.UUID, name string) (bool, error) {
	params, err := program.ParseListParams("1", "100", "name", "asc", name, "", "", "")
	if err != nil {
		return false, err
	}
	result, err := svc.List(ctx, userID, params)
	if err != nil {
		return false, err
	}
	for _, item := range result.Items {
		if item.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func emptyDraftExists(ctx context.Context, svc *program.Service, userID uuid.UUID) (bool, error) {
	params, err := program.ParseListParams("1", "100", "created_at", "desc", "", "draft", "", "")
	if err != nil {
		return false, err
	}
	result, err := svc.List(ctx, userID, params)
	if err != nil {
		return false, err
	}
	for _, item := range result.Items {
		if item.Name == "" && item.Status == program.StatusDraft {
			return true, nil
		}
	}
	return false, nil
}

func createProgram(ctx context.Context, svc *program.Service, userID uuid.UUID, spec programSeed, exByName map[string]uuid.UUID) error {
	draft, err := svc.Create(ctx, userID)
	if err != nil {
		return err
	}

	if !spec.SkipMetadata {
		in := program.UpdateInput{
			Name:          strPtr(spec.Name),
			NameRu:        strPtr(spec.NameRu),
			Description:   strPtr(spec.Description),
			DescriptionRu: strPtr(spec.DescriptionRu),
			Category:      spec.Category,
			Difficulty:    spec.Difficulty,
		}
		if _, err := svc.Update(ctx, userID, draft.ID, in); err != nil {
			return err
		}
	}

	if len(spec.ExerciseNames) > 0 {
		detail, err := svc.Get(ctx, userID, draft.ID)
		if err != nil {
			return err
		}
		if len(detail.Weeks) == 0 || len(detail.Weeks[0].Days) == 0 {
			return fmt.Errorf("program has no days")
		}
		weekID := detail.Weeks[0].ID
		dayID := detail.Weeks[0].Days[0].ID
		for i, exName := range spec.ExerciseNames {
			exID, ok := exByName[exName]
			if !ok {
				return fmt.Errorf("exercise %q not found", exName)
			}
			sets, reps := 3, 8+i%3
			_, err := svc.CreateDayBlock(ctx, userID, draft.ID, weekID, dayID, program.CreateDayBlockInput{
				BlockType: program.BlockTypeSingle,
				Exercise: &program.DayExerciseInput{
					ExerciseID: exID,
					Sets:       &sets,
					Reps:       &reps,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	switch spec.TargetStatus {
	case programStatusDraft:
		return nil
	case programStatusPublished:
		_, err := svc.Publish(ctx, userID, draft.ID)
		return err
	case programStatusArchived:
		if _, err := svc.Publish(ctx, userID, draft.ID); err != nil {
			return err
		}
		_, err := svc.Archive(ctx, userID, draft.ID)
		return err
	default:
		return fmt.Errorf("unknown target status %q", spec.TargetStatus)
	}
}

func strPtr(s string) *string {
	return &s
}
