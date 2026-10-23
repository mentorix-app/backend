//go:build integration

package storetest

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func trainersRowID(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM mentorix.trainers WHERE user_id = $1`, userID).Scan(&id); err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	return id
}

func newCoach(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	return newTrainer(t, pool, email, "Coach")
}

// activatedTrainer is a Google client who became a trainer in the app.
func activatedTrainer(t *testing.T, store *auth.Store, subject string) uuid.UUID {
	t.Helper()
	id := newClientWithIdentity(t, store, auth.ProviderGoogle, subject, "")
	if err := store.ActivateTrainer(context.Background(), id, "2026-10-01"); err != nil {
		t.Fatalf("activate trainer: %v", err)
	}
	return id
}

func execSQL(t *testing.T, pool *pgxpool.Pool, what, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("seed %s: %v", what, err)
	}
}

// seedTrainerOwnRows gives the trainer what goes away with the account: a Telegram sign-in,
// a refresh session, an active invite, an invite consumed by another person, and an
// admin-granted entitlement.
func seedTrainerOwnRows(t *testing.T, pool *pgxpool.Pool, store *auth.Store, trainer uuid.UUID, tag string) {
	t.Helper()
	trainerID := trainersRowID(t, pool, trainer)
	other := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-invitee-"+tag, "")
	if _, err := store.LinkTelegram(context.Background(), trainer, "tg-"+tag); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	if err := store.InsertRefreshSession(context.Background(), trainer, []byte("session-"+tag), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	execSQL(t, pool, "active invite", `
INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at) VALUES ($1, $2, now() + interval '1 day')`, trainerID, "active-"+tag)
	execSQL(t, pool, "consumed invite", `
INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at, consumed_at, consumed_by)
VALUES ($1, $2, now() + interval '1 day', now(), $3)`, trainerID, "consumed-"+tag, other)
	execSQL(t, pool, "admin entitlement", `
