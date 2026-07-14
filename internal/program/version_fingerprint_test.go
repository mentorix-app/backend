package program

import (
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func TestDetailFingerprint_stableForSameContent(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets, reps := "3", "10"
	exerciseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	d := Detail{
		Program: Program{
			Name:       "Program",
			Category:   &category,
			Difficulty: (*Difficulty)(&difficulty),
		},
		Weeks: []Week{{
			WeekNumber: 1,
			SortOrder:  1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: exerciseID,
				SortOrder:  1,
				Sets:       &sets,
				Reps:       &reps,
			})},
		}},
	}

	fp1, err := DetailFingerprint(d)
	if err != nil {
		t.Fatalf("DetailFingerprint: %v", err)
	}
	fp2, err := DetailFingerprint(d)
	if err != nil {
		t.Fatalf("DetailFingerprint: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprints differ for identical detail")
	}
}

func TestDetailFingerprint_changesWhenNameChanges(t *testing.T) {
	d1 := Detail{Program: Program{Name: "A"}}
	d2 := Detail{Program: Program{Name: "B"}}
	fp1, err := DetailFingerprint(d1)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := DetailFingerprint(d2)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatal("expected different fingerprints")
	}
}
