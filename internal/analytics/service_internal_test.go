package analytics

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func TestCompletionPercent(t *testing.T) {
	tests := []struct {
		completed, total int
		want             float64
	}{
		{0, 0, 0},
		{5, 0, 0},
		{0, 10, 0},
		{7, 10, 70},
		{1, 3, 33.3},
		{2, 3, 66.7},
		{15, 10, 100}, // clamped: journal keeps days removed from the version
	}
	for _, tt := range tests {
		if got := completionPercent(tt.completed, tt.total); got != tt.want {
			t.Errorf("completionPercent(%d, %d) = %v, want %v", tt.completed, tt.total, got, tt.want)
		}
	}
}

func TestBuildProgress(t *testing.T) {
	rows := []sqlc.ListCycleWeekProgressRow{
		{WeekNumber: 1, TotalDays: 3, CompletedDays: 3},
		{WeekNumber: 2, TotalDays: 3, CompletedDays: 1},
		{WeekNumber: 3, TotalDays: 2, CompletedDays: 0},
	}
	p := buildProgress(rows)
	if p.CompletedDays != 4 || p.TotalTrainingDays != 8 {
		t.Fatalf("progress = %+v", p)
	}
	if p.CompletionPercent != 50 {
		t.Fatalf("percent = %v", p.CompletionPercent)
	}
	if len(p.Weeks) != 3 || p.Weeks[1].CompletedDays != 1 {
		t.Fatalf("weeks = %+v", p.Weeks)
	}

	empty := buildProgress(nil)
	if empty.CompletionPercent != 0 || len(empty.Weeks) != 0 {
		t.Fatalf("empty = %+v", empty)
	}
}

func TestMapProgramActivity(t *testing.T) {
	programID := uuid.New()
	first := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	last := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	rows := []sqlc.ListClientProgramActivityRow{
		{
			ProgramID:        pgconv.ToPGUUID(programID),
			ProgramName:      "Strength",
			ProgramNameRu:    "Сила",
			TotalCompletions: 12,
			FirstCompletedAt: first,
			LastCompletedAt:  last,
		},
		{
			ProgramID:        pgtype.UUID{}, // deleted program
			ProgramName:      "Old",
			ProgramNameRu:    "Старая",
			TotalCompletions: 3,
			FirstCompletedAt: first,
			LastCompletedAt:  first,
		},
	}
	out := mapProgramActivity(rows)
	if len(out) != 2 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].ProgramID == nil || *out[0].ProgramID != programID {
		t.Fatalf("program_id = %v", out[0].ProgramID)
	}
	if !out[0].LastCompletedAt.Equal(last) || out[0].TotalCompletions != 12 {
		t.Fatalf("row = %+v", out[0])
	}
	if out[1].ProgramID != nil {
		t.Fatalf("deleted program should have nil id, got %v", out[1].ProgramID)
	}
}

func TestTimePtrFromAny(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if got := timePtrFromAny(now); got == nil || !got.Equal(now) {
		t.Fatalf("time.Time: %v", got)
	}
	if got := timePtrFromAny(time.Time{}); got != nil {
		t.Fatalf("zero time: %v", got)
	}
	if got := timePtrFromAny(&now); got == nil || !got.Equal(now) {
		t.Fatalf("*time.Time: %v", got)
	}
	if got := timePtrFromAny((*time.Time)(nil)); got != nil {
		t.Fatalf("nil ptr: %v", got)
	}
	if got := timePtrFromAny(pgtype.Timestamptz{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("timestamptz: %v", got)
	}
	if got := timePtrFromAny(pgtype.Timestamptz{}); got != nil {
		t.Fatalf("invalid timestamptz: %v", got)
	}
	if got := timePtrFromAny(nil); got != nil {
		t.Fatalf("nil: %v", got)
	}
}

func TestUUIDPtrFromPG(t *testing.T) {
	id := uuid.New()
	if got := uuidPtrFromPG(pgconv.ToPGUUID(id)); got == nil || *got != id {
		t.Fatalf("valid: %v", got)
	}
	if got := uuidPtrFromPG(pgtype.UUID{}); got != nil {
		t.Fatalf("invalid: %v", got)
	}
}

func TestToPGTimestamptz(t *testing.T) {
	if got := toPGTimestamptz(nil); got.Valid {
		t.Fatalf("nil should be invalid: %+v", got)
	}
	now := time.Now()
	got := toPGTimestamptz(&now)
	if !got.Valid || !got.Time.Equal(now.UTC()) {
		t.Fatalf("got %+v", got)
	}
}

func TestRoundPercent(t *testing.T) {
	if got := roundPercent(33.333); got != 33.3 {
		t.Fatalf("got %v", got)
	}
	if got := roundPercent(66.666); got != 66.7 {
		t.Fatalf("got %v", got)
	}
}
