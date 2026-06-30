package program

import (
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func TestParseListParams_defaults(t *testing.T) {
	params, err := ParseListParams("", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}
	if params.Page != DefaultPage || params.Limit != DefaultLimit {
		t.Fatalf("defaults = page %d limit %d", params.Page, params.Limit)
	}
	if params.SortBy != defaultSortBy || params.SortOrder != defaultSortOrder {
		t.Fatalf("sort = %s %s", params.SortBy, params.SortOrder)
	}
}

func TestParseListParams_statusFilter(t *testing.T) {
	params, err := ParseListParams("", "", "", "", "", "draft,archived", "", "")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}
	if len(params.Statuses) != 2 {
		t.Fatalf("statuses len = %d, want 2", len(params.Statuses))
	}
}

func TestParseListParams_categoryAndDifficulty(t *testing.T) {
	params, err := ParseListParams("", "", "", "", "", "", "muscle_gain", "beginner")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}
	if params.Category == nil || *params.Category != CategoryMuscleGain {
		t.Fatalf("category = %v, want muscle_gain", params.Category)
	}
	if params.Difficulty == nil || *params.Difficulty != exercise.DifficultyBeginner {
		t.Fatalf("difficulty = %v, want beginner", params.Difficulty)
	}
}

func TestParseListParams_invalidStatus(t *testing.T) {
	_, err := ParseListParams("", "", "", "", "", "active", "", "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestParseListParams_invalidCategory(t *testing.T) {
	_, err := ParseListParams("", "", "", "", "", "", "invalid", "")
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestValidatePublishDetail(t *testing.T) {
	category := CategoryWeightLoss
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10

	d := Detail{
		Program: Program{Name: "Test", Category: &category, Difficulty: &difficulty},
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
	if err := validatePublishDetail(d); err != nil {
		t.Fatalf("validatePublishDetail() error = %v", err)
	}
}