INSERT INTO mentorix.trainer_plan_entitlements (trainer_id, plan_code, source, status) VALUES ($1, 'elite', 'admin', 'active')`, trainerID)
}

// A trainer with no clients, programs, exercises or history is deleted with the rows that
// only exist because of the trainer: the profile, the role, the invites and the admin grant.
func TestAccountDeletion_trainerWithoutBusinessDataIsDeleted(t *testing.T) {
	tests := []struct {
		name string
		make func(t *testing.T, pool *pgxpool.Pool, store *auth.Store) uuid.UUID
		// roles and identities are the rows the account holds before the seed adds a Telegram.
		roles, identities int
	}{
		{"trainer-only", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store) uuid.UUID {
			return newCoach(t, pool, "coach-del@test.com")
		}, 1, 1},
		{"client who became a trainer", func(t *testing.T, _ *pgxpool.Pool, store *auth.Store) uuid.UUID {
			return activatedTrainer(t, store, "g-activated-del")
		}, 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			ctx := context.Background()
			base := seedDeletionBase(t, pool, "del-trainer")
			seedClientData(t, pool, base, newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-neighbour", ""), "neighbour")
			trainer := tt.make(t, pool, store)
			seedTrainerOwnRows(t, pool, store, trainer, "coach")
			baseBefore := rowsFingerprint(t, pool, "program_assignments", "trainer_id", base.trainerID)
			counts := tableCounts(t, pool)

			if has, err := store.TrainerHasBusinessData(ctx, trainer); err != nil || has {
				t.Fatalf("TrainerHasBusinessData() = %v, %v, want false", has, err)
			}
			telegramID, err := store.DeleteClientAccount(ctx, trainer, "")

			if err != nil {
				t.Fatalf("DeleteClientAccount() error = %v", err)
			}
			if telegramID != "tg-coach" {
				t.Errorf("telegram id = %q, want tg-coach", telegramID)
			}
			if userExists(t, pool, trainer) {
				t.Error("the user row is still there")
			}
			for key, n := range userReferences(t, pool, trainer) {
				if n != 0 {
					t.Errorf("%s still holds %d rows of the deleted trainer", key, n)
				}
			}
			assertTableDeltas(t, counts, tableCounts(t, pool), map[string]int{
				"users":                     -1,
				"trainers":                  -1,
				"user_roles":                -tt.roles,
				"auth_identities":           -(tt.identities + 1), // the sign-in and the Telegram
				"auth_refresh_sessions":     -1,
				"trainer_invites":           -2,
				"trainer_plan_entitlements": -1,
			})
			if got := rowsFingerprint(t, pool, "program_assignments", "trainer_id", base.trainerID); got != baseBefore {
				t.Error("the other trainer's assignments changed")
			}
		})
	}
}

// A client who also became a trainer keeps nothing of the client side either: the links,
// the assignment from another trainer, the completions and the comments go with the account.
func TestAccountDeletion_clientTrainerWithoutTrainerDataIsDeletedWithItsClientData(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	account := activatedTrainer(t, store, "g-dual-del")
	seedClientData(t, pool, base, account, "dual")
	neighbour := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-dual-neighbour", "")
	seedClientData(t, pool, base, neighbour, "dual-neighbour")
	neighbourBefore := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour)
	counts := tableCounts(t, pool)

	if _, err := store.DeleteClientAccount(ctx, account, ""); err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}

	if userExists(t, pool, account) {
		t.Error("the user row is still there")
	}
	for key, n := range userReferences(t, pool, account) {
		if n != 0 {
			t.Errorf("%s still holds %d rows of the deleted account", key, n)
		}
	}
	assertTableDeltas(t, counts, tableCounts(t, pool), map[string]int{
		"users":                              -1,
		"trainers":                           -1,
		"user_roles":                         -2,
		"auth_identities":                    -1,
		"auth_refresh_sessions":              -1,
		"user_link_codes":                    -1,
		"trainer_clients":                    -1,
		"program_assignments":                -1,
		"client_workout_completions":         -2,
		"client_workout_completion_comments": -1,
		"program_block_clients":              -1,
		"trainer_invites":                    -1,
	})
	if got := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour); got != neighbourBefore {
		t.Error("the neighbour's workout completions changed")
	}
}

// deletedTrainerData names the rows seedDeletedTrainerData left behind.
type deletedTrainerData struct {
	programID, exerciseID, clientID uuid.UUID
}

// seedDeletedTrainerData gives trainer what is left of work that was deleted in the app: a
// published program that uses a private exercise and that carries a block visibility rule,
// both soft-deleted. The rule belongs to a client who is not a client of the trainer.
func seedDeletedTrainerData(t *testing.T, pool *pgxpool.Pool, store *auth.Store, trainer uuid.UUID, tag string) deletedTrainerData {
	t.Helper()
	ctx := context.Background()
	trainerID := trainersRowID(t, pool, trainer)
	ex, err := exercise.NewStore(pool).Create(ctx, trainer, &trainerID, exercise.UpsertInput{
		Name:        "Row " + tag,
		NameRu:      "Тяга " + tag,
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	programs := program.NewStore(pool)
	detail, err := programs.CreateDraft(ctx, trainer)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	detail, err = createSingleBlockStore(ctx, programs, trainer, detail.ID, detail.Weeks[0].ID, detail.Weeks[0].Days[0].ID,
		program.DayExerciseInput{ExerciseID: ex.ID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	published, err := programs.PublishFromDraft(ctx, detail.ID, trainer, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-deleted-rule-"+tag, "")
	execSQL(t, pool, "block rule", `
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by) VALUES ($1, $2, $3, $4)`,
		published.ID, published.Weeks[0].Days[0].Blocks[0].BlockKey, client, trainer)
	// Edits made while the program was live leave the trainer in these columns.
	execSQL(t, pool, "week editor", `UPDATE mentorix.program_weeks SET modified_by = $1 WHERE program_id = $2`, trainer, published.ID)
	execSQL(t, pool, "day editor", `UPDATE mentorix.program_week_days SET modified_by = $1 WHERE program_id = $2`, trainer, published.ID)
	execSQL(t, pool, "block exercise editor", `
