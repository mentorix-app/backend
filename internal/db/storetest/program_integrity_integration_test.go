//go:build integration

package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/program"
)

// versionedProgram is a published program whose V1 has a client and whose
// newer V2 has nobody on it yet.
type versionedProgram struct {
	trainerUserID uuid.UUID
	trainerID     uuid.UUID
	clientUserID  uuid.UUID
	programID     uuid.UUID
	v1ID, v2ID    uuid.UUID
}

func seedProgramWithAssignedV1AndFreshV2(t *testing.T, pool *pgxpool.Pool, prefix string) versionedProgram {
	t.Helper()
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, prefix)
	draft, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week, day := draft.Weeks[0], draft.Weeks[0].Days[0]
	detail, err := createSingleBlockStore(ctx, store, userID, draft.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	v1, err := store.PublishFromDraft(ctx, draft.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}

	clientUserID, trainerID := seedAssignedClient(t, pool, userID, prefix+"-client")
	programID := draft.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	name := "Second version"
	if _, err := store.Update(ctx, programID, userID, program.UpdateInput{Name: &name}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	changed, err := store.GetDetail(ctx, programID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	v2, err := store.FreezePublishedVersion(ctx, programID, userID, changed)
	if err != nil {
		t.Fatalf("FreezePublishedVersion: %v", err)
	}
	if *v2.LatestProgramVersionID == *v1.LatestProgramVersionID {
		t.Fatal("test premise broken: publishing the update did not create a new version")
	}
	return versionedProgram{
		trainerUserID: userID,
		trainerID:     trainerID,
		clientUserID:  clientUserID,
		programID:     programID,
		v1ID:          *v1.LatestProgramVersionID,
		v2ID:          *v2.LatestProgramVersionID,
	}
}

func versionExists(t *testing.T, pool *pgxpool.Pool, versionID uuid.UUID) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM mentorix.program_versions WHERE id = $1)`, versionID).Scan(&ok); err != nil {
		t.Fatalf("check version: %v", err)
	}
	return ok
}

// A version nobody is assigned to yet is where the next assignment and the
// next sync land, so cleanup must never remove it while an older version is
// still in use.
func TestCleanupProgramVersions_keepsLatestVersion(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "cleanup-latest")

	result, err := store.CleanupProgramVersions(ctx, seed.programID)
	if err != nil {
		t.Fatalf("CleanupProgramVersions: %v", err)
	}

	if !versionExists(t, pool, seed.v2ID) {
		t.Fatalf("latest version %s was deleted by cleanup (deleted=%v skipped=%+v)",
			seed.v2ID, result.DeletedVersionIDs, result.Skipped)
	}
	if !versionExists(t, pool, seed.v1ID) {
		t.Fatalf("assigned version %s was deleted by cleanup", seed.v1ID)
	}
	if len(result.DeletedVersionIDs) != 0 {
		t.Fatalf("deleted = %v, want none", result.DeletedVersionIDs)
	}
	reasons := map[uuid.UUID]string{}
	for _, s := range result.Skipped {
		reasons[s.VersionID] = s.Reason
	}
	if reasons[seed.v2ID] != "latest_version" || reasons[seed.v1ID] != "has_assignments" {
		t.Fatalf("skipped reasons = %v, want v2=latest_version v1=has_assignments", reasons)
	}
}

// Once nobody is on the old version any more, cleanup still removes it and
// keeps the latest. Sync runs cleanup itself, so the client is moved to V2 with
// a plain UPDATE: that leaves cleanup as the only thing that can delete V1.
func TestCleanupProgramVersions_removesOldUnassignedVersion(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "cleanup-old")

	if _, err := pool.Exec(ctx,
		`UPDATE mentorix.program_assignments SET program_version_id = $1 WHERE program_id = $2`,
		seed.v2ID, seed.programID); err != nil {
		t.Fatalf("move client to V2: %v", err)
	}

	result, err := store.CleanupProgramVersions(ctx, seed.programID)
	if err != nil {
		t.Fatalf("CleanupProgramVersions: %v", err)
	}

	if len(result.DeletedVersionIDs) != 1 || result.DeletedVersionIDs[0] != seed.v1ID {
		t.Fatalf("deleted = %v, want only V1 %s", result.DeletedVersionIDs, seed.v1ID)
	}
	if versionExists(t, pool, seed.v1ID) {
		t.Fatal("old version V1 still exists after cleanup")
	}
	if !versionExists(t, pool, seed.v2ID) {
		t.Fatal("latest version V2 was deleted by cleanup")
	}
}

func TestDeleteProgramVersion_refusesLatestVersion(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "delete-latest")

	if err := store.DeleteProgramVersion(ctx, seed.programID, seed.v2ID); !errors.Is(err, program.ErrLatestProgramVersion) {
		t.Fatalf("DeleteProgramVersion(latest) error = %v, want ErrLatestProgramVersion", err)
	}
	if !versionExists(t, pool, seed.v2ID) {
		t.Fatal("latest version was deleted")
	}

	list, err := store.ListProgramVersions(ctx, seed.programID)
	if err != nil {
		t.Fatalf("ListProgramVersions: %v", err)
	}
	for _, item := range list.Items {
		if item.ID == seed.v2ID && item.CanDelete {
			t.Fatal("latest version reports can_delete = true")
		}
	}
}

func assignmentCount(t *testing.T, pool *pgxpool.Pool, programID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.program_assignments WHERE program_id = $1`, programID).Scan(&n); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	return n
}

