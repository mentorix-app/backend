//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

// This file pins the program working-tree detail, the program list and the
// assignment list against an oracle: the original loaders that issued one
// query per week, per day, per block and per assignment. The oracle lives here
// on purpose, so the production loaders can stay set-based while the proof of
// equality stays independent of them.

// oracleWeeks is the original per-parent loader: weeks, then days per week,
// blocks per day, exercises per block, then the Go sort and the block client
// rules.
func oracleWeeks(ctx context.Context, q *sqlc.Queries, programID uuid.UUID) ([]program.Week, error) {
	weekRows, err := q.ListProgramWeeks(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return nil, err
	}
	weeks := make([]program.Week, 0, len(weekRows))
	for _, w := range weekRows {
		dayRows, err := q.ListProgramDaysForWeek(ctx, w.ID)
		if err != nil {
			return nil, err
		}
		days := make([]program.Day, 0, len(dayRows))
		for _, d := range dayRows {
			blockRows, err := q.ListDayBlocks(ctx, d.ID)
			if err != nil {
				return nil, err
			}
			blocks := make([]program.DayBlock, 0, len(blockRows))
			for _, b := range blockRows {
				exRows, err := q.ListBlockExercises(ctx, b.ID)
				if err != nil {
					return nil, err
				}
				exercises := make([]program.DayExercise, 0, len(exRows))
				for _, e := range exRows {
					exercises = append(exercises, program.DayExercise{
						ID:             pgconv.FromPGUUID(e.ID),
						ExerciseID:     pgconv.FromPGUUID(e.ExerciseID),
						ExerciseName:   e.Name,
						ExerciseNameRu: e.NameRu,
						SortOrder:      int(e.SortOrder),
						Sets:           e.Sets,
						Reps:           e.Reps,
						Instruction:    e.Instruction,
						CreatedAt:      e.CreatedAt.UTC(),
					})
				}
				blocks = append(blocks, program.DayBlock{
					ID:          pgconv.FromPGUUID(b.ID),
					BlockKey:    pgconv.FromPGUUID(b.BlockKey),
					BlockType:   program.BlockType(b.BlockType),
					Instruction: b.Instruction,
					SortOrder:   int(b.SortOrder),
					Exercises:   exercises,
					CreatedAt:   b.CreatedAt.UTC(),
				})
			}
			days = append(days, program.Day{
				ID:        pgconv.FromPGUUID(d.ID),
				DayKey:    pgconv.FromPGUUID(d.DayKey),
				DayNumber: int(d.DayNumber),
				SortOrder: int(d.SortOrder),
				Blocks:    blocks,
				CreatedAt: d.CreatedAt.UTC(),
			})
		}
		weeks = append(weeks, program.Week{
			ID:         pgconv.FromPGUUID(w.ID),
			WeekNumber: int(w.WeekNumber),
			SortOrder:  int(w.SortOrder),
			Days:       days,
			CreatedAt:  w.CreatedAt.UTC(),
		})
	}

	// Same total order as the production sort: sort_order, then id as text.
	sort.Slice(weeks, func(i, j int) bool {
		if weeks[i].SortOrder == weeks[j].SortOrder {
			return weeks[i].ID.String() < weeks[j].ID.String()
		}
		return weeks[i].SortOrder < weeks[j].SortOrder
	})
	for wi := range weeks {
		days := weeks[wi].Days
		sort.Slice(days, func(i, j int) bool {
			if days[i].SortOrder == days[j].SortOrder {
				return days[i].ID.String() < days[j].ID.String()
			}
			return days[i].SortOrder < days[j].SortOrder
		})
		for di := range days {
			blocks := days[di].Blocks
			sort.Slice(blocks, func(i, j int) bool {
				if blocks[i].SortOrder == blocks[j].SortOrder {
					return blocks[i].ID.String() < blocks[j].ID.String()
				}
				return blocks[i].SortOrder < blocks[j].SortOrder
			})
			for bi := range blocks {
				ex := blocks[bi].Exercises
				sort.Slice(ex, func(i, j int) bool {
					if ex[i].SortOrder == ex[j].SortOrder {
						return ex[i].ID.String() < ex[j].ID.String()
					}
					return ex[i].SortOrder < ex[j].SortOrder
				})
			}
		}
	}

	ruleRows, err := q.ListProgramBlockClients(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return nil, err
	}
	rules := make(map[uuid.UUID][]uuid.UUID, len(ruleRows))
	for _, r := range ruleRows {
		key := pgconv.FromPGUUID(r.BlockKey)
		rules[key] = append(rules[key], pgconv.FromPGUUID(r.ClientUserID))
	}
	for wi := range weeks {
		for di := range weeks[wi].Days {
			for bi := range weeks[wi].Days[di].Blocks {
				block := &weeks[wi].Days[di].Blocks[bi]
				ids := rules[block.BlockKey]
				if ids == nil {
					ids = []uuid.UUID{}
				}
				block.ClientUserIDs = ids
			}
		}
	}
	return weeks, nil
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// assertWeeksMatchOracle compares weeks produced by the store with the oracle
// for the same program: deep-equal on the structs (nil and empty slices differ)
// and byte-equal on the JSON.
func assertWeeksMatchOracle(t *testing.T, label string, pool *pgxpool.Pool, programID uuid.UUID, got []program.Week) {
	t.Helper()
	want, err := oracleWeeks(context.Background(), sqlc.New(pool), programID)
	if err != nil {
		t.Fatalf("%s: oracle: %v", label, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: weeks differ from oracle (struct)\n got: %s\nwant: %s", label, mustJSON(t, got), mustJSON(t, want))
	}
	if gj, wj := mustJSON(t, got), mustJSON(t, want); string(gj) != string(wj) {
		t.Fatalf("%s: weeks differ from oracle (json)\n got: %s\nwant: %s", label, gj, wj)
	}
}

type detailFixture struct {
	pool      *pgxpool.Pool
	svc       *program.Service
	store     *program.Store
	trainer   uuid.UUID
	exercises [3]uuid.UUID
}

func newDetailFixture(t *testing.T, pool *pgxpool.Pool, emailPrefix string) *detailFixture {
	t.Helper()
	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainer, err := auth.NewStore(pool).RegisterTrainerEmailPassword(ctx, emailPrefix+"@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	f := &detailFixture{
		pool:    pool,
		svc:     program.NewService(pool, program.WithQuotaChecker(nil)),
		store:   program.NewStore(pool),
		trainer: trainer,
	}
	names := [3][2]string{{"Squat", "Присед"}, {"Bench", "Жим"}, {"Row", "Тяга"}}
	for i, n := range names {
		ex, err := exercise.NewStore(pool).Create(ctx, trainer, nil, exercise.UpsertInput{
			Name:        n[0],
			NameRu:      n[1],
			Type:        exercise.ExerciseTypeStrength,
			MuscleGroup: exercise.MuscleGroupLegs,
			Difficulty:  exercise.DifficultyBeginner,
		})
		if err != nil {
			t.Fatalf("create exercise: %v", err)
		}
		f.exercises[i] = ex.ID
	}
	return f
}

func (f *detailFixture) ex(i int, sets, reps, instruction string) program.DayExerciseInput {
	in := program.DayExerciseInput{ExerciseID: f.exercises[i%3]}
	if sets != "" {
		in.Sets = &sets
	}
	if reps != "" {
		in.Reps = &reps
	}
	if instruction != "" {
		in.Instruction = &instruction
	}
	return in
}

func (f *detailFixture) publishable(t *testing.T, programID uuid.UUID, name string) {
	t.Helper()
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := f.svc.Update(context.Background(), f.trainer, programID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func (f *detailFixture) addBlock(t *testing.T, d program.Detail, weekIdx, dayIdx int, in program.DayExerciseInput) program.Detail {
	t.Helper()
	w := d.Weeks[weekIdx]
	out, err := createSingleBlock(context.Background(), f.svc, f.trainer, d.ID, w.ID, w.Days[dayIdx].ID, in)
	if err != nil {
		t.Fatalf("create block: %v", err)
	}
	return out
}

// buildRichProgram creates a draft with 3 weeks, days with zero, one and
// several blocks, a group block with three exercises, per-client visibility
// rules, and a week, a day, a block and an exercise reordered so sort_order
// differs from creation order.
func (f *detailFixture) buildRichProgram(t *testing.T, name string, clients []uuid.UUID) program.Detail {
	t.Helper()
	ctx := context.Background()

	d, err := f.svc.Create(ctx, f.trainer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d, err = f.svc.AddWeek(ctx, f.trainer, d.ID); err != nil {
		t.Fatalf("AddWeek: %v", err)
	}
	if d, err = f.svc.AddWeek(ctx, f.trainer, d.ID); err != nil {
		t.Fatalf("AddWeek: %v", err)
	}
	if len(d.Weeks) != 3 {
		t.Fatalf("weeks = %d, want 3", len(d.Weeks))
	}

	// Week 1: day 0 stays empty; day 1 gets one single block; day 2 gets
	// three singles, two of them merged into a group.
	d = f.addBlock(t, d, 0, 1, f.ex(0, "3", "10", "slow tempo"))
	d = f.addBlock(t, d, 0, 2, f.ex(1, "5/4", "8", ""))
	d = f.addBlock(t, d, 0, 2, f.ex(2, "", "", "unilateral"))
	d = f.addBlock(t, d, 0, 2, f.ex(0, "4", "3-6", ""))
	w1 := d.Weeks[0]
	day2 := w1.Days[2]
	d, err = f.svc.MergeDayBlocks(ctx, f.trainer, d.ID, w1.ID, day2.ID, []uuid.UUID{day2.Blocks[0].ID, day2.Blocks[1].ID})
	if err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	day2 = d.Weeks[0].Days[2]
	var group program.DayBlock
	for _, b := range day2.Blocks {
		if len(b.Exercises) > 1 {
			group = b
		}
	}
	if group.ID == uuid.Nil {
		t.Fatal("no group block after merge")
	}
	d, err = f.svc.AddBlockExercise(ctx, f.trainer, d.ID, w1.ID, group.ID, f.ex(1, "2", "12", "finisher"))
	if err != nil {
		t.Fatalf("AddBlockExercise: %v", err)
	}

	// Week 2: two blocks on day 0, one on day 3. Week 3: one block on day 6.
	d = f.addBlock(t, d, 1, 0, f.ex(2, "3", "5", ""))
	d = f.addBlock(t, d, 1, 0, f.ex(0, "3", "5", ""))
	d = f.addBlock(t, d, 1, 3, f.ex(1, "1", "1", ""))
	d = f.addBlock(t, d, 2, 6, f.ex(2, "6", "6", ""))

	// Reorders: weeks reversed, days of week 1 reversed, blocks of week 1 day 2
	// reversed, exercises of the group reversed.
	ids := make([]uuid.UUID, 0, len(d.Weeks))
	for i := len(d.Weeks) - 1; i >= 0; i-- {
		ids = append(ids, d.Weeks[i].ID)
	}
	if d, err = f.svc.ReorderWeeks(ctx, f.trainer, d.ID, ids); err != nil {
		t.Fatalf("ReorderWeeks: %v", err)
	}
	if d.Weeks[0].ID == w1.ID {
		t.Fatal("weeks were not reordered: the first created week is still first")
	}
	var week1 program.Week
	for _, w := range d.Weeks {
		if w.ID == w1.ID {
			week1 = w
		}
	}
	dayIDs := make([]uuid.UUID, 0, len(week1.Days))
	for i := len(week1.Days) - 1; i >= 0; i-- {
		dayIDs = append(dayIDs, week1.Days[i].ID)
	}
	if d, err = f.svc.ReorderDays(ctx, f.trainer, d.ID, week1.ID, dayIDs); err != nil {
		t.Fatalf("ReorderDays: %v", err)
	}
	var day2After program.Day
	for _, w := range d.Weeks {
		for _, dy := range w.Days {
			if dy.ID == day2.ID {
				day2After = dy
			}
		}
	}
	blockIDs := make([]uuid.UUID, 0, len(day2After.Blocks))
	for i := len(day2After.Blocks) - 1; i >= 0; i-- {
		blockIDs = append(blockIDs, day2After.Blocks[i].ID)
	}
	if d, err = f.svc.ReorderDayBlocks(ctx, f.trainer, d.ID, week1.ID, day2.ID, blockIDs); err != nil {
		t.Fatalf("ReorderDayBlocks: %v", err)
	}
	exIDs := make([]uuid.UUID, 0, len(group.Exercises)+1)
	for _, w := range d.Weeks {
		for _, dy := range w.Days {
			for _, b := range dy.Blocks {
				if b.ID == group.ID {
					for i := len(b.Exercises) - 1; i >= 0; i-- {
						exIDs = append(exIDs, b.Exercises[i].ID)
					}
				}
			}
		}
	}
	if d, err = f.svc.ReorderBlockExercises(ctx, f.trainer, d.ID, week1.ID, group.ID, exIDs); err != nil {
		t.Fatalf("ReorderBlockExercises: %v", err)
	}

	// Visibility rules on the group block (two clients) and on one single.
	if len(clients) >= 2 {
		for _, c := range clients[:2] {
			if _, err := f.pool.Exec(ctx, `
				INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
				VALUES ($1, $2, $3)`, d.ID, group.BlockKey, c); err != nil {
				t.Fatalf("insert rule: %v", err)
			}
		}
		single := d.Weeks[1].Days[0].Blocks[0]
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
			VALUES ($1, $2, $3)`, d.ID, single.BlockKey, clients[0]); err != nil {
			t.Fatalf("insert rule: %v", err)
		}
	}

	f.publishable(t, d.ID, name)
	got, err := f.store.GetDetail(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	return got
}

func registerClient(t *testing.T, pool *pgxpool.Pool, trainerUserID uuid.UUID, prefix string) uuid.UUID {
	t.Helper()
	id, _ := seedAssignedClient(t, pool, trainerUserID, prefix)
	return id
}

// assertProgramQueriesScoped calls the whole-program queries directly and
// checks that they return only rows of the requested program: every parent key
// belongs to the set collected from the level above, and the row counts equal
// the per-parent totals of the oracle queries. Without it, rows of another
// program would only be grouped under parent keys that are never read.
func assertProgramQueriesScoped(t *testing.T, pool *pgxpool.Pool, programID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	q := sqlc.New(pool)
	pid := pgconv.ToPGUUID(programID)

	weeks, err := q.ListProgramWeeks(ctx, pid)
	if err != nil {
		t.Fatalf("ListProgramWeeks: %v", err)
	}
	weekIDs := map[pgtype.UUID]bool{}
	var wantDays, wantBlocks, wantExercises int
	dayIDs := map[pgtype.UUID]bool{}
	blockIDs := map[pgtype.UUID]bool{}
	for _, w := range weeks {
		weekIDs[w.ID] = true
		days, err := q.ListProgramDaysForWeek(ctx, w.ID)
		if err != nil {
			t.Fatalf("ListProgramDaysForWeek: %v", err)
		}
		wantDays += len(days)
		for _, d := range days {
			dayIDs[d.ID] = true
			blocks, err := q.ListDayBlocks(ctx, d.ID)
			if err != nil {
				t.Fatalf("ListDayBlocks: %v", err)
			}
			wantBlocks += len(blocks)
			for _, b := range blocks {
				blockIDs[b.ID] = true
				exs, err := q.ListBlockExercises(ctx, b.ID)
				if err != nil {
					t.Fatalf("ListBlockExercises: %v", err)
				}
				wantExercises += len(exs)
			}
		}
	}
	if wantDays == 0 || wantBlocks == 0 || wantExercises == 0 {
		t.Fatalf("scope check needs a populated program: days=%d blocks=%d exercises=%d", wantDays, wantBlocks, wantExercises)
	}

	days, err := q.ListProgramDays(ctx, pid)
	if err != nil {
		t.Fatalf("ListProgramDays: %v", err)
	}
	if len(days) != wantDays {
		t.Errorf("ListProgramDays returned %d rows, want %d", len(days), wantDays)
	}
	for _, d := range days {
		if !weekIDs[d.WeekID] {
			t.Errorf("ListProgramDays returned day %v of a foreign week %v", d.ID, d.WeekID)
		}
	}
	blocks, err := q.ListProgramBlocks(ctx, pid)
	if err != nil {
		t.Fatalf("ListProgramBlocks: %v", err)
	}
	if len(blocks) != wantBlocks {
		t.Errorf("ListProgramBlocks returned %d rows, want %d", len(blocks), wantBlocks)
	}
	for _, b := range blocks {
		if !dayIDs[b.ProgramWeekDayID] {
			t.Errorf("ListProgramBlocks returned block %v of a foreign day %v", b.ID, b.ProgramWeekDayID)
		}
	}
	exs, err := q.ListProgramBlockExercises(ctx, pid)
	if err != nil {
		t.Fatalf("ListProgramBlockExercises: %v", err)
	}
	if len(exs) != wantExercises {
		t.Errorf("ListProgramBlockExercises returned %d rows, want %d", len(exs), wantExercises)
	}
	for _, e := range exs {
		if !blockIDs[e.ProgramWeekDayBlockID] {
			t.Errorf("ListProgramBlockExercises returned exercise %v of a foreign block %v", e.ID, e.ProgramWeekDayBlockID)
		}
	}
}

func TestProgramDetail_batchLoadMatchesPerParentOracle(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	f := newDetailFixture(t, pool, "detail-oracle-trainer")
	clients := []uuid.UUID{
		registerClient(t, pool, f.trainer, "detail-oracle-c1"),
		registerClient(t, pool, f.trainer, "detail-oracle-c2"),
	}

	d := f.buildRichProgram(t, "Oracle Program", clients)
	// A second populated program of the same trainer: its rows must never leak
	// into the first program's whole-program reads.
	other := f.buildRichProgram(t, "Other Program", clients)
	if other.ID == d.ID {
		t.Fatal("second program has the same id as the first")
	}
	assertProgramQueriesScoped(t, pool, d.ID)

	// Shape guard: the program really exercises every case the loader has.
	var emptyDays, oneBlockDays, manyBlockDays, groups, multiExercise, restricted int
	for _, w := range d.Weeks {
		for _, dy := range w.Days {
			switch len(dy.Blocks) {
			case 0:
				emptyDays++
			case 1:
				oneBlockDays++
			default:
				manyBlockDays++
			}
			for _, b := range dy.Blocks {
				if len(b.Exercises) > 1 {
					multiExercise++
				}
				if b.BlockType != program.BlockTypeSingle {
					groups++
				}
				if len(b.ClientUserIDs) > 0 {
					restricted++
				}
			}
		}
	}
	if len(d.Weeks) != 3 || emptyDays == 0 || oneBlockDays == 0 || manyBlockDays == 0 || groups == 0 || multiExercise == 0 || restricted < 2 {
		t.Fatalf("fixture too thin: weeks=%d empty=%d one=%d many=%d groups=%d multi=%d restricted=%d",
			len(d.Weeks), emptyDays, oneBlockDays, manyBlockDays, groups, multiExercise, restricted)
	}
	assertWeeksMatchOracle(t, "draft", pool, d.ID, d.Weeks)

	published, err := f.svc.Publish(ctx, f.trainer, d.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.HasUnpublishedChanges {
		t.Fatal("HasUnpublishedChanges after publish = true, want false")
	}
	assertWeeksMatchOracle(t, "published (publish result)", pool, d.ID, published.Weeks)
	got, err := f.store.GetDetail(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	assertWeeksMatchOracle(t, "published (GetDetail)", pool, d.ID, got.Weeks)
	if got.HasUnpublishedChanges {
		t.Fatal("HasUnpublishedChanges on GetDetail = true, want false")
	}

	edited := f.addBlock(t, got, 0, 0, f.ex(1, "2", "2", "post-publish"))
	if !edited.HasUnpublishedChanges {
		t.Fatal("HasUnpublishedChanges after edit = false, want true")
	}
	assertWeeksMatchOracle(t, "after post-publish edit (mutation result)", pool, d.ID, edited.Weeks)
	got, err = f.store.GetDetail(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	assertWeeksMatchOracle(t, "after post-publish edit (GetDetail)", pool, d.ID, got.Weeks)
	assertProgramQueriesScoped(t, pool, d.ID)
	assertProgramQueriesScoped(t, pool, other.ID)
	if !got.HasUnpublishedChanges {
		t.Fatal("HasUnpublishedChanges on GetDetail after edit = false, want true")
	}
}

// queryCounter counts statements that reach the server through a pool.
type queryCounter struct {
	mu  sync.Mutex
	sql []string
}

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, data.SQL)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *queryCounter) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = nil
}

func (c *queryCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sql)
}

func (c *queryCounter) statements() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.sql))
	for _, s := range c.sql {
		first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
		names = append(names, first)
	}
	return strings.Join(names, " | ")
}

func newCountingPool(t *testing.T, url string) (*pgxpool.Pool, *queryCounter) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pgxpool with tracer: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, counter
}

// Upper bounds, independent of the number of weeks, days and blocks:
// detail = program row + weeks + days + blocks + exercises + rules, then
// enrich = assignment count + latest version.
const (
	maxQueriesPerDetail = 8
	// list = count + page, then 2 batched aggregates and 4 tree queries per
	// published program.
	listFixedQueries        = 4
	listQueriesPerPublished = 4
)

func TestProgramDetail_queryCountIsConstant(t *testing.T) {
	pool, url := NewPoolWithURL(t)
	ctx := context.Background()
	f := newDetailFixture(t, pool, "detail-count-trainer")
	clients := []uuid.UUID{
		registerClient(t, pool, f.trainer, "detail-count-c1"),
		registerClient(t, pool, f.trainer, "detail-count-c2"),
	}

	rich := f.buildRichProgram(t, "Rich", clients)
	small, err := f.svc.Create(ctx, f.trainer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cpool, counter := newCountingPool(t, url)
	cstore := program.NewStore(cpool)

	counts := map[string]int{}
	for label, id := range map[string]uuid.UUID{"small": small.ID, "rich": rich.ID} {
		counter.reset()
		if _, err := cstore.GetDetail(ctx, id); err != nil {
			t.Fatalf("GetDetail %s: %v", label, err)
		}
		counts[label] = counter.count()
		t.Logf("GetDetail %s: %d queries: %s", label, counts[label], counter.statements())
	}
	if counts["small"] != counts["rich"] {
		t.Fatalf("query count depends on program size: small=%d rich=%d", counts["small"], counts["rich"])
	}
	if counts["rich"] > maxQueriesPerDetail {
		t.Fatalf("GetDetail issued %d queries, want at most %d", counts["rich"], maxQueriesPerDetail)
	}
}

func TestProgramList_queryCountAndEquality(t *testing.T) {
	pool, url := NewPoolWithURL(t)
	ctx := context.Background()
	// Two trainers: the free plan allows three active programs each.
	f := newDetailFixture(t, pool, "list-oracle-trainer")
	clients := []uuid.UUID{
		registerClient(t, pool, f.trainer, "list-oracle-c1"),
		registerClient(t, pool, f.trainer, "list-oracle-c2"),
	}
	f2 := newDetailFixture(t, pool, "list-oracle-trainer2")
	clients2 := []uuid.UUID{
		registerClient(t, pool, f2.trainer, "list-oracle-c3"),
		registerClient(t, pool, f2.trainer, "list-oracle-c4"),
	}

	// Draft: rich tree, never published.
	draft := f.buildRichProgram(t, "List Draft", clients)

	// Published, unchanged, with two active assignments.
	unchanged := f.buildRichProgram(t, "List Unchanged", clients)
	if _, err := f.svc.Publish(ctx, f.trainer, unchanged.ID); err != nil {
		t.Fatalf("Publish unchanged: %v", err)
	}
	for _, c := range clients {
		pid := unchanged.ID
		if _, err := f.svc.SetClientProgramAssignment(ctx, f.trainer, c, &pid); err != nil {
			t.Fatalf("assign: %v", err)
		}
	}

	// Published with unpublished changes.
	changed := f.buildRichProgram(t, "List Changed", clients)
	if _, err := f.svc.Publish(ctx, f.trainer, changed.ID); err != nil {
		t.Fatalf("Publish changed: %v", err)
	}
	changed = f.addBlock(t, changed, 0, 0, f.ex(2, "1", "1", "unpublished"))
	if !changed.HasUnpublishedChanges {
		t.Fatal("changed program has no unpublished changes")
	}

	// Published twice (two versions), then archived.
	archived := f2.buildRichProgram(t, "List Archived", clients2)
	if _, err := f2.svc.Publish(ctx, f2.trainer, archived.ID); err != nil {
		t.Fatalf("Publish archived: %v", err)
	}
	f2.addBlock(t, archived, 1, 1, f2.ex(0, "2", "2", "v2"))
	if _, err := f2.svc.PublishUpdate(ctx, f2.trainer, archived.ID); err != nil {
		t.Fatalf("PublishUpdate archived: %v", err)
	}
	if _, err := f2.svc.Archive(ctx, f2.trainer, archived.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Empty draft: no weeks content at all.
	empty, err := f2.svc.Create(ctx, f2.trainer)
	if err != nil {
		t.Fatalf("Create empty: %v", err)
	}

	params, err := program.ParseListParams("1", "100", "name", "asc", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}

	cpool, counter := newCountingPool(t, url)
	cstore := program.NewStore(cpool)
	counter.reset()
	res, err := cstore.List(ctx, params)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	listQueries := counter.count()
	t.Logf("List page of %d (2 published with a version): %d queries: %s", len(res.Items), listQueries, counter.statements())

	if len(res.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(res.Items))
	}
	byID := map[uuid.UUID]program.Program{}
	for _, it := range res.Items {
		byID[it.ID] = it
	}

	q := sqlc.New(pool)
	published := 0
	for _, id := range []uuid.UUID{draft.ID, unchanged.ID, changed.ID, archived.ID, empty.ID} {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("program %s missing from list", id)
		}

		// Oracle: the original per-row enrichment.
		want := got
		want.AssignmentCount = 0
		want.LatestProgramVersionID = nil
		want.LatestClientPlanAt = nil
		want.HasUnpublishedChanges = false

		pid := pgconv.ToPGUUID(id)
		count, err := q.CountActiveProgramAssignmentsByProgramID(ctx, pid)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		want.AssignmentCount = int(count)

		var sqlTrainingDays int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT d.id)::int
			FROM mentorix.program_week_days d
			JOIN mentorix.program_weeks w ON w.id = d.week_id
			WHERE w.program_id = $1
			  AND EXISTS (SELECT 1 FROM mentorix.program_week_day_blocks b WHERE b.program_week_day_id = d.id)`,
			id).Scan(&sqlTrainingDays); err != nil {
			t.Fatalf("training days: %v", err)
		}
		want.TrainingDaysCount = sqlTrainingDays

		latest, err := q.GetLatestProgramVersionByProgramID(ctx, pid)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			t.Fatalf("latest: %v", err)
		default:
			lid := pgconv.FromPGUUID(latest.ID)
			want.LatestProgramVersionID = &lid
			at := latest.PublishedAt.UTC()
			want.LatestClientPlanAt = &at
			if got.Status == program.StatusPublished {
				published++
				weeks, err := oracleWeeks(ctx, q, id)
				if err != nil {
					t.Fatalf("oracle weeks: %v", err)
				}
				want.TrainingDaysCount = program.CountTrainingDays(weeks)
				fp, err := program.DetailFingerprint(program.Detail{Program: got, Weeks: weeks})
				if err != nil {
					t.Fatalf("fingerprint: %v", err)
				}
				want.HasUnpublishedChanges = fp != latest.ContentFingerprint
			}
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("list item %q differs from per-row oracle\n got: %s\nwant: %s", got.Name, mustJSON(t, got), mustJSON(t, want))
		}
		if string(mustJSON(t, got)) != string(mustJSON(t, want)) {
			t.Fatalf("list item %q JSON differs from per-row oracle", got.Name)
		}
	}
	if published != 2 {
		t.Fatalf("published programs with a version = %d, want 2", published)
	}

	// Pin the interesting values so the oracle cannot agree on a degenerate case.
	if !byID[changed.ID].HasUnpublishedChanges || byID[unchanged.ID].HasUnpublishedChanges ||
		byID[draft.ID].HasUnpublishedChanges || byID[archived.ID].HasUnpublishedChanges {
		t.Fatalf("has_unpublished_changes: changed=%v unchanged=%v draft=%v archived=%v",
			byID[changed.ID].HasUnpublishedChanges, byID[unchanged.ID].HasUnpublishedChanges,
			byID[draft.ID].HasUnpublishedChanges, byID[archived.ID].HasUnpublishedChanges)
	}
	if byID[unchanged.ID].AssignmentCount != 2 || byID[archived.ID].LatestProgramVersionID == nil ||
		byID[draft.ID].LatestProgramVersionID != nil {
		t.Fatal("assignment count / latest version fields are not what the fixture implies")
	}

	// Published count: the page above has 2 published programs (unchanged, changed).
	if limit := listFixedQueries + listQueriesPerPublished*2; listQueries > limit {
		t.Fatalf("List issued %d queries, want at most %d", listQueries, limit)
	}
}

// oracleAssignment is the original per-row conversion: one version lookup and
// one latest-version lookup per assignment.
func oracleAssignment(ctx context.Context, q *sqlc.Queries, row sqlc.MentorixProgramAssignment) (program.Assignment, error) {
	version, err := q.GetProgramVersionByID(ctx, row.ProgramVersionID)
	if err != nil {
		return program.Assignment{}, err
	}
	latest, err := q.GetLatestProgramVersionByProgramID(ctx, row.ProgramID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return program.Assignment{}, err
	}
	a := program.Assignment{
		ID:                pgconv.FromPGUUID(row.ID),
		ProgramID:         pgconv.FromPGUUID(row.ProgramID),
		ProgramVersionID:  pgconv.FromPGUUID(row.ProgramVersionID),
		TrainerID:         pgconv.FromPGUUID(row.TrainerID),
		ClientUserID:      pgconv.FromPGUUID(row.ClientUserID),
		Status:            program.AssignmentStatus(row.Status),
		AssignedAt:        row.AssignedAt.UTC(),
		CreatedAt:         row.CreatedAt.UTC(),
		CompletionCycleID: pgconv.FromPGUUID(row.CompletionCycleID),
	}
	publishedAt := version.PublishedAt.UTC()
	a.ClientPlanAt = &publishedAt
	if err == nil {
		behind := pgconv.FromPGUUID(latest.ID) != a.ProgramVersionID
		a.IsBehindLatest = &behind
	}
	return a, nil
}

func TestProgramAssignments_listMatchesPerRowOracle(t *testing.T) {
	pool, url := NewPoolWithURL(t)
	ctx := context.Background()
	f := newDetailFixture(t, pool, "assign-oracle-trainer")
	var clients []uuid.UUID
	for _, p := range []string{"a1", "a2", "a3", "a4"} {
		clients = append(clients, registerClient(t, pool, f.trainer, "assign-oracle-"+p))
	}

	d := f.buildRichProgram(t, "Assign Oracle", clients[:2])
	if _, err := f.svc.Publish(ctx, f.trainer, d.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	pid := d.ID
	assign := func(c uuid.UUID) {
		t.Helper()
		if _, err := f.svc.SetClientProgramAssignment(ctx, f.trainer, c, &pid); err != nil {
			t.Fatalf("assign: %v", err)
		}
		time.Sleep(5 * time.Millisecond) // distinct assigned_at keeps the page order unambiguous
	}
	assign(clients[0])
	assign(clients[1])
	assign(clients[3])
	// Client 4 leaves again: an inactive row must stay out of the list.
	if _, err := f.svc.SetClientProgramAssignment(ctx, f.trainer, clients[3], nil); err != nil {
		t.Fatalf("clear assignment: %v", err)
	}

	// A second version, then one more client on it: two assignments on v1, one on v2.
	f.addBlock(t, d, 0, 0, f.ex(1, "2", "2", "v2"))
	if _, err := f.svc.PublishUpdate(ctx, f.trainer, d.ID); err != nil {
		t.Fatalf("PublishUpdate: %v", err)
	}
	assign(clients[2])

	cpool, counter := newCountingPool(t, url)
	cstore := program.NewStore(cpool)
	counter.reset()
	got, err := cstore.ListProgramAssignments(ctx, d.ID)
	if err != nil {
		t.Fatalf("ListProgramAssignments: %v", err)
	}
	queries := counter.count()
	t.Logf("ListProgramAssignments (%d items): %d queries: %s", len(got.Items), queries, counter.statements())

	q := sqlc.New(pool)
	rows, err := q.ListActiveProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(d.ID))
	if err != nil {
		t.Fatalf("oracle rows: %v", err)
	}
	latest, err := q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(d.ID))
	if err != nil {
		t.Fatalf("oracle latest: %v", err)
	}
	latestID := pgconv.FromPGUUID(latest.ID)
	want := program.AssignmentListResult{Items: make([]program.Assignment, 0, len(rows)), LatestProgramVersionID: &latestID}
	for _, row := range rows {
		a, err := oracleAssignment(ctx, q, row)
		if err != nil {
			t.Fatalf("oracle assignment: %v", err)
		}
		want.Items = append(want.Items, a)
	}

	if len(got.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(got.Items))
	}
	var behind, current int
	for _, a := range got.Items {
		if a.IsBehindLatest != nil && *a.IsBehindLatest {
			behind++
		} else {
			current++
		}
	}
	if behind != 2 || current != 1 {
		t.Fatalf("behind=%d current=%d, want 2 and 1", behind, current)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assignments differ from per-row oracle\n got: %s\nwant: %s", mustJSON(t, got), mustJSON(t, want))
	}
	if string(mustJSON(t, got)) != string(mustJSON(t, want)) {
		t.Fatal("assignments JSON differs from per-row oracle")
	}

	const maxAssignmentListQueries = 2
	if queries > maxAssignmentListQueries {
		t.Fatalf("ListProgramAssignments issued %d queries, want at most %d", queries, maxAssignmentListQueries)
	}

	// A program that was never published has no assignments and no latest version.
	empty, err := f.svc.Create(ctx, f.trainer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	none, err := cstore.ListProgramAssignments(ctx, empty.ID)
	if err != nil {
		t.Fatalf("ListProgramAssignments empty: %v", err)
	}
	if none.LatestProgramVersionID != nil || none.Items == nil || len(none.Items) != 0 {
		t.Fatalf("empty list = %+v, want non-nil empty items and no latest version", none)
	}
}