UPDATE mentorix.program_week_day_block_exercises SET modified_by = $1
WHERE program_week_day_block_id IN (
  SELECT b.id FROM mentorix.program_week_day_blocks b
  JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
  WHERE d.program_id = $2)`, trainer, published.ID)
	if err := program.NewService(pool).Delete(ctx, trainer, published.ID); err != nil {
		t.Fatalf("delete program: %v", err)
	}
	if n, err := exercise.NewStore(pool).DeleteMany(ctx, trainer, &trainerID, []uuid.UUID{ex.ID}); err != nil || n != 1 {
		t.Fatalf("DeleteMany() = %d, %v, want 1", n, err)
	}
	return deletedTrainerData{programID: published.ID, exerciseID: ex.ID, clientID: client}
}

// A trainer who deleted everything they made in the app has no business data left: the
// soft-deleted program with its trees, versions and rules, and the soft-deleted private
// exercise, go with the account. Another trainer's program and exercise stay.
func TestAccountDeletion_trainerWithOnlyDeletedProgramsAndExercisesIsDeleted(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	account := activatedTrainer(t, store, "g-deleted-data")
	data := seedDeletedTrainerData(t, pool, store, account, "own")
	baseCounts := func() []int {
		return []int{
			countRows(t, pool, `SELECT count(*) FROM mentorix.programs WHERE id = $1`, base.programID),
			countRows(t, pool, `SELECT count(*) FROM mentorix.program_weeks WHERE program_id = $1`, base.programID),
			countRows(t, pool, `SELECT count(*) FROM mentorix.program_versions WHERE program_id = $1`, base.programID),
			countRows(t, pool, `SELECT count(*) FROM mentorix.program_block_clients WHERE program_id = $1`, base.programID),
			countRows(t, pool, `SELECT count(*) FROM mentorix.exercises WHERE created_by = $1`, base.trainerUser),
		}
	}
	baseBefore := baseCounts()
	programRows := []struct{ name, query string }{
		{"programs", `SELECT count(*) FROM mentorix.programs WHERE id = $1`},
		{"program weeks", `SELECT count(*) FROM mentorix.program_weeks WHERE program_id = $1`},
		{"program days", `SELECT count(*) FROM mentorix.program_week_days WHERE program_id = $1`},
		{"program blocks", `SELECT count(*) FROM mentorix.program_week_day_blocks b JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id WHERE d.program_id = $1`},
		{"block exercises", `SELECT count(*) FROM mentorix.program_week_day_block_exercises e JOIN mentorix.program_week_day_blocks b ON b.id = e.program_week_day_block_id JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id WHERE d.program_id = $1`},
		{"program versions", `SELECT count(*) FROM mentorix.program_versions WHERE program_id = $1`},
		{"version weeks", `SELECT count(*) FROM mentorix.program_version_weeks w JOIN mentorix.program_versions v ON v.id = w.program_version_id WHERE v.program_id = $1`},
		{"version days", `SELECT count(*) FROM mentorix.program_version_week_days d JOIN mentorix.program_versions v ON v.id = d.program_version_id WHERE v.program_id = $1`},
		{"version blocks", `SELECT count(*) FROM mentorix.program_version_week_day_blocks b JOIN mentorix.program_version_week_days d ON d.id = b.program_version_week_day_id JOIN mentorix.program_versions v ON v.id = d.program_version_id WHERE v.program_id = $1`},
		{"version block exercises", `SELECT count(*) FROM mentorix.program_version_week_day_block_exercises e JOIN mentorix.program_version_week_day_blocks b ON b.id = e.program_version_week_day_block_id JOIN mentorix.program_version_week_days d ON d.id = b.program_version_week_day_id JOIN mentorix.program_versions v ON v.id = d.program_version_id WHERE v.program_id = $1`},
		{"block visibility rules", `SELECT count(*) FROM mentorix.program_block_clients WHERE program_id = $1`},
	}
	const exerciseRows = `SELECT count(*) FROM mentorix.exercises WHERE id = $1`
	for _, rows := range programRows {
		if n := countRows(t, pool, rows.query, data.programID); n == 0 {
			t.Fatalf("seeded %s: no rows, the test would prove nothing", rows.name)
		}
	}
	if n := countRows(t, pool, exerciseRows, data.exerciseID); n == 0 {
		t.Fatal("seeded exercise: no row, the test would prove nothing")
	}

	if has, err := store.TrainerHasBusinessData(ctx, account); err != nil || has {
		t.Errorf("TrainerHasBusinessData() = %v, %v, want false", has, err)
	}
	_, err := store.DeleteClientAccount(ctx, account, "")

	if err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}
	if userExists(t, pool, account) {
		t.Error("the user row is still there")
	}
	for key, n := range userReferences(t, pool, account) {
		if n != 0 {
			t.Errorf("%s still holds %d rows of the deleted trainer", key, n)
		}
	}
	for _, rows := range programRows {
		if n := countRows(t, pool, rows.query, data.programID); n != 0 {
			t.Errorf("%s of the deleted program = %d rows, want 0", rows.name, n)
		}
	}
	if n := countRows(t, pool, exerciseRows, data.exerciseID); n != 0 {
		t.Errorf("the deleted exercise = %d rows, want 0", n)
	}
	if !userExists(t, pool, data.clientID) {
		t.Error("the client of the visibility rule was removed")
	}
	if got := baseCounts(); !reflect.DeepEqual(got, baseBefore) {
		t.Errorf("the other trainer's rows changed: %v, want %v", got, baseBefore)
	}
}

// businessDataKind seeds one kind of trainer business data for account, a trainer-only
// account. base is another trainer with a published program.
type businessDataKind struct {
	name string
	seed func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, trainerID uuid.UUID)
}

func insertExercise(t *testing.T, pool *pgxpool.Pool, createdBy uuid.UUID, ownerTrainer *uuid.UUID, deleted bool) uuid.UUID {
	t.Helper()
	deletedAt := "NULL"
	if deleted {
		deletedAt = "now()"
	}
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
INSERT INTO mentorix.exercises (name, name_ru, created_by, modified_by, exercise_type, muscle_group, difficulty, owner_trainer_id, deleted_at)
VALUES ('Row', 'Тяга', $1, $1, 'strength', 'back', 'beginner', $2, `+deletedAt+`)
RETURNING id`, createdBy, ownerTrainer).Scan(&id)
	if err != nil {
		t.Fatalf("seed exercise: %v", err)
	}
	return id
}