func programDeleted(t *testing.T, pool *pgxpool.Pool, programID uuid.UUID) bool {
	t.Helper()
	var deleted bool
	if err := pool.QueryRow(context.Background(),
		`SELECT deleted_at IS NOT NULL FROM mentorix.programs WHERE id = $1`, programID).Scan(&deleted); err != nil {
		t.Fatalf("read program: %v", err)
	}
	return deleted
}

func TestProgramService_Delete_removesAssignmentsAndProgramTogether(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "delete-program")

	if got := assignmentCount(t, pool, seed.programID); got != 1 {
		t.Fatalf("test premise broken: assignments = %d, want 1", got)
	}
	if err := program.NewService(pool).Delete(ctx, seed.trainerUserID, seed.programID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := assignmentCount(t, pool, seed.programID); got != 0 {
		t.Fatalf("assignments after delete = %d, want 0", got)
	}
	if !programDeleted(t, pool, seed.programID) {
		t.Fatal("program is not soft-deleted")
	}
	// Cleanup runs after the commit: the unused older version goes, the latest stays.
	if versionExists(t, pool, seed.v1ID) || !versionExists(t, pool, seed.v2ID) {
		t.Fatalf("versions after delete: v1 exists=%v v2 exists=%v, want only v2",
			versionExists(t, pool, seed.v1ID), versionExists(t, pool, seed.v2ID))
	}
}

// Deleting a program that is already deleted is a no-op that reports success,
// the same answer a sequential repeat of the request gets, and a program that
// never existed is still pgx.ErrNoRows. Nothing here forces a failure inside
// the transaction, so the rollback of a failed delete is not covered by a test.
func TestProgramStore_DeleteWithAssignments_alreadyDeletedIsNoOp(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "delete-repeat")

	if err := store.SoftDelete(ctx, seed.programID, seed.trainerUserID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := store.DeleteWithAssignments(ctx, seed.programID, seed.trainerUserID); err != nil {
		t.Fatalf("DeleteWithAssignments on a deleted program = %v, want nil", err)
	}
	if got := assignmentCount(t, pool, seed.programID); got != 1 {
		t.Fatalf("assignments after no-op delete = %d, want 1 (nothing to do)", got)
	}

	err := store.DeleteWithAssignments(ctx, uuid.New(), seed.trainerUserID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DeleteWithAssignments on a missing program = %v, want pgx.ErrNoRows", err)
	}
}

