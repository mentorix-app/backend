//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

// stepMigration moves the schema by n versions (negative: down) through a
// separate migrate instance, so data can be inserted between the steps.
func stepMigration(t *testing.T, connStr string, n int) {
	t.Helper()
	m, err := migrate.New(fmt.Sprintf("file://%s", filepath.Join(repoRoot(t), "db", "migrations")), connStr)
	if err != nil {
		t.Fatalf("migrate new: %v", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			t.Logf("migrate close: source %v, db %v", srcErr, dbErr)
		}
	}()
	if err := m.Steps(n); err != nil {
		t.Fatalf("migrate steps %d: %v", n, err)
	}
}

// TestMigration36_convertsBlocksSetsAndSnapshots runs migration 36 over data written
// the way version 35 left it: removed block types, sets in timed blocks and completion
// snapshots that mention the removed types.
func TestMigration36_convertsBlocksSetsAndSnapshots(t *testing.T) {
	pool, connStr := NewPoolWithURL(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	trainer, exerciseID := seedTrainerAndExercise(t, pool, "migration-36")
	draft, err := store.CreateDraft(ctx, trainer)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week, day := draft.Weeks[0], draft.Weeks[0].Days[0]
	sets, reps := "3", "10"
	for i := 0; i < 8; i++ {
		d, err := createSingleBlockStore(ctx, store, trainer, draft.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID, Sets: &sets, Reps: &reps})
		if err != nil {
			t.Fatalf("create block %d: %v", i, err)
		}
		draft = d
	}
	if _, err := store.PublishFromDraft(ctx, draft.ID, trainer, draft); err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}

	// Back to version 35, then write the data version 35 allowed.
	stepMigration(t, connStr, -1)
	types := []string{"ladder", "death_by", "complex", "emom", "amrap", "chipper", "strength", "single"}
	for i, bt := range types {
		for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
			// Each tree holds one day with the eight blocks; sort_order is 1..8 there.
			if _, err := pool.Exec(ctx, fmt.Sprintf(
				`UPDATE mentorix.%[1]s SET block_type = $1 WHERE sort_order = $2`, table), bt, i+1); err != nil {
				t.Fatalf("set %s on %s: %v", bt, table, err)
			}
		}
	}

	clientID := registerClient(t, pool, trainer, "migration-36-client")
	var trainerRowID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM mentorix.trainers WHERE user_id = $1`, trainer).Scan(&trainerRowID); err != nil {
		t.Fatalf("select trainer id: %v", err)
	}
	snapshots := map[string]string{
		"removed": `{"blocks":[
			{"block_type":"strength","instruction":"a","exercises":[{"name":"x","sets":"3"}]},
			{"block_type":"ladder","instruction":"b","exercises":[{"name":"y","sets":"5","reps":"1"}]},
			{"block_type":"complex","exercises":[]},
			{"block_type":"death_by","instruction":"c","exercises":[{"name":"z","name_ru":"я"}]},
			{"block_type":"emom","exercises":[{"name":"w","sets":"2"}]}]}`,
		"untouched": `{"blocks":[{"block_type":"emom","instruction":"k","exercises":[{"name":"x","sets":"3"}]}]}`,
		"empty":     `{"blocks":[]}`,
		"no_blocks": `{}`,
		"null":      `{"blocks":null}`,
		// blocks is an object, not an array: the migration must neither fail nor touch it.
		"object_blocks": `{"blocks":{"block_type":"complex","exercises":[{"name":"x","sets":"3"}]}}`,
	}
	ids := make(map[string]uuid.UUID, len(snapshots))
	for name, snap := range snapshots {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO mentorix.client_workout_completions (
				client_user_id, trainer_id, completed_at, completion_cycle_id, day_key,
				week_number, day_number, program_name, program_name_ru, day_snapshot, result_text, source)
			VALUES ($1, $2, now(), gen_random_uuid(), gen_random_uuid(), 1, 1, 'p', 'п', $3::jsonb, '', 'app')
			RETURNING id`, clientID, trainerRowID, snap).Scan(&id); err != nil {
			t.Fatalf("insert snapshot %s: %v", name, err)
		}
		ids[name] = id
	}

	stepMigration(t, connStr, 1)

	// Blocks: removed types became skill_work in both trees, the rest is unchanged.
	want := []string{"skill_work", "skill_work", "skill_work", "emom", "amrap", "chipper", "strength", "single"}
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT block_type FROM mentorix.%s ORDER BY sort_order, created_at`, table))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var bt string
			if err := rows.Scan(&bt); err != nil {
				t.Fatal(err)
			}
			got = append(got, bt)
		}
		rows.Close()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s block types = %v, want %v", table, got, want)
		}
	}

	// Sets: cleared exactly in the timed blocks, reps and the others untouched.
	for _, table := range []struct{ exercises, blocks, fk string }{
		{"program_week_day_block_exercises", "program_week_day_blocks", "program_week_day_block_id"},
		{"program_version_week_day_block_exercises", "program_version_week_day_blocks", "program_version_week_day_block_id"},
	} {
		rows, err := pool.Query(ctx, fmt.Sprintf(`
			SELECT b.block_type, e.sets, e.reps
			FROM mentorix.%s e JOIN mentorix.%s b ON b.id = e.%s`, table.exercises, table.blocks, table.fk))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var bt string
			var s, r *string
			if err := rows.Scan(&bt, &s, &r); err != nil {
				t.Fatal(err)
			}
			n++
			timed := bt == "emom" || bt == "amrap" || bt == "chipper"
			if timed && s != nil {
				t.Fatalf("%s: %s keeps sets %q", table.exercises, bt, *s)
			}
			if !timed && (s == nil || *s != "3") {
				t.Fatalf("%s: %s sets = %v, want 3", table.exercises, bt, s)
			}
			if r == nil || *r != "10" {
				t.Fatalf("%s: %s reps = %v, want 10", table.exercises, bt, r)
			}
		}
		rows.Close()
		if n != 8 {
			t.Fatalf("%s rows = %d, want 8", table.exercises, n)
		}
	}

	// Snapshots: only block_type of the removed formats changes, nothing else.
	wantSnapshots := map[string]string{
		"removed": `{"blocks":[
			{"block_type":"strength","instruction":"a","exercises":[{"name":"x","sets":"3"}]},
			{"block_type":"skill_work","instruction":"b","exercises":[{"name":"y","sets":"5","reps":"1"}]},
			{"block_type":"skill_work","exercises":[]},
			{"block_type":"skill_work","instruction":"c","exercises":[{"name":"z","name_ru":"я"}]},
			{"block_type":"emom","exercises":[{"name":"w","sets":"2"}]}]}`,
		"untouched":     snapshots["untouched"],
		"empty":         snapshots["empty"],
		"no_blocks":     snapshots["no_blocks"],
		"null":          snapshots["null"],
		"object_blocks": snapshots["object_blocks"],
	}
	for name, id := range ids {
		var got []byte
		if err := pool.QueryRow(ctx, `SELECT day_snapshot::text FROM mentorix.client_workout_completions WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !jsonEqual(t, got, []byte(wantSnapshots[name])) {
			t.Fatalf("snapshot %s = %s, want %s", name, got, wantSnapshots[name])
		}
	}

	// The new CHECK refuses a removed type and the new columns exist.
	if _, err := pool.Exec(ctx, `UPDATE mentorix.program_week_day_blocks SET block_type = 'complex'`); err == nil {
		t.Fatal("block_type complex is still accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE mentorix.program_version_week_day_blocks SET block_type = 'ladder'`); err == nil {
		t.Fatal("version block_type ladder is still accepted")
	}
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM information_schema.columns
			WHERE table_schema = 'mentorix' AND table_name = $1
			  AND column_name IN ('rounds', 'duration_sec', 'time_cap_sec', 'work_sec', 'rest_sec')
			  AND is_nullable = 'YES'`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 5 {
			t.Fatalf("%s nullable setting columns = %d, want 5", table, n)
		}
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal %s: %v", a, err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return string(ab) == string(bb)
}

// TestMigration36_downRestoresTheOldShape pins that the down migration drops the
// settings columns and lets the old block types back in.
func TestMigration36_downRestoresTheOldShape(t *testing.T) {
	pool, connStr := NewPoolWithURL(t)
	ctx := context.Background()

	stepMigration(t, connStr, -1)
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM information_schema.columns
			WHERE table_schema = 'mentorix' AND table_name = $1
			  AND column_name IN ('rounds', 'duration_sec', 'time_cap_sec', 'work_sec', 'rest_sec')`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d setting columns after down", table, n)
		}
	}
	var def string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname = 'program_week_day_blocks_block_type_check'`).Scan(&def); err != nil {
		t.Fatal(err)
	}
	for _, bt := range []string{"ladder", "death_by", "complex"} {
		if !strings.Contains(def, "'"+bt+"'") {
			t.Fatalf("down CHECK misses %s: %s", bt, def)
		}
	}
	stepMigration(t, connStr, 1)
}
