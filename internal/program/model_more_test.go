package program

import (
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func TestValidatePublishDetail_noDays(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Days:    nil,
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for no days")
	}
}

func TestValidatePublishDetail_invalidSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	zero := 0
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Days: []Day{{
			Exercises: []DayExercise{{
				ExerciseID: uuid.New(),
				Sets:       &zero,
				Reps:       &zero,
			}},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for zero sets/reps")
	}
}

func TestDayExerciseInput_ValidatePublish(t *testing.T) {
	sets, reps := 3, 10
	if err := (DayExerciseInput{
		ExerciseID: uuid.New(),
		Sets:       &sets,
		Reps:       &reps,
	}).ValidatePublish(); err != nil {
		t.Fatalf("ValidatePublish() error = %v", err)
	}
}

func TestUpdateInput_Validate_invalidCategory(t *testing.T) {
	bad := Category("invalid")
	if err := (UpdateInput{Category: &bad}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestUpdateInput_Validate_invalidDifficulty(t *testing.T) {
	bad := Difficulty("invalid")
	if err := (UpdateInput{Difficulty: &bad}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDayExerciseInput_ValidateDraft_errors(t *testing.T) {
	neg := -1.0
	tests := []struct {
		name string
		in   DayExerciseInput
	}{
		{
			name: "missing exercise id",
			in:   DayExerciseInput{},
		},
		{
			name: "zero sets",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				Sets:       intPtr(0),
			},
		},
		{
			name: "negative weight",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				WeightKg:   &neg,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.ValidateDraft(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidatePublishDetail_missingMetadata(t *testing.T) {
	sets, reps := 3, 10
	base := Detail{
		Days: []Day{{
			Exercises: []DayExercise{{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			}},
		}},
	}
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner

	tests := []struct {
		name string
		d    Detail
	}{
		{
			name: "empty name",
			d: Detail{
				Program: Program{Category: &category, Difficulty: &difficulty},
				Days:    base.Days,
			},
		},
		{
			name: "missing category",
			d: Detail{
				Program: Program{Name: "Plan", Difficulty: &difficulty},
				Days:    base.Days,
			},
		},
		{
			name: "empty day exercises",
			d: Detail{
				Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
				Days:    []Day{{DayNumber: 1}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePublishDetail(tt.d); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestStatusAndCategory_valid(t *testing.T) {
	if !StatusDraft.valid() || !StatusPublished.valid() || Status("bad").valid() {
		t.Fatal("unexpected status validity")
	}
	if !CategoryMuscleGain.valid() || Category("bad").valid() {
		t.Fatal("unexpected category validity")
	}
	if !difficultyValid(exercise.DifficultyBeginner) || difficultyValid("bad") {
		t.Fatal("unexpected difficulty validity")
	}
}

func intPtr(v int) *int {
	return &v
}
