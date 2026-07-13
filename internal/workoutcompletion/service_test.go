package workoutcompletion

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

type recordingStore struct {
	exists     bool
	existsErr  error
	keys       map[uuid.UUID]struct{}
	keysErr    error
	insertErr  error
	lastInsert Completion
}

func (f *recordingStore) Exists(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.exists, f.existsErr
}
func (f *recordingStore) ListCompletedDayKeys(context.Context, uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return f.keys, f.keysErr
}
func (f *recordingStore) Insert(_ context.Context, c Completion) (Completion, error) {
	f.lastInsert = c
	if f.insertErr != nil {
		return Completion{}, f.insertErr
	}
	c.ID = uuid.New()
	c.CreatedAt = time.Now().UTC()
	return c, nil
}

func TestService_IsCompletedAndKeys(t *testing.T) {
	cycle, day := uuid.New(), uuid.New()
	st := &recordingStore{exists: true, keys: map[uuid.UUID]struct{}{day: {}}}
	svc := New(st)

	ok, err := svc.IsCompleted(context.Background(), cycle, day)
	if err != nil || !ok {
		t.Fatalf("IsCompleted = %v, %v", ok, err)
	}
	keys, err := svc.CompletedDayKeys(context.Background(), cycle)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := keys[day]; !has {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestService_Complete_success(t *testing.T) {
	st := &recordingStore{}
	svc := New(st)
	cycle, dayKey := uuid.New(), uuid.New()
	in := CompleteInput{
		ClientUserID:        uuid.New(),
		TrainerID:           uuid.New(),
		ProgramID:           uuid.New(),
		ProgramVersionID:    uuid.New(),
		ProgramAssignmentID: uuid.New(),
		CompletionCycleID:   cycle,
		DayKey:              dayKey,
		WeekNumber:          1,
		DayNumber:           3,
		ProgramName:         "Force",
		ProgramNameRu:       "Сила",
		Day: program.Day{
			Blocks: []program.DayBlock{{
				BlockType: program.BlockTypeSingle,
				Exercises: []program.DayExercise{{ExerciseName: "Squat", ExerciseNameRu: "Присед"}},
			}},
		},
		ResultText: "  all good  ",
	}
	got, err := svc.Complete(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultText != "all good" || got.Source != SourceTelegram {
		t.Fatalf("got %+v", got)
	}
	if st.lastInsert.WeekNumber != 1 || st.lastInsert.DayNumber != 3 {
		t.Fatalf("inserted %+v", st.lastInsert)
	}
	if len(st.lastInsert.DaySnapshot) == 0 {
		t.Fatal("expected snapshot")
	}
}

func TestService_Complete_validationCases(t *testing.T) {
	svc := New(&recordingStore{})
	cycle, day := uuid.New(), uuid.New()
	base := CompleteInput{
		CompletionCycleID: cycle,
		DayKey:            day,
		ResultText:        "ok",
	}

	cases := []struct {
		name string
		in   CompleteInput
		want string
	}{
		{"empty", CompleteInput{CompletionCycleID: cycle, DayKey: day, ResultText: " \t"}, "result_text required"},
		{"too long", CompleteInput{CompletionCycleID: cycle, DayKey: day, ResultText: strings.Repeat("я", MaxResultTextLen+1)}, "too long"},
		{"bad source", CompleteInput{CompletionCycleID: cycle, DayKey: day, ResultText: "ok", Source: "web"}, "invalid source"},
		{"nil cycle", CompleteInput{DayKey: day, ResultText: "ok"}, "cycle and day_key"},
		{"nil day", CompleteInput{CompletionCycleID: cycle, ResultText: "ok"}, "cycle and day_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Complete(context.Background(), tc.in)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contain %q", err, tc.want)
			}
		})
	}

	// max length allowed
	base.ResultText = strings.Repeat("a", MaxResultTextLen)
	if utf8.RuneCountInString(base.ResultText) != MaxResultTextLen {
		t.Fatal("setup")
	}
	if _, err := svc.Complete(context.Background(), base); err != nil {
		t.Fatal(err)
	}
}

func TestService_Complete_insertAlreadyCompleted(t *testing.T) {
	svc := New(&recordingStore{insertErr: ErrAlreadyCompleted})
	_, err := svc.Complete(context.Background(), CompleteInput{
		CompletionCycleID: uuid.New(),
		DayKey:            uuid.New(),
		ResultText:        "done",
	})
	if !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("err = %v", err)
	}
}

func TestOptionalPGUUID(t *testing.T) {
	if optionalPGUUID(nil).Valid {
		t.Fatal("nil should be invalid")
	}
	id := uuid.New()
	pg := optionalPGUUID(&id)
	if !pg.Valid || pgconv.FromPGUUID(pg) != id {
		t.Fatalf("pg = %+v", pg)
	}
}

func TestCompletionFromRow(t *testing.T) {
	id := uuid.New()
	client := uuid.New()
	trainer := uuid.New()
	cycle := uuid.New()
	day := uuid.New()
	programID := uuid.New()
	versionID := uuid.New()
	assignmentID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	row := sqlc.MentorixClientWorkoutCompletion{
		ID:                  pgconv.ToPGUUID(id),
		ClientUserID:        pgconv.ToPGUUID(client),
		TrainerID:           pgconv.ToPGUUID(trainer),
		CompletedAt:         now,
		ProgramID:           pgconv.ToPGUUID(programID),
		ProgramVersionID:    pgconv.ToPGUUID(versionID),
		ProgramAssignmentID: pgconv.ToPGUUID(assignmentID),
		CompletionCycleID:   pgconv.ToPGUUID(cycle),
		DayKey:              pgconv.ToPGUUID(day),
		WeekNumber:          2,
		DayNumber:           4,
		ProgramName:         "P",
		ProgramNameRu:       "П",
		DaySnapshot:         []byte(`{"blocks":[]}`),
		ResultText:          "ok",
		Source:              SourceTelegram,
		CreatedAt:           now,
	}
	got := completionFromRow(row)
	if got.ID != id || got.ClientUserID != client || got.TrainerID != trainer {
		t.Fatalf("ids %+v", got)
	}
	if got.ProgramID == nil || *got.ProgramID != programID {
		t.Fatalf("program %v", got.ProgramID)
	}
	if got.ProgramVersionID == nil || *got.ProgramVersionID != versionID {
		t.Fatalf("version %v", got.ProgramVersionID)
	}
	if got.ProgramAssignmentID == nil || *got.ProgramAssignmentID != assignmentID {
		t.Fatalf("assignment %v", got.ProgramAssignmentID)
	}
	if got.WeekNumber != 2 || got.DayNumber != 4 || got.ResultText != "ok" {
		t.Fatalf("fields %+v", got)
	}

	row.ProgramID = pgtype.UUID{}
	row.ProgramVersionID = pgtype.UUID{}
	row.ProgramAssignmentID = pgtype.UUID{}
	got = completionFromRow(row)
	if got.ProgramID != nil || got.ProgramVersionID != nil || got.ProgramAssignmentID != nil {
		t.Fatalf("want nil FKs, got %+v", got)
	}
}
