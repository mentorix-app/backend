package program

import (
	"testing"
	"time"
)

func TestFlattenDays(t *testing.T) {
	weeks := []Week{{
		WeekNumber: 1,
		Days: []Day{
			{DayNumber: 1},
			{DayNumber: 2},
		},
	}}
	flat := FlattenDays(weeks)
	if len(flat) != 2 {
		t.Fatalf("len = %d", len(flat))
	}
}

func TestDayOffsetFromAssignment(t *testing.T) {
	assigned := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 3, 8, 0, 0, 0, time.UTC)
	if got := DayOffsetFromAssignment(assigned, now); got != 2 {
		t.Fatalf("offset = %d", got)
	}
}

func TestTodayFlatDay(t *testing.T) {
	assigned := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	weeks := []Week{{
		WeekNumber: 1,
		Days:       []Day{{DayNumber: 1, Blocks: []DayBlock{{}}}},
	}}
	flat, n, ok := TodayFlatDay(assigned, now, weeks)
	if !ok || n != 1 || flat.DayNumber != 1 {
		t.Fatalf("flat=%+v n=%d ok=%v", flat, n, ok)
	}
}

func TestIsRestDay(t *testing.T) {
	if !IsRestDay(Day{}) {
		t.Fatal("empty day should be rest")
	}
	if IsRestDay(Day{Blocks: []DayBlock{{Exercises: []DayExercise{{ExerciseName: "sq"}}}}}) {
		t.Fatal("day with exercise should not be rest")
	}
}

func TestIsTrainingDay(t *testing.T) {
	if IsTrainingDay(Day{}) {
		t.Fatal("empty day should not count")
	}
	if !IsTrainingDay(Day{Blocks: []DayBlock{{Exercises: []DayExercise{{}}}}}) {
		t.Fatal("day with exercise should count")
	}
	if !IsTrainingDay(Day{Blocks: []DayBlock{{BlockType: BlockTypeComplex}}}) {
		t.Fatal("day with block should count")
	}
}

func TestCountTrainingDays(t *testing.T) {
	weeks := []Week{{
		Days: []Day{
			{},
			{Blocks: []DayBlock{{Exercises: []DayExercise{{}}}}},
			{Blocks: []DayBlock{{}}},
		},
	}}
	if got := CountTrainingDays(weeks); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestFlattenDays_empty(t *testing.T) {
	if len(FlattenDays(nil)) != 0 {
		t.Fatal("expected empty")
	}
}

func TestTodayFlatDay_clampsToLastDay(t *testing.T) {
	assigned := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	weeks := []Week{{
		WeekNumber: 1,
		Days:       []Day{{DayNumber: 1}, {DayNumber: 2}},
	}}
	flat, n, ok := TodayFlatDay(assigned, now, weeks)
	if !ok || n != 2 || flat.DayNumber != 2 {
		t.Fatalf("flat=%+v n=%d ok=%v", flat, n, ok)
	}
}

func TestTodayFlatDay_emptyWeeks(t *testing.T) {
	_, _, ok := TodayFlatDay(time.Now(), time.Now(), nil)
	if ok {
		t.Fatal("expected false")
	}
}
