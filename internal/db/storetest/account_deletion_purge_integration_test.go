//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// insertLiveProgram adds a program that createdBy made and that is not deleted.
func insertLiveProgram(t *testing.T, pool *pgxpool.Pool, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
INSERT INTO mentorix.programs (created_by, modified_by) VALUES ($1, $1) RETURNING id`, createdBy).Scan(&id)
	if err != nil {
		t.Fatalf("seed live program: %v", err)
	}
	return id
}

func rowExists(t *testing.T, pool *pgxpool.Pool, table string, id uuid.UUID) bool {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM mentorix.`+table+` WHERE id = $1`, id) == 1
}

// PurgeDeletedProgramsByCreator removes only the soft-deleted programs the caller created.
func TestPurgeDeletedProgramsByCreator_removesOnlyTheCallersDeletedPrograms(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	q := sqlc.New(pool)
	account := activatedTrainer(t, store, "g-purge-programs")
	other := newCoach(t, pool, "coach-purge-programs@test.com")
	live := insertLiveProgram(t, pool, account)
	deleted := insertDeletedProgram(t, pool, account)
	othersDeleted := insertDeletedProgram(t, pool, other)

	n, err := q.PurgeDeletedProgramsByCreator(context.Background(), pgconv.ToPGUUID(account))

	if err != nil {
		t.Fatalf("PurgeDeletedProgramsByCreator() error = %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeDeletedProgramsByCreator() removed %d rows, want 1", n)
	}
	if rowExists(t, pool, "programs", deleted) {
		t.Error("the caller's soft-deleted program is still there")
	}
	if !rowExists(t, pool, "programs", live) {
		t.Error("the caller's live program was removed")
	}
	if !rowExists(t, pool, "programs", othersDeleted) {
		t.Error("the soft-deleted program of another creator was removed")
	}
}

// PurgeDeletedExercisesByOwner removes only the soft-deleted exercises the caller's trainers
// row owns, and nothing for a user who has no trainers row.
func TestPurgeDeletedExercisesByOwner_removesOnlyTheOwnersDeletedExercises(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	q := sqlc.New(pool)
	ctx := context.Background()
	account := activatedTrainer(t, store, "g-purge-exercises")
	accountTrainer := trainersRowID(t, pool, account)
	other := newCoach(t, pool, "coach-purge-exercises@test.com")
	otherTrainer := trainersRowID(t, pool, other)
	noTrainer := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-purge-no-trainer", "")
	live := insertExercise(t, pool, account, &accountTrainer, false)
	deleted := insertExercise(t, pool, account, &accountTrainer, true)
	global := insertExercise(t, pool, account, nil, true)
	othersDeleted := insertExercise(t, pool, other, &otherTrainer, true)
	all := map[string]uuid.UUID{
		"live own":                live,
		"soft-deleted own":        deleted,
		"soft-deleted global":     global,
		"soft-deleted of another": othersDeleted,
	}

	n, err := q.PurgeDeletedExercisesByOwner(ctx, pgconv.ToPGUUID(noTrainer))
	if err != nil || n != 0 {
		t.Fatalf("PurgeDeletedExercisesByOwner(user without a trainers row) = %d, %v, want 0", n, err)
	}
	for name, id := range all {
		if !rowExists(t, pool, "exercises", id) {
			t.Errorf("a user without a trainers row removed the %s exercise", name)
		}
	}

	n, err = q.PurgeDeletedExercisesByOwner(ctx, pgconv.ToPGUUID(account))

	if err != nil {
		t.Fatalf("PurgeDeletedExercisesByOwner() error = %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeDeletedExercisesByOwner() removed %d rows, want 1", n)
	}
	if rowExists(t, pool, "exercises", deleted) {
		t.Error("the owner's soft-deleted exercise is still there")
	}
	for _, name := range []string{"live own", "soft-deleted global", "soft-deleted of another"} {
		if !rowExists(t, pool, "exercises", all[name]) {
			t.Errorf("the %s exercise was removed", name)
		}
	}
}