// insertDeletedProgram adds a soft-deleted program that createdBy made and nobody else
// edited, and returns its id.
func insertDeletedProgram(t *testing.T, pool *pgxpool.Pool, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
INSERT INTO mentorix.programs (created_by, modified_by, deleted_at) VALUES ($1, $1, now()) RETURNING id`, createdBy).Scan(&id)
	if err != nil {
		t.Fatalf("seed deleted program: %v", err)
	}
	return id
}

// insertProgramVersion adds a version of programID that publishedBy published.
func insertProgramVersion(t *testing.T, pool *pgxpool.Pool, programID, publishedBy uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
INSERT INTO mentorix.program_versions (program_id, version_number, published_by) VALUES ($1, 1, $2) RETURNING id`, programID, publishedBy).Scan(&id)
	if err != nil {
		t.Fatalf("seed program version: %v", err)
	}
	return id
}

// baseVersionID is the version that seedDeletionBase published.
func baseVersionID(t *testing.T, pool *pgxpool.Pool, base deletionBase) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM mentorix.program_versions WHERE program_id = $1`, base.programID).Scan(&id); err != nil {
		t.Fatalf("version id: %v", err)
	}
	return id
}

// insertAssignmentRow adds an assignment of the trainer of base for a client, made by base's
// trainer user, that points at the given program and version.
func insertAssignmentRow(t *testing.T, pool *pgxpool.Pool, base deletionBase, client, programID, versionID uuid.UUID) {
	t.Helper()
	execSQL(t, pool, "assignment", `
INSERT INTO mentorix.program_assignments (program_id, program_version_id, trainer_id, client_user_id, created_by, modified_by)
VALUES ($1, $2, $3, $4, $5, $5)`, programID, versionID, base.trainerID, client, base.trainerUser)
}

func insertAssignment(t *testing.T, pool *pgxpool.Pool, base deletionBase, trainerID, client, createdBy uuid.UUID) {
	t.Helper()
	var versionID uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM mentorix.program_versions WHERE program_id = $1`, base.programID).Scan(&versionID); err != nil {
		t.Fatalf("version id: %v", err)
	}
	execSQL(t, pool, "assignment", `
INSERT INTO mentorix.program_assignments (program_id, program_version_id, trainer_id, client_user_id, created_by, modified_by)
VALUES ($1, $2, $3, $4, $5, $5)`, base.programID, versionID, trainerID, client, createdBy)
}

