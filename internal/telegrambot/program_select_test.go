package telegrambot

import (
	"testing"

	"mentorix-backend/internal/program"
)

func TestDayHasExercises(t *testing.T) {
	if dayHasExercises(program.Day{}) {
		t.Fatal("empty day should not have exercises")
	}
	if dayHasExercises(program.Day{Blocks: []program.DayBlock{{BlockType: program.BlockTypeComplex}}}) {
		t.Fatal("block without exercises should not count")
	}
	if !dayHasExercises(program.Day{Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{}}}}}) {
		t.Fatal("day with exercise should count")
	}
}

func TestWeekHasSelectableDays(t *testing.T) {
	week := program.Week{
		Days: []program.Day{
			{},
			{Blocks: []program.DayBlock{{BlockType: program.BlockTypeComplex}}},
			{Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{}}}}},
		},
	}
	if !weekHasSelectableDays(week) {
		t.Fatal("week with one exercise day should be selectable")
	}
	if weekHasSelectableDays(program.Week{Days: []program.Day{{}, {Blocks: []program.DayBlock{{}}}}}) {
		t.Fatal("week with only empty blocks should not be selectable")
	}
}

func TestCountSelectableWeeksAndDays(t *testing.T) {
	sets, reps := 3, 10
	weeks := []program.Week{
		{WeekNumber: 1, Days: []program.Day{{}, {Blocks: []program.DayBlock{{}}}}},
		{WeekNumber: 2, Days: []program.Day{
			{DayNumber: 1, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}}},
			{DayNumber: 2, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}}},
		}},
	}
	if got := countSelectableWeeks(weeks); got != 1 {
		t.Fatalf("weeks = %d, want 1", got)
	}
	if got := countSelectableDays(weeks); got != 2 {
		t.Fatalf("days = %d, want 2", got)
	}
}

func TestFindWeekAndDay(t *testing.T) {
	weeks := []program.Week{{
		WeekNumber: 2,
		Days:       []program.Day{{DayNumber: 3}},
	}}
	week, ok := findWeek(weeks, 2)
	if !ok || week.WeekNumber != 2 {
		t.Fatalf("week = %+v ok=%v", week, ok)
	}
	day, ok := findDay(week.Days, 3)
	if !ok || day.DayNumber != 3 {
		t.Fatalf("day = %+v ok=%v", day, ok)
	}
	if _, ok := findWeek(weeks, 9); ok {
		t.Fatal("expected missing week")
	}
}

func TestNextSelectableDayInWeek(t *testing.T) {
	sets, reps := 3, 10
	ex := program.DayBlock{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}
	week := program.Week{
		Days: []program.Day{
			{DayNumber: 1, Blocks: []program.DayBlock{ex}},
			{DayNumber: 2},
			{DayNumber: 3, Blocks: []program.DayBlock{ex}},
			{DayNumber: 4, Blocks: []program.DayBlock{ex}},
		},
	}
	next, ok := nextSelectableDayInWeek(week, 1)
	if !ok || next != 3 {
		t.Fatalf("next = %d ok=%v, want day 3", next, ok)
	}
	next, ok = nextSelectableDayInWeek(week, 3)
	if !ok || next != 4 {
		t.Fatalf("next = %d ok=%v, want day 4", next, ok)
	}
	if _, ok := nextSelectableDayInWeek(week, 4); ok {
		t.Fatal("expected no next day after last training day")
	}
}

func TestNextSelectableWeek(t *testing.T) {
	sets, reps := 3, 10
	ex := []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}}
	weeks := []program.Week{
		{WeekNumber: 1, Days: []program.Day{{DayNumber: 1, Blocks: ex}}},
		{WeekNumber: 2, Days: []program.Day{{DayNumber: 1}}},
		{WeekNumber: 3, Days: []program.Day{{DayNumber: 1, Blocks: ex}}},
	}
	next, ok := nextSelectableWeek(weeks, 1)
	if !ok || next != 3 {
		t.Fatalf("next week = %d ok=%v, want 3", next, ok)
	}
	if _, ok := nextSelectableWeek(weeks, 3); ok {
		t.Fatal("expected no next week")
	}
}
