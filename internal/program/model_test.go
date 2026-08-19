package program

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func singleBlockDay(ex DayExercise) Day {
	return Day{
		Blocks: []DayBlock{{
			BlockType: BlockTypeSingle,
			Exercises: []DayExercise{ex},
		}},
	}
}

func validPublishDetail(name string) Detail {
	category := CategoryWeightLoss
	difficulty := exercise.DifficultyBeginner
	sets := "3"
	reps := "10"
	return Detail{
		Program: Program{Name: name, Category: &category, Difficulty: &difficulty},
		Weeks: []Week{{
			WeekNumber: 1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			})},
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
	sets := "3"
	reps := "10"
	d := Detail{
		Program: Program{Name: "Test", Difficulty: &difficulty},
		Weeks: []Week{{
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			})},
		}},
	}
	if err := validatePublishDetail(d); err == nil {
		t.Fatal("validatePublishDetail() error = nil, want validation error for missing category")
	}
}

func TestValidatePublishDetail_dayWithoutSharedBlock(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	client := uuid.New()

	d := Detail{
		Program: Program{
			Name:       "Program",
			Category:   &category,
			Difficulty: (*Difficulty)(&difficulty),
		},
		Weeks: []Week{{
			WeekNumber: 1,
			SortOrder:  1,
			Days: []Day{{
				DayNumber: 1,
				Blocks: []DayBlock{{
					BlockType:     BlockTypeSingle,
					BlockKey:      uuid.New(),
					ClientUserIDs: []uuid.UUID{client},
					Exercises:     []DayExercise{{ExerciseID: uuid.New(), SortOrder: 1}},
				}},
			}},
		}},
	}

	err := validatePublishDetail(d)
	if !errors.Is(err, ErrLastSharedBlock) {
		t.Fatalf("validatePublishDetail() error = %v, want ErrLastSharedBlock", err)
	}
}

func TestBlockPatchInput_Validate_rejectsSingle(t *testing.T) {
	single := BlockTypeSingle
	if err := (BlockPatchInput{BlockType: &single}).Validate(); err == nil {
		t.Fatal("expected validation error for single block_type in patch")
	}
	emom := BlockTypeEMOM
	if err := (BlockPatchInput{BlockType: &emom}).Validate(); err != nil {
		t.Fatalf("BlockPatchInput.Validate() error = %v, want nil for group type", err)
	}
}