func insertCompletion(t *testing.T, pool *pgxpool.Pool, trainerID, client uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
INSERT INTO mentorix.client_workout_completions
  (client_user_id, trainer_id, completed_at, completion_cycle_id, day_key, week_number, day_number,
   program_name, program_name_ru, day_snapshot, result_text, source)
VALUES ($1, $2, now(), $3, $4, 1, 1, 'Strength', 'Сила', '{}'::jsonb, 'done', 'telegram')
RETURNING id`, client, trainerID, uuid.New(), uuid.New()).Scan(&id)
	if err != nil {
		t.Fatalf("seed completion: %v", err)
	}
	return id
}

func businessDataKinds() []businessDataKind {
	otherClient := func(t *testing.T, store *auth.Store, tag string) uuid.UUID {
		return newClientWithIdentity(t, store, auth.ProviderGoogle, "g-data-"+tag, "")
	}
	return []businessDataKind{
		{"a client of the trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase, _, trainerID uuid.UUID) {
			execSQL(t, pool, "trainer link", `INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`, trainerID, otherClient(t, store, "active"))
		}},
		{"a blocked client of the trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase, _, trainerID uuid.UUID) {
			execSQL(t, pool, "blocked link", `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status, blocked_at) VALUES ($1, $2, 'blocked', now())`, trainerID, otherClient(t, store, "blocked"))
		}},
		{"a draft program", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, account, _ uuid.UUID) {
			if _, err := program.NewStore(pool).CreateDraft(context.Background(), account); err != nil {
				t.Fatalf("CreateDraft: %v", err)
			}
		}},
		{"a soft-deleted program an assignment points at", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			// The assignment belongs to the other trainer, so only the program check refuses.
			insertAssignmentRow(t, pool, base, otherClient(t, store, "deleted-program"), insertDeletedProgram(t, pool, account), baseVersionID(t, pool, base))
		}},
		{"a soft-deleted program whose version an assignment points at", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			programID := insertDeletedProgram(t, pool, account)
			versionID := insertProgramVersion(t, pool, programID, base.trainerUser)
			insertAssignmentRow(t, pool, base, otherClient(t, store, "deleted-version"), base.programID, versionID)
		}},
		{"a soft-deleted program a workout completion points at", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			completion := insertCompletion(t, pool, base.trainerID, otherClient(t, store, "deleted-program-done"))
			execSQL(t, pool, "completion program", `UPDATE mentorix.client_workout_completions SET program_id = $1 WHERE id = $2`,
				insertDeletedProgram(t, pool, account), completion)
		}},
		{"a soft-deleted program whose version a workout completion points at", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			completion := insertCompletion(t, pool, base.trainerID, otherClient(t, store, "deleted-version-done"))
			versionID := insertProgramVersion(t, pool, insertDeletedProgram(t, pool, account), base.trainerUser)
			execSQL(t, pool, "completion version", `UPDATE mentorix.client_workout_completions SET program_version_id = $1 WHERE id = $2`, versionID, completion)
		}},
		{"a soft-deleted program of another trainer that the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "deleted program editor", `UPDATE mentorix.programs SET deleted_at = now(), modified_by = $1 WHERE id = $2`, account, base.programID)
		}},
		{"an exercise of the trainer", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, account, trainerID uuid.UUID) {
			insertExercise(t, pool, account, &trainerID, false)
		}},
		{"a soft-deleted exercise of the trainer in another trainer's live program", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, trainerID uuid.UUID) {
			exerciseID := insertExercise(t, pool, account, &trainerID, true)
			execSQL(t, pool, "program tree exercise", `
UPDATE mentorix.program_week_day_block_exercises SET exercise_id = $1
WHERE program_week_day_block_id IN (
  SELECT b.id FROM mentorix.program_week_day_blocks b
  JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
  WHERE d.program_id = $2)`, exerciseID, base.programID)
		}},
		{"a soft-deleted exercise of the trainer in another trainer's program version", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, trainerID uuid.UUID) {
			exerciseID := insertExercise(t, pool, account, &trainerID, true)
			execSQL(t, pool, "version tree exercise", `
UPDATE mentorix.program_version_week_day_block_exercises SET exercise_id = $1
WHERE program_version_week_day_block_id IN (
  SELECT b.id FROM mentorix.program_version_week_day_blocks b
  JOIN mentorix.program_version_week_days d ON d.id = b.program_version_week_day_id
  JOIN mentorix.program_versions v ON v.id = d.program_version_id
  WHERE v.program_id = $2)`, exerciseID, base.programID)
		}},
		{"a soft-deleted exercise the user created without an owner", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, account, _ uuid.UUID) {
			insertExercise(t, pool, account, nil, true)
		}},
		{"an exercise the user created without an owner", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, account, _ uuid.UUID) {
			insertExercise(t, pool, account, nil, false)
		}},
		{"an exercise owned by the trainer and created by another user", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, _, trainerID uuid.UUID) {
			insertExercise(t, pool, base.trainerUser, &trainerID, false)
		}},
		{"an assignment of the trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, _, trainerID uuid.UUID) {
			insertAssignment(t, pool, base, trainerID, otherClient(t, store, "assigned"), base.trainerUser)
		}},
		{"an assignment the user created for another trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			insertAssignment(t, pool, base, base.trainerID, otherClient(t, store, "created"), account)
		}},
		{"a workout completion of the trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase, _, trainerID uuid.UUID) {
			insertCompletion(t, pool, trainerID, otherClient(t, store, "done"))
		}},
		{"a comment of the trainer", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, _, trainerID uuid.UUID) {
			completion := insertCompletion(t, pool, base.trainerID, otherClient(t, store, "commented"))
			execSQL(t, pool, "comment", `
INSERT INTO mentorix.client_workout_completion_comments (client_workout_completion_id, trainer_id, comment_text) VALUES ($1, $2, 'nice')`, completion, trainerID)
		}},
		{"a block visibility rule the user created", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "block rule", `
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by) VALUES ($1, $2, $3, $4)`,
				base.programID, base.blockKey, otherClient(t, store, "rule"), account)
		}},
		{"an exercise the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "exercise editor", `