// Two racing DELETE requests both succeed: the loser of the lock finds the
// program already deleted instead of failing with "not found".
func TestProgramStore_DeleteWithAssignments_concurrentBothSucceed(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	for round := 0; round < 10; round++ {
		seed := seedProgramWithAssignedV1AndFreshV2(t, pool, fmt.Sprintf("delete-race-%d", round))
		errA, errB := raceTwice(
			func() error { return store.DeleteWithAssignments(ctx, seed.programID, seed.trainerUserID) },
			func() error { return store.DeleteWithAssignments(ctx, seed.programID, seed.trainerUserID) },
		)
		if errA != nil || errB != nil {
			t.Fatalf("round %d: errors = %v, %v, want both nil", round, errA, errB)
		}
		if !programDeleted(t, pool, seed.programID) || assignmentCount(t, pool, seed.programID) != 0 {
			t.Fatalf("round %d: program not fully deleted", round)
		}
	}
}

// raceTwice runs a and b at the same moment and returns their errors.
func raceTwice(a, b func() error) (error, error) {
	var wg sync.WaitGroup
	var errA, errB error
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; errA = a() }()
	go func() { defer wg.Done(); <-start; errB = b() }()
	close(start)
	wg.Wait()
	return errA, errB
}

// Without the program lock both deletes see two weeks, both pass the
// "not the last week" check and the program ends up with no weeks.
// DeleteDay uses the same lock and the same count-then-delete shape.
func TestProgramStore_DeleteWeek_concurrentKeepsOneWeek(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	userID, _ := seedTrainerAndExercise(t, pool, "race-delete-week")
	draft, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	const rounds = 25
	for round := 0; round < rounds; round++ {
		detail, err := store.AddWeek(ctx, draft.ID)
		if err != nil {
			t.Fatalf("round %d: AddWeek: %v", round, err)
		}
		if len(detail.Weeks) != 2 {
			t.Fatalf("round %d: weeks = %d, want 2", round, len(detail.Weeks))
		}
		w1, w2 := detail.Weeks[0].ID, detail.Weeks[1].ID

		errA, errB := raceTwice(
			func() error { _, err := store.DeleteWeek(ctx, draft.ID, w1); return err },
			func() error { _, err := store.DeleteWeek(ctx, draft.ID, w2); return err },
		)

		lastWeek := 0
		for _, err := range []error{errA, errB} {
			switch {
			case errors.Is(err, program.ErrLastWeek):
				lastWeek++
			case err != nil:
				t.Fatalf("round %d: unexpected error: %v", round, err)
			}
		}
		if lastWeek != 1 {
			t.Fatalf("round %d: ErrLastWeek count = %d, want exactly 1 (errs: %v, %v)", round, lastWeek, errA, errB)
		}
		final, err := store.GetDetail(ctx, draft.ID)
		if err != nil {
			t.Fatalf("round %d: GetDetail: %v", round, err)
		}
		if len(final.Weeks) != 1 {
			t.Fatalf("round %d: weeks after race = %d, want 1", round, len(final.Weeks))
		}
	}
}

// Without the program lock both calls see six days and both insert one,
// exceeding the weekly maximum (or colliding on the day number).
func TestProgramStore_AddDay_concurrentRespectsWeeklyMaximum(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	userID, _ := seedTrainerAndExercise(t, pool, "race-add-day")
	draft, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID

	const rounds = 25
	for round := 0; round < rounds; round++ {
		detail, err := store.GetDetail(ctx, draft.ID)
		if err != nil {
			t.Fatalf("round %d: GetDetail: %v", round, err)
		}
		days := detail.Weeks[0].Days
		if _, err := store.DeleteDay(ctx, draft.ID, weekID, days[len(days)-1].ID); err != nil {
			t.Fatalf("round %d: DeleteDay: %v", round, err)
		}

		errA, errB := raceTwice(
			func() error { _, err := store.AddDay(ctx, draft.ID, weekID); return err },
			func() error { _, err := store.AddDay(ctx, draft.ID, weekID); return err },
		)

		atMax := 0
		for _, err := range []error{errA, errB} {
			switch {
			case errors.Is(err, program.ErrMaxDaysPerWeek):
				atMax++
			case err != nil:
				t.Fatalf("round %d: unexpected error: %v", round, err)
			}
		}
		if atMax != 1 {
			t.Fatalf("round %d: ErrMaxDaysPerWeek count = %d, want exactly 1 (errs: %v, %v)", round, atMax, errA, errB)
		}
		final, err := store.GetDetail(ctx, draft.ID)
		if err != nil {
			t.Fatalf("round %d: GetDetail: %v", round, err)
		}
		if got := len(final.Weeks[0].Days); got != program.DefaultWeekDays {
			t.Fatalf("round %d: days after race = %d, want %d", round, got, program.DefaultWeekDays)
		}
	}
}

