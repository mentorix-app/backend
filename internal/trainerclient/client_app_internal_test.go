package trainerclient

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/workoutcompletion"
)

func dayWith(key uuid.UUID, number int, blocks ...program.DayBlock) program.Day {
	return program.Day{DayKey: key, DayNumber: number, Blocks: blocks}
}

func blockWithExercise(clients ...uuid.UUID) program.DayBlock {
	return program.DayBlock{
		BlockType:     program.BlockTypeSingle,
		ClientUserIDs: clients,
		Exercises:     []program.DayExercise{{ExerciseName: "Squat"}},
	}
}

func TestMatchesCurrentAssignment(t *testing.T) {
	assignmentID, versionID, cycleID := uuid.New(), uuid.New(), uuid.New()
	current := &program.Assignment{ID: assignmentID, ProgramVersionID: versionID, CompletionCycleID: cycleID}
	req := ClientCompletionRequest{ProgramAssignmentID: assignmentID, ProgramVersionID: versionID, CompletionCycleID: cycleID}

	tests := []struct {
		name       string
		assignment *program.Assignment
		mutate     func(*ClientCompletionRequest)
		want       bool
	}{
		{"all three ids match", current, func(*ClientCompletionRequest) {}, true},
		{"no assignment", nil, func(*ClientCompletionRequest) {}, false},
		{"other assignment", current, func(r *ClientCompletionRequest) { r.ProgramAssignmentID = uuid.New() }, false},
		{"other version", current, func(r *ClientCompletionRequest) { r.ProgramVersionID = uuid.New() }, false},
		// A to B to A brings the assignment and the version back; only the cycle differs.
		{"other cycle", current, func(r *ClientCompletionRequest) { r.CompletionCycleID = uuid.New() }, false},
		{"zero cycle", current, func(r *ClientCompletionRequest) { r.CompletionCycleID = uuid.Nil }, false},
		{"all three other", current, func(r *ClientCompletionRequest) {
			r.ProgramAssignmentID, r.ProgramVersionID, r.CompletionCycleID = uuid.New(), uuid.New(), uuid.New()
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := req
			tt.mutate(&r)
			if got := matchesCurrentAssignment(tt.assignment, r); got != tt.want {
				t.Fatalf("matchesCurrentAssignment = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindWorkoutDay(t *testing.T) {
	keyOne, keyTwo, keyRest, keyEmptyBlock := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	detail := program.Detail{Weeks: []program.Week{
		{WeekNumber: 1, Days: []program.Day{
			dayWith(keyOne, 1, blockWithExercise()),
			dayWith(keyRest, 2),
		}},
		{WeekNumber: 2, Days: []program.Day{
			dayWith(keyTwo, 4, blockWithExercise(), blockWithExercise()),
			dayWith(keyEmptyBlock, 5, program.DayBlock{BlockType: program.BlockTypeSingle}),
		}},
	}}

	tests := []struct {
		name       string
		key        uuid.UUID
		wantOK     bool
		wantWeek   int
		wantDay    int
		wantBlocks int
	}{
		{"first day of week one", keyOne, true, 1, 1, 1},
		{"later week gives its week and day numbers", keyTwo, true, 2, 4, 2},
		{"a day with no blocks is not a workout day", keyRest, false, 0, 0, 0},
		{"a day whose blocks hold no exercises is not a workout day", keyEmptyBlock, false, 0, 0, 0},
		{"unknown key", uuid.New(), false, 0, 0, 0},
		{"zero key", uuid.Nil, false, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			week, dayNumber, day, ok := findWorkoutDay(detail, tt.key)
			if ok != tt.wantOK || week != tt.wantWeek || dayNumber != tt.wantDay {
				t.Fatalf("findWorkoutDay = (%d, %d, ok %v), want (%d, %d, ok %v)", week, dayNumber, ok, tt.wantWeek, tt.wantDay, tt.wantOK)
			}
			if ok && (day.DayKey != tt.key || len(day.Blocks) != tt.wantBlocks) {
				t.Fatalf("day = %+v, want key %v with %d blocks", day, tt.key, tt.wantBlocks)
			}
		})
	}
}

func TestFindWorkoutDay_emptyDetail(t *testing.T) {
	if _, _, _, ok := findWorkoutDay(program.Detail{}, uuid.New()); ok {
		t.Fatal("found a day in an empty program")
	}
}

func TestHideOtherClients(t *testing.T) {
	me, other, third := uuid.New(), uuid.New(), uuid.New()
	shared := blockWithExercise()
	shared.ClientUserIDs = nil
	restricted := blockWithExercise(me, other, third)
	solo := blockWithExercise(me)
	in := program.Detail{Weeks: []program.Week{{WeekNumber: 1, Days: []program.Day{
		dayWith(uuid.New(), 1, shared, restricted, solo),
	}}}}

	got := hideOtherClients(in, me)

	blocks := got.Weeks[0].Days[0].Blocks
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(blocks))
	}
	if blocks[0].ClientUserIDs == nil || len(blocks[0].ClientUserIDs) != 0 {
		t.Errorf("shared block client_user_ids = %#v, want a non-nil empty slice", blocks[0].ClientUserIDs)
	}
	for i, b := range blocks[1:] {
		if !reflect.DeepEqual(b.ClientUserIDs, []uuid.UUID{me}) {
			t.Errorf("restricted block %d client_user_ids = %v, want only the caller", i, b.ClientUserIDs)
		}
	}
	// The input keeps the full rules: the helper works on a copy.
	if !reflect.DeepEqual(in.Weeks[0].Days[0].Blocks[1].ClientUserIDs, []uuid.UUID{me, other, third}) {
		t.Errorf("input block was changed: %v", in.Weeks[0].Days[0].Blocks[1].ClientUserIDs)
	}
}

func TestNewClientProgram_carriesOnlyWhatTheClientNeeds(t *testing.T) {
	me, other := uuid.New(), uuid.New()
	category := program.CategoryMuscleGain
	detail := program.Detail{
		Program: program.Program{
			ID: uuid.New(), CreatedBy: uuid.New(), CreatedByName: "Coach", ModifiedBy: uuid.New(),
			Status: program.StatusPublished, Name: "Strength", NameRu: "Сила", Description: "d", DescriptionRu: "о",
			Category: &category, PreviewImageURL: "https://img", LatestProgramVersionID: &uuid.UUID{1},
			HasUnpublishedChanges: true, AssignmentCount: 9, TrainingDaysCount: 4,
		},
		Weeks: []program.Week{{WeekNumber: 1, Days: []program.Day{dayWith(uuid.New(), 1, blockWithExercise(me, other))}}},
	}

	got := newClientProgram(detail, me)

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	want := []string{"category", "description", "description_ru", "difficulty", "name", "name_ru", "preview_image_url", "program_id", "weeks"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("keys = %v, want exactly %v", names, want)
	}
	if got.ProgramID != detail.ID || got.Name != "Strength" || got.NameRu != "Сила" || got.Category == nil || *got.Category != category {
		t.Fatalf("program = %+v", got)
	}
	if string(keys["difficulty"]) != "null" {
		t.Errorf("difficulty = %s, want null when unset", keys["difficulty"])
	}
	if ids := got.Weeks[0].Days[0].Blocks[0].ClientUserIDs; !reflect.DeepEqual(ids, []uuid.UUID{me}) {
		t.Errorf("client_user_ids = %v, want only the caller", ids)
	}
}

type fakeCompletions struct {
	got   workoutcompletion.CompleteInput
	calls int
	out   workoutcompletion.Completion
	err   error
}

func (f *fakeCompletions) CompletedDayKeys(context.Context, uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return nil, nil
}

func (f *fakeCompletions) CompleteForAssignment(_ context.Context, in workoutcompletion.CompleteInput) (workoutcompletion.Completion, error) {
	f.calls++
	f.got = in
	return f.out, f.err
}

type markFixture struct {
	client, trainer uuid.UUID
	assignment      *program.Assignment
	detail          program.Detail
	dayKey          uuid.UUID
	req             ClientCompletionRequest
}

func newMarkFixture() markFixture {
	m := markFixture{client: uuid.New(), trainer: uuid.New(), dayKey: uuid.New()}
	m.assignment = &program.Assignment{
		ID: uuid.New(), ProgramID: uuid.New(), ProgramVersionID: uuid.New(), CompletionCycleID: uuid.New(),
	}
	m.detail = program.Detail{
		Program: program.Program{Name: "Strength", NameRu: "Сила"},
		Weeks:   []program.Week{{WeekNumber: 2, Days: []program.Day{dayWith(m.dayKey, 3, blockWithExercise())}}},
	}
	m.req = ClientCompletionRequest{
		ProgramAssignmentID: m.assignment.ID, ProgramVersionID: m.assignment.ProgramVersionID,
		CompletionCycleID: m.assignment.CompletionCycleID, DayKey: m.dayKey, ResultText: "done",
	}
	return m
}

func (m markFixture) mark(f *fakeCompletions) (ClientCompletionResponse, error) {
	s := &Service{completions: f}
	return s.markDay(context.Background(), m.client, m.trainer, m.assignment, m.detail, m.req)
}

func TestMarkDay_writesAnAppMarkOfTheCurrentCycle(t *testing.T) {
	m := newMarkFixture()
	completedAt := time.Date(2026, 10, 18, 7, 0, 0, 0, time.UTC)
	f := &fakeCompletions{out: workoutcompletion.Completion{
		ID: uuid.New(), DayKey: m.dayKey, WeekNumber: 2, DayNumber: 3, CompletedAt: completedAt, ResultText: "done",
	}}

	got, err := m.mark(f)

	if err != nil {
		t.Fatalf("markDay: %v", err)
	}
	in := f.got
	if in.Source != workoutcompletion.SourceApp {
		t.Errorf("Source = %q, want %q", in.Source, workoutcompletion.SourceApp)
	}
	if in.ClientUserID != m.client || in.TrainerID != m.trainer || in.ProgramID != m.assignment.ProgramID ||
		in.ProgramVersionID != m.assignment.ProgramVersionID || in.ProgramAssignmentID != m.assignment.ID ||
		in.CompletionCycleID != m.assignment.CompletionCycleID || in.DayKey != m.dayKey {
		t.Errorf("input ids = %+v", in)
	}
	if in.WeekNumber != 2 || in.DayNumber != 3 || in.ProgramName != "Strength" || in.ProgramNameRu != "Сила" || in.ResultText != "done" {
		t.Errorf("input = %+v", in)
	}
	if len(in.Day.Blocks) != 1 || in.Day.DayKey != m.dayKey {
		t.Errorf("day = %+v, want the day the client sees", in.Day)
	}
	if got.ID != f.out.ID || got.WeekNumber != 2 || got.DayNumber != 3 || !got.CompletedAt.Equal(completedAt) || got.ResultText != "done" {
		t.Errorf("answer = %+v", got)
	}
}

func TestMarkDay_refusesBeforeWritingWhenTheRequestIsStale(t *testing.T) {
	for name, mutate := range map[string]func(*markFixture){
		"other cycle":      func(m *markFixture) { m.req.CompletionCycleID = uuid.New() },
		"other version":    func(m *markFixture) { m.req.ProgramVersionID = uuid.New() },
		"other assignment": func(m *markFixture) { m.req.ProgramAssignmentID = uuid.New() },
		"no assignment":    func(m *markFixture) { m.assignment = nil },
	} {
		t.Run(name, func(t *testing.T) {
			m := newMarkFixture()
			mutate(&m)
			f := &fakeCompletions{}
			if _, err := m.mark(f); !errors.Is(err, ErrProgramChanged) || f.calls != 0 {
				t.Fatalf("err = %v with %d writes, want ErrProgramChanged and none", err, f.calls)
			}
		})
	}
}

func TestMarkDay_unknownDayIsNotWritten(t *testing.T) {
	m := newMarkFixture()
	m.req.DayKey = uuid.New()
	f := &fakeCompletions{}

	if _, err := m.mark(f); !errors.Is(err, ErrDayNotFound) || f.calls != 0 {
		t.Fatalf("err = %v with %d writes, want ErrDayNotFound and none", err, f.calls)
	}
}

func TestMarkDay_mapsTheWriteErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"the conditional insert found no current assignment", workoutcompletion.ErrAssignmentChanged, ErrProgramChanged},
		{"wrapped", errors.Join(boom, workoutcompletion.ErrAssignmentChanged), ErrProgramChanged},
		{"already completed passes through", workoutcompletion.ErrAlreadyCompleted, workoutcompletion.ErrAlreadyCompleted},
		{"validation passes through", workoutcompletion.ErrValidation, workoutcompletion.ErrValidation},
		{"anything else passes through", boom, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMarkFixture()
			if _, err := m.mark(&fakeCompletions{err: tt.err}); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