UPDATE mentorix.exercises SET modified_by = $1 WHERE id = (SELECT id FROM mentorix.exercises WHERE created_by = $2 LIMIT 1)`, account, base.trainerUser)
		}},
		{"a program the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "program editor", `UPDATE mentorix.programs SET modified_by = $1 WHERE id = $2`, account, base.programID)
		}},
		{"a program week the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "week editor", `UPDATE mentorix.program_weeks SET modified_by = $1 WHERE program_id = $2`, account, base.programID)
		}},
		{"a program day the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "day editor", `UPDATE mentorix.program_week_days SET modified_by = $1 WHERE program_id = $2`, account, base.programID)
		}},
		{"a block exercise the user last modified", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "block exercise editor", `
UPDATE mentorix.program_week_day_block_exercises SET modified_by = $1
WHERE program_week_day_block_id IN (
  SELECT b.id FROM mentorix.program_week_day_blocks b
  JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
  WHERE d.program_id = $2)`, account, base.programID)
		}},
		{"a program version the user published", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, base deletionBase, account, _ uuid.UUID) {
			execSQL(t, pool, "version publisher", `UPDATE mentorix.program_versions SET published_by = $1 WHERE program_id = $2`, account, base.programID)
		}},
		{"an assignment the user last modified", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase, account, _ uuid.UUID) {
			insertAssignment(t, pool, base, base.trainerID, otherClient(t, store, "modified"), base.trainerUser)
			execSQL(t, pool, "assignment editor", `UPDATE mentorix.program_assignments SET modified_by = $1 WHERE trainer_id = $2`, account, base.trainerID)
		}},
		{"an expired App Store entitlement", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, _, trainerID uuid.UUID) {
			execSQL(t, pool, "app store entitlement", `
INSERT INTO mentorix.trainer_plan_entitlements (trainer_id, plan_code, source, status) VALUES ($1, 'advance', 'app_store', 'expired')`, trainerID)
		}},
		{"an active Google Play entitlement", func(t *testing.T, pool *pgxpool.Pool, _ *auth.Store, _ deletionBase, _, trainerID uuid.UUID) {
			execSQL(t, pool, "google play entitlement", `
INSERT INTO mentorix.trainer_plan_entitlements (trainer_id, plan_code, source, status) VALUES ($1, 'elite', 'google_play', 'active')`, trainerID)
		}},
	}
}

// Each kind of business data alone makes the deletion refuse, and a refusal changes nothing.
func TestAccountDeletion_trainerBusinessDataRefusesAndChangesNothing(t *testing.T) {
	for _, kind := range businessDataKinds() {
		t.Run(kind.name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			ctx := context.Background()
			base := seedDeletionBase(t, pool, "del-trainer")
			account := newCoach(t, pool, "coach-data@test.com")
			seedTrainerOwnRows(t, pool, store, account, "coach-data")
			kind.seed(t, pool, store, base, account, trainersRowID(t, pool, account))
			roles, err := store.UserRoles(ctx, account)
			if err != nil {
				t.Fatalf("UserRoles: %v", err)
			}
			before := takeSnapshot(t, pool, account)

			has, err := store.TrainerHasBusinessData(ctx, account)
			if err != nil || !has {
				t.Errorf("TrainerHasBusinessData() = %v, %v, want true", has, err)
			}
			telegramID, err := store.DeleteClientAccount(ctx, account, "")

			if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
				t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
			}
			if telegramID != "" {
				t.Errorf("telegram id = %q, want empty", telegramID)
			}
			before.assertUnchanged(t, pool, account)
			if got, err := store.UserRoles(ctx, account); err != nil || !reflect.DeepEqual(got, roles) {
				t.Errorf("roles = %v (%v), want %v", got, err, roles)
			}
		})
	}
}

// The same data on an account that became a trainer in the app.
func TestAccountDeletion_activatedTrainerWithAClientIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	account := activatedTrainer(t, store, "g-activated-data")
	execSQL(t, pool, "trainer link", `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`,
		trainersRowID(t, pool, account), newClientWithIdentity(t, store, auth.ProviderGoogle, "g-activated-client", ""))
	before := takeSnapshot(t, pool, account)

	_, err := store.DeleteClientAccount(ctx, account, "")

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	before.assertUnchanged(t, pool, account)
}

// What belongs to the account alone is not business data: invites, the admin grant, programs
// and exercises it deleted that nothing else uses, being somebody's client, and an account that
// never had a trainers row.
func TestAccountDeletion_trainerHasBusinessDataIsFalseForWhatGoesWithTheAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	coach := newCoach(t, pool, "coach-clean@test.com")
	seedTrainerOwnRows(t, pool, store, coach, "coach-clean")
	seedDeletedTrainerData(t, pool, store, coach, "coach-clean")
	asClient := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-only-client", "")
	seedClientData(t, pool, base, asClient, "only-client")

	for name, id := range map[string]uuid.UUID{
		"trainer with invites, an admin grant and deleted work": coach,
		"client with client data only":                          asClient,
		"unknown user":                                          uuid.New(),
	} {
		if has, err := store.TrainerHasBusinessData(ctx, id); err != nil || has {
			t.Errorf("%s: TrainerHasBusinessData() = %v, %v, want false", name, has, err)
		}
	}
	// The trainer with data is the positive control for the whole query.
	if has, err := store.TrainerHasBusinessData(ctx, base.trainerUser); err != nil || !has {
		t.Errorf("trainer with a program: TrainerHasBusinessData() = %v, %v, want true", has, err)
	}
}

// A client of the trainer that arrives while the deletion waits is seen: the deletion holds
// the trainers row first, so the new link waits for it or is committed before the check.
func TestAccountDeletion_trainerDataCommittedWhileWaitingIsSeen(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	account := newCoach(t, pool, "coach-race@test.com")
	trainerID := trainersRowID(t, pool, account)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-race-client", "")
	before := takeSnapshot(t, pool, account)

	err := deleteBlockedBy(t, pool, deleteWithoutRevocation(store, account),
		`INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`, trainerID, client)

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	if !userExists(t, pool, account) {
		t.Fatal("the user row was removed")
	}
	if got := userReferences(t, pool, account); !reflect.DeepEqual(got, before.references) {
		t.Errorf("rows changed\nbefore: %v\nafter:  %v", before.references, got)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE trainer_id = $1`, trainerID); n != 1 {
		t.Errorf("trainer_clients rows of the trainer = %d, want the 1 that committed", n)
	}
}