func versionCount(t *testing.T, pool *pgxpool.Pool, programID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.program_versions WHERE program_id = $1`, programID).Scan(&n); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	return n
}

// Two concurrent publishes of the same state must behave like two sequential
// ones: the first wins, the second gets the conflict the service answers when
// there is nothing left to publish, and no duplicate version appears.
func TestProgramStore_publishTwiceConcurrently(t *testing.T) {
	t.Run("first publish from draft", func(t *testing.T) {
		pool := NewPool(t)
		ctx := context.Background()
		store := program.NewStore(pool)
		userID, exerciseID := seedTrainerAndExercise(t, pool, "race-publish")

		// Free plan allows 3 programs, one per round.
		for round := 0; round < 3; round++ {
			draft, err := store.CreateDraft(ctx, userID)
			if err != nil {
				t.Fatalf("round %d: CreateDraft: %v", round, err)
			}
			detail, err := createSingleBlockStore(ctx, store, userID, draft.ID, draft.Weeks[0].ID, draft.Weeks[0].Days[0].ID,
				program.DayExerciseInput{ExerciseID: exerciseID})
			if err != nil {
				t.Fatalf("round %d: createSingleBlockStore: %v", round, err)
			}

			errA, errB := raceTwice(
				func() error { _, err := store.PublishFromDraft(ctx, draft.ID, userID, detail); return err },
				func() error { _, err := store.PublishFromDraft(ctx, draft.ID, userID, detail); return err },
			)
			conflicts := 0
			for _, err := range []error{errA, errB} {
				switch {
				case errors.Is(err, program.ErrInvalidStatusTransition):
					conflicts++
				case err != nil:
					t.Fatalf("round %d: unexpected error: %v", round, err)
				}
			}
			if conflicts != 1 {
				t.Fatalf("round %d: conflicts = %d, want exactly 1 (errs: %v, %v)", round, conflicts, errA, errB)
			}
			if got := versionCount(t, pool, draft.ID); got != 1 {
				t.Fatalf("round %d: versions = %d, want 1", round, got)
			}
		}
	})

	t.Run("publish update", func(t *testing.T) {
		pool := NewPool(t)
		ctx := context.Background()
		store := program.NewStore(pool)
		seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "race-publish-update")

		for round := 0; round < 10; round++ {
			name := fmt.Sprintf("Edit %d", round)
			if _, err := store.Update(ctx, seed.programID, seed.trainerUserID, program.UpdateInput{Name: &name}); err != nil {
				t.Fatalf("round %d: Update: %v", round, err)
			}
			changed, err := store.GetDetail(ctx, seed.programID)
			if err != nil {
				t.Fatalf("round %d: GetDetail: %v", round, err)
			}
			before := versionCount(t, pool, seed.programID)

			errA, errB := raceTwice(
				func() error {
					_, err := store.FreezePublishedVersion(ctx, seed.programID, seed.trainerUserID, changed)
					return err
				},
				func() error {
					_, err := store.FreezePublishedVersion(ctx, seed.programID, seed.trainerUserID, changed)
					return err
				},
			)
			conflicts := 0
			for _, err := range []error{errA, errB} {
				switch {
				case errors.Is(err, program.ErrNoUnpublishedChanges):
					conflicts++
				case err != nil:
					t.Fatalf("round %d: unexpected error: %v", round, err)
				}
			}
			if conflicts != 1 {
				t.Fatalf("round %d: conflicts = %d, want exactly 1 (errs: %v, %v)", round, conflicts, errA, errB)
			}
			if got := versionCount(t, pool, seed.programID); got != before+1 {
				t.Fatalf("round %d: versions = %d, want %d", round, got, before+1)
			}
		}
	})
}

// Every assign holds a transaction connection. If it also reaches back into the
// pool for a second one, a burst of assigns larger than the pool deadlocks on
// itself. The timeout turns that regression into a failure instead of a hang.
func TestProgramStore_SetClientProgramAssignment_burstLargerThanPoolFinishes(t *testing.T) {
	pool := NewPool(t)
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "assign-burst")

	n := int(pool.Config().MaxConns) * 3
	clients := make([]uuid.UUID, n)
	for i := range clients {
		clients[i], _ = seedAssignedClient(t, pool, seed.trainerUserID, fmt.Sprintf("assign-burst-c%d", i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	programID := seed.programID
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = store.SetClientProgramAssignment(ctx, seed.trainerUserID, seed.trainerID, clients[i], &programID)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("assign %d of %d (pool %d): %v", i, n, pool.Config().MaxConns, err)
		}
	}
}

// Two first assigns of the same trainer/client pair both see "no assignment" and
// both insert; the loser must get what a sequential repeat gets, not a
// unique-violation 500.
func TestProgramStore_SetClientProgramAssignment_concurrentFirstAssignSamePair(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)
	seed := seedProgramWithAssignedV1AndFreshV2(t, pool, "assign-pair")
	clientUserID, _ := seedAssignedClient(t, pool, seed.trainerUserID, "assign-pair-c")
	programID := seed.programID

	const rounds = 15
	for round := 0; round < rounds; round++ {
		assign := func() error {
			_, err := store.SetClientProgramAssignment(ctx, seed.trainerUserID, seed.trainerID, clientUserID, &programID)
			return err
		}
		errA, errB := raceTwice(assign, assign)

		already := 0
		for _, err := range []error{errA, errB} {
			switch {
			case errors.Is(err, program.ErrAlreadyAssigned):
				already++
			case err != nil:
				t.Fatalf("round %d: unexpected error: %v", round, err)
			}
		}
		if already != 1 {
			t.Fatalf("round %d: ErrAlreadyAssigned count = %d, want exactly 1 (errs: %v, %v)", round, already, errA, errB)
		}

		var rows int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM mentorix.program_assignments
			WHERE trainer_id = $1 AND client_user_id = $2`, seed.trainerID, clientUserID).Scan(&rows); err != nil {
			t.Fatalf("round %d: count assignment rows: %v", round, err)
		}
		if rows != 1 {
			t.Fatalf("round %d: assignment rows = %d, want 1", round, rows)
		}
		if _, err := store.SetClientProgramAssignment(ctx, seed.trainerUserID, seed.trainerID, clientUserID, nil); err != nil {
			t.Fatalf("round %d: clear: %v", round, err)
		}
	}
}

