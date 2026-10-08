package program

import "testing"

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