// Quota writers lock the trainers row and then touch the user row. The deletion takes them in
// the same order, so a writer that is mid-flight finishes instead of ending in a deadlock.
func TestAccountDeletion_locksTheTrainerRowBeforeTheUserRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	account := newCoach(t, pool, "coach-lock@test.com")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.trainers WHERE user_id = $1 FOR UPDATE`, account); err != nil {
		t.Fatalf("lock trainers row: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := store.DeleteClientAccount(ctx, account, "")
		done <- err
	}()
	waitForLockWait(t, pool)
	if _, err := tx.Exec(ctx, `UPDATE mentorix.users SET display_name = 'Coach 2' WHERE id = $1`, account); err != nil {
		t.Fatalf("touch the user row while the deletion waits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DeleteClientAccount() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the deletion did not finish after the trainers row was released")
	}
	if userExists(t, pool, account) {
		t.Error("the user row is still there")
	}
}

// DELETE /auth/me through app.Mount for a trainer without data, then the same trainer's
// tokens find nothing.
func TestAccountDeletion_mountedRouteDeletesATrainerWithoutData(t *testing.T) {
	m := newMountedEnv(t)
	account := activatedTrainer(t, m.store, "g-mount-trainer-del")
	seedTrainerOwnRows(t, m.pool, m.store, account, "mount-coach")
	token := m.accessToken(t, account)

	rec := m.do(http.MethodDelete, "/auth/me", token, "")

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, body = %q, want 204 and no body", rec.Code, rec.Body.String())
	}
	if userExists(t, m.pool, account) {
		t.Error("the user row is still there")
	}
	if n := countRows(t, m.pool, `SELECT count(*) FROM mentorix.trainers WHERE user_id = $1`, account); n != 0 {
		t.Errorf("trainers rows = %d, want 0", n)
	}
	assertErrorBody(t, m.do(http.MethodGet, "/auth/me", token, ""), http.StatusNotFound, "user_not_found")
}

func TestAccountDeletion_mountedRouteRefusesATrainerWithAClient(t *testing.T) {
	m := newMountedEnv(t)
	account := newCoach(t, m.pool, "coach-mount-refuse@test.com")
	execSQL(t, m.pool, "trainer link", `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`,
		trainersRowID(t, m.pool, account), newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-refuse-client", ""))
	before := takeSnapshot(t, m.pool, account)

	rec := m.do(http.MethodDelete, "/auth/me", m.accessToken(t, account), "")

	assertErrorBody(t, rec, http.StatusConflict, "account_deletion_unavailable")
	before.assertUnchanged(t, m.pool, account)
}

// A plain client has no trainers row and no trainer role, and can still have edited a row that
// the database cannot detach from them. The data check runs for every account, so this is a
// refusal and not an unmapped database error.
func TestAccountDeletion_clientWhoEditedAnAssignmentIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-editor-client", "")
	insertAssignment(t, pool, base, base.trainerID, newClientWithIdentity(t, store, auth.ProviderGoogle, "g-editor-assignee", ""), base.trainerUser)
	execSQL(t, pool, "assignment editor", `UPDATE mentorix.program_assignments SET modified_by = $1 WHERE trainer_id = $2`, client, base.trainerID)
	before := takeSnapshot(t, pool, client)

	_, err := store.DeleteClientAccount(ctx, client, "")

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	before.assertUnchanged(t, pool, client)
}

// acceptInviteShape is what trainerclient does to accept an invite once it holds the invite
// row (GetTrainerInviteByTokenForUpdate, taken by the caller): for a new link, ensureClientLink
// locks the trainers row through subscription.LockTrainer (LockTrainerRow, FOR UPDATE), checks
// the quota, inserts the trainer_clients row, and the invite is marked consumed.
func acceptInviteShape(t *testing.T, tx pgx.Tx, token string, trainerID, client uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, step := range []struct {
		what, query string
		args        []any
	}{
		{"lock the trainers row", `SELECT id FROM mentorix.trainers WHERE id = $1 FOR UPDATE`, []any{trainerID}},
		{"link the client", `INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`, []any{trainerID, client}},
		{"consume the invite", `UPDATE mentorix.trainer_invites SET consumed_at = now(), consumed_by = $2 WHERE token = $1`, []any{token, client}},
	} {
		if _, err := tx.Exec(ctx, step.query, step.args...); err != nil {
			t.Fatalf("accept: %s: %v", step.what, err)
		}
	}
}

// Accepting an invite locks the invite row and then the trainers row. The deletion takes the
// invite rows first too, so an accept that is mid-flight finishes instead of ending in a
// deadlock, and the deletion then sees the new client and refuses.
func TestAccountDeletion_acceptInFlightFinishesAndTheDeletionIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	account := newCoach(t, pool, "coach-accept@test.com")
	trainerID := trainersRowID(t, pool, account)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-accept-client", "")
	execSQL(t, pool, "invite", `INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at) VALUES ($1, 'inv-accept', now() + interval '1 day')`, trainerID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.trainer_invites WHERE token = 'inv-accept' FOR UPDATE`); err != nil {
		t.Fatalf("lock the invite: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := store.DeleteClientAccount(ctx, account, "")
		done <- err
	}()
	waitForLockWait(t, pool)
	acceptInviteShape(t, tx, "inv-accept", trainerID, client)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit the accept: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
			t.Fatalf("DeleteClientAccount() error = %v, want ErrAccountDeletionUnavailable", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the deletion did not finish after the accept committed")
	}
	if !userExists(t, pool, account) {
		t.Error("the user row was removed")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE trainer_id = $1 AND client_user_id = $2`, trainerID, client); n != 1 {
		t.Errorf("the accepted link = %d rows, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_invites WHERE token = 'inv-accept' AND consumed_by = $1`, client); n != 1 {
		t.Errorf("the consumed invite = %d rows, want 1", n)
	}
}

// If the deletion commits first, an accept of the removed invite fails cleanly.
func TestAccountDeletion_acceptAfterTheDeletionFindsNoInvite(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	account := newCoach(t, pool, "coach-accept-late@test.com")
	trainerID := trainersRowID(t, pool, account)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-accept-late-client", "")
	execSQL(t, pool, "invite", `INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at) VALUES ($1, 'inv-late', now() + interval '1 day')`, trainerID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.trainer_invites WHERE token = 'inv-late' FOR UPDATE`); err != nil {
		t.Fatalf("lock the invite: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := store.DeleteClientAccount(ctx, account, "")
		done <- err
	}()
	waitForLockWait(t, pool)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("release the invite: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DeleteClientAccount() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the deletion did not finish after the invite was released")
	}

	_, err = trainerclient.NewStore(pool).AcceptInviteAsUser(ctx, "inv-late", client)

	if !errors.Is(err, trainerclient.ErrInviteNotFound) {
		t.Fatalf("AcceptInviteAsUser() error = %v, want ErrInviteNotFound", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE client_user_id = $1`, client); n != 0 {
		t.Errorf("trainer_clients rows of the client = %d, want 0", n)
	}
}
