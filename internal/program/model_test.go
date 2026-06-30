package program

import (
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func validPublishDetail(name string) Detail {
	category := CategoryWeightLoss
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	return Detail{
		Program: Program{Name: name, Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{{
				DayNumber: 1,
				Exercises: []DayExercise{{
					ExerciseID: uuid.New(),
					Sets:       &sets,
					Reps:       &reps,
				}},
			}},
		}},
	}
}

func TestUpdateInput_allowsEmptyName(t *testing.T) {
	empty := ""
	in := UpdateInput{Name: &empty}
	if err := in.Validate(); err != nil {
		t.Fatalf("UpdateInput.Validate() error = %v, want nil for empty name in draft", err)
	}
}

func TestValidatePublishDetail_emptyName(t *testing.T) {
	d := validPublishDetail("")
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("validatePublishDetail() error = nil, want validation error for empty name")
	}
}

func TestValidatePublishDetail_whitespaceName(t *testing.T) {
	d := validPublishDetail("   ")
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("validatePublishDetail() error = nil, want validation error for whitespace name")
	}
}

func TestValidatePublishDetail_missingCategory(t *testing.T) {
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	d := Detail{
		Program: Program{Name: "Test", Difficulty: &difficulty},
		Weeks: []Week{{
			Days: []Day{{
				Exercises: []DayExercise{{
					ExerciseID: uuid.New(),
					Sets:       &sets,
					Reps:       &reps,
				}},
			}},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("validatePublishDetail() error = nil, want validation error for missing category")
	}
}