// sortOrders reads the stored sort_order values of the rows selected by query
// (a single uuid argument), ordered, straight from the database.
func sortOrders(t *testing.T, pool *pgxpool.Pool, query string, parent uuid.UUID) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(), query, parent)
	if err != nil {
		t.Fatalf("read sort_order: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan sort_order: %v", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read sort_order: %v", err)
	}
	return out
}

func requireGapless(t *testing.T, round int, what string, got []int) {
	t.Helper()
	for i, n := range got {
		if n != i+1 {
			t.Fatalf("round %d: %s sort_order = %v, want 1..%d without gaps", round, what, got, len(got))
		}
	}
}

// A reorder and a delete on the same program used to lock sibling rows in
// opposite orders and could abort one another with a deadlock (SQLSTATE
// 40P01), or leave a gap in sort_order when the reorder worked from a list
// the delete had already invalidated. Both now serialize on the program lock.
func TestProgramStore_ReorderAndDelete_concurrentStaysConsistent(t *testing.T) {
	const rounds = 60
	reversed := func(ids []uuid.UUID) []uuid.UUID {
		out := make([]uuid.UUID, len(ids))
		for i, id := range ids {
			out[len(ids)-1-i] = id
		}
		return out
	}
	expected := func(t *testing.T, round int, name string, err error) {
		t.Helper()
		if err != nil && !errors.Is(err, program.ErrInvalidReorder) {
			t.Fatalf("round %d: %s: unexpected error: %v", round, name, err)
		}
	}

	t.Run("weeks", func(t *testing.T) {
		pool := NewPool(t)
		ctx := context.Background()
		store := program.NewStore(pool)
		userID, _ := seedTrainerAndExercise(t, pool, "race-reorder-week")
		draft, err := store.CreateDraft(ctx, userID)
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		for i := 0; i < 3; i++ {
			if _, err := store.AddWeek(ctx, draft.ID); err != nil {
				t.Fatalf("AddWeek: %v", err)
			}
		}
		for round := 0; round < rounds; round++ {
			detail, err := store.GetDetail(ctx, draft.ID)
			if err != nil {
				t.Fatalf("round %d: GetDetail: %v", round, err)
			}
			ids := make([]uuid.UUID, 0, len(detail.Weeks))
			for _, w := range detail.Weeks {
				ids = append(ids, w.ID)
			}
			victim := ids[1+round%2]
			errA, errB := raceTwice(
				func() error { _, err := store.ReorderWeeks(ctx, draft.ID, reversed(ids)); return err },
				func() error { _, err := store.DeleteWeek(ctx, draft.ID, victim); return err },
			)
			expected(t, round, "ReorderWeeks", errA)
			if errB != nil {
				t.Fatalf("round %d: DeleteWeek: %v", round, errB)
			}
			requireGapless(t, round, "weeks", sortOrders(t, pool,
				`SELECT sort_order FROM mentorix.program_weeks WHERE program_id = $1 ORDER BY sort_order`, draft.ID))
			if _, err := store.AddWeek(ctx, draft.ID); err != nil {
				t.Fatalf("round %d: AddWeek: %v", round, err)
			}
		}
	})

	t.Run("days", func(t *testing.T) {
		pool := NewPool(t)
		ctx := context.Background()
		store := program.NewStore(pool)
		userID, _ := seedTrainerAndExercise(t, pool, "race-reorder-day")
		draft, err := store.CreateDraft(ctx, userID)
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		weekID := draft.Weeks[0].ID
		for round := 0; round < rounds; round++ {
			detail, err := store.GetDetail(ctx, draft.ID)
			if err != nil {
				t.Fatalf("round %d: GetDetail: %v", round, err)
			}
			ids := make([]uuid.UUID, 0, len(detail.Weeks[0].Days))
			for _, d := range detail.Weeks[0].Days {
				ids = append(ids, d.ID)
			}
			victim := ids[1+round%3]
			errA, errB := raceTwice(
				func() error { _, err := store.ReorderDays(ctx, draft.ID, weekID, reversed(ids)); return err },
				func() error { _, err := store.DeleteDay(ctx, draft.ID, weekID, victim); return err },
			)
			expected(t, round, "ReorderDays", errA)
			if errB != nil {
				t.Fatalf("round %d: DeleteDay: %v", round, errB)
			}
			requireGapless(t, round, "days", sortOrders(t, pool,
				`SELECT sort_order FROM mentorix.program_week_days WHERE week_id = $1 ORDER BY sort_order`, weekID))
			if _, err := store.AddDay(ctx, draft.ID, weekID); err != nil {
				t.Fatalf("round %d: AddDay: %v", round, err)
			}
		}
	})
}
