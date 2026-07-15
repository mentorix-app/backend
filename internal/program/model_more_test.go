package program

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func TestValidatePublishDetail_noWeeks(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks:   nil,
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for no weeks")
	}
}

func TestValidatePublishDetail_invalidSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	bad := "3/"
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &bad,
				Reps:       &bad,
			})},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for invalid sets/reps")
	}
}

func TestValidatePublishDetail_optionalSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
			})},
		}},
	}
	if err := validatePublishDetail(d); err != nil {
		t.Fatalf("validatePublishDetail() error = %v, want nil", err)
	}
}

func TestValidatePublishDetail_rangeSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets, reps := "3-6", "5/4"
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			})},
		}},
	}
	if err := validatePublishDetail(d); err != nil {
		t.Fatalf("validatePublishDetail() error = %v, want nil", err)
	}
}

func TestDayExerciseInput_ValidatePublish(t *testing.T) {
	sets, reps := "3", "10"
	if err := (DayExerciseInput{
		ExerciseID: uuid.New(),
		Sets:       &sets,
		Reps:       &reps,
	}).ValidatePublish(); err != nil {
		t.Fatalf("ValidatePublish() error = %v", err)
	}
}

func TestDayExerciseInput_ValidatePublish_optionalSetsReps(t *testing.T) {
	reps := "10"
	if err := (DayExerciseInput{
		ExerciseID: uuid.New(),
		Reps:       &reps,
	}).ValidatePublish(); err != nil {
		t.Fatalf("ValidatePublish() error = %v", err)
	}

	sets := "3"
	if err := (DayExerciseInput{
		ExerciseID: uuid.New(),
		Sets:       &sets,
	}).ValidatePublish(); err != nil {
		t.Fatalf("ValidatePublish() error = %v", err)
	}

	if err := (DayExerciseInput{
		ExerciseID: uuid.New(),
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
	tests := []struct {
		name string
		in   DayExerciseInput
	}{
		{
			name: "missing exercise id",
			in:   DayExerciseInput{},
		},
		{
			name: "trailing slash sets",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				Sets:       strPtr("3/"),
			},
		},
		{
			name: "letters in reps",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				Reps:       strPtr("abc"),
			},
		},
		{
			name: "double hyphen sets",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				Sets:       strPtr("3-6-8"),
			},
		},
		{
			name: "mixed separators reps",
			in: DayExerciseInput{
				ExerciseID: uuid.New(),
				Reps:       strPtr("5/4-2"),
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

func TestDayExerciseInput_ValidateDraft_volumeFormats(t *testing.T) {
	tests := []string{"3", "5/4", "3-6", "0", "10/10", "1-12"}
	for _, v := range tests {
		t.Run(v, func(t *testing.T) {
			vol := v
			if err := (DayExerciseInput{ExerciseID: uuid.New(), Sets: &vol, Reps: &vol}).ValidateDraft(); err != nil {
				t.Fatalf("ValidateDraft() error = %v", err)
			}
		})
	}
}

func TestDayExerciseInput_ValidateDraft_optionalSetsReps(t *testing.T) {
	tests := []DayExerciseInput{
		{ExerciseID: uuid.New()},
	}
	for i, in := range tests {
		if err := in.ValidateDraft(); err != nil {
			t.Fatalf("case %d: ValidateDraft() error = %v", i, err)
		}
	}
}

func TestValidatePublishDetail_groupBlockOptionalSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{{
				DayNumber: 1,
				SortOrder: 1,
				Blocks: []DayBlock{{
					BlockType: BlockTypeEMOM,
					Exercises: []DayExercise{{
						ExerciseID: uuid.New(),
					}},
				}},
			}},
		}},
	}
	if err := validatePublishDetail(d); err != nil {
		t.Fatalf("validatePublishDetail() error = %v, want nil", err)
	}
}

func TestValidatePublishDetail_groupBlockInvalidSetsReps(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	bad := "5/"
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{{
				DayNumber: 1,
				SortOrder: 1,
				Blocks: []DayBlock{{
					BlockType: BlockTypeEMOM,
					Exercises: []DayExercise{{
						ExerciseID: uuid.New(),
						Sets:       &bad,
					}},
				}},
			}},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for invalid sets in group block")
	}
}

func TestValidatePublishDetail_skipsDaysWithoutExercises(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets, reps := "3", "10"
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{
				{DayNumber: 1, SortOrder: 1},
				{
					DayNumber: 2,
					SortOrder: 2,
					Blocks: []DayBlock{{
						BlockType: BlockTypeSingle,
						Exercises: []DayExercise{{
							ExerciseID: uuid.New(),
							Sets:       &sets,
							Reps:       &reps,
						}},
					}},
				},
			},
		}},
	}
	if err := validatePublishDetail(d); err != nil {
		t.Fatalf("validatePublishDetail() error = %v, want nil", err)
	}
}

func TestValidatePublishDetail_groupBlockWithoutExercises(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	d := Detail{
		Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{{
				DayNumber: 1,
				SortOrder: 1,
				Blocks: []DayBlock{{
					BlockType: BlockTypeEMOM,
				}},
			}},
		}},
	}
	if err := validatePublishDetail(d); !errors.Is(err, ErrValidation) {
		t.Fatalf("validatePublishDetail() error = %v, want ErrValidation", err)
	}
}

func TestValidatePublishDetail_missingDifficulty(t *testing.T) {
	category := CategoryMuscleGain
	sets, reps := "3", "10"
	d := Detail{
		Program: Program{Name: "Plan", Category: &category},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			})},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("expected validation error for missing difficulty")
	}
}

func TestValidatePublishDetail_missingMetadata(t *testing.T) {
	sets, reps := "3", "10"
	base := Detail{
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			})},
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
				Weeks:   base.Weeks,
			},
		},
		{
			name: "missing category",
			d: Detail{
				Program: Program{Name: "Plan", Difficulty: &difficulty},
				Weeks:   base.Weeks,
			},
		},
		{
			name: "week without training day",
			d: Detail{
				Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
				Weeks:   []Week{{WeekNumber: 1, Days: []Day{{DayNumber: 1}}}},
			},
		},
		{
			name: "second week without exercises",
			d: Detail{
				Program: Program{Name: "Plan", Category: &category, Difficulty: &difficulty},
				Weeks: []Week{
					base.Weeks[0],
					{WeekNumber: 2, Days: []Day{{DayNumber: 1}}},
				},
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

func strPtr(v string) *string {
	return &v
}
