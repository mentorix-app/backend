package exercise

import (
	"errors"
	"testing"
)

func TestParseListParams_defaults(t *testing.T) {
	params, err := ParseListParams("", "", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Page != DefaultPage {
		t.Errorf("Page = %d, want %d", params.Page, DefaultPage)
	}
	if params.Limit != DefaultLimit {
		t.Errorf("Limit = %d, want %d", params.Limit, DefaultLimit)
	}
	if params.SortBy != "name" {
		t.Errorf("SortBy = %q, want name", params.SortBy)
	}
	if params.SortOrder != "asc" {
		t.Errorf("SortOrder = %q, want asc", params.SortOrder)
	}
}

func TestParseListParams_pagination(t *testing.T) {
	params, err := ParseListParams("2", "50", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Page != 2 || params.Limit != 50 {
		t.Errorf("got page=%d limit=%d, want 2/50", params.Page, params.Limit)
	}
	if params.Offset() != 50 {
		t.Errorf("Offset() = %d, want 50", params.Offset())
	}
}

func TestParseListParams_invalidPagination(t *testing.T) {
	tests := []struct {
		name  string
		page  string
		limit string
	}{
		{"zero page", "0", ""},
		{"negative page", "-1", ""},
		{"bad page", "abc", ""},
		{"zero limit", "", "0"},
		{"over max limit", "", "101"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseListParams(tt.page, tt.limit, "", "", "", "", "", "", "")
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestParseListParams_sort(t *testing.T) {
	params, err := ParseListParams("", "", "created_at", "desc", "", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.SortBy != "created_at" || params.SortOrder != "desc" {
		t.Errorf("got sort_by=%q sort_order=%q", params.SortBy, params.SortOrder)
	}

	params, err = ParseListParams("", "", "equipment", "asc", "", "", "", "", "")
	if err != nil {
		t.Fatalf("equipment sort: unexpected error: %v", err)
	}
	if params.SortBy != "equipment" || params.SortOrder != "asc" {
		t.Errorf("equipment sort: got sort_by=%q sort_order=%q", params.SortBy, params.SortOrder)
	}

	params, err = ParseListParams("", "", "equipment", "desc", "", "", "", "", "")
	if err != nil {
		t.Fatalf("equipment desc sort: unexpected error: %v", err)
	}
	if params.SortBy != "equipment" || params.SortOrder != "desc" {
		t.Errorf("equipment desc sort: got sort_by=%q sort_order=%q", params.SortBy, params.SortOrder)
	}

	_, err = ParseListParams("", "", "bad_col", "", "", "", "", "", "")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid sort_by: expected ErrValidation, got %v", err)
	}

	_, err = ParseListParams("", "", "", "sideways", "", "", "", "", "")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid sort_order: expected ErrValidation, got %v", err)
	}
}

func TestParseListParams_filters(t *testing.T) {
	params, err := ParseListParams("", "", "", "", " squat ",
		string(ExerciseTypeStrength),
		string(MuscleGroupLegs),
		string(DifficultyBeginner),
		string(EquipmentBarbell),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Query != "squat" {
		t.Errorf("Query = %q, want squat", params.Query)
	}
	if params.Type == nil || *params.Type != ExerciseTypeStrength {
		t.Errorf("unexpected type filter: %v", params.Type)
	}
	if params.MuscleGroup == nil || *params.MuscleGroup != MuscleGroupLegs {
		t.Errorf("unexpected muscle_group filter: %v", params.MuscleGroup)
	}
	if params.Difficulty == nil || *params.Difficulty != DifficultyBeginner {
		t.Errorf("unexpected difficulty filter: %v", params.Difficulty)
	}
	if params.Equipment == nil || *params.Equipment != EquipmentBarbell {
		t.Errorf("unexpected equipment filter: %v", params.Equipment)
	}
}

func TestParseListParams_equipmentNone(t *testing.T) {
	params, err := ParseListParams("", "", "", "", "", "", "", "", "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !params.EquipmentIsNull {
		t.Error("expected EquipmentIsNull")
	}
	if params.Equipment != nil {
		t.Error("expected Equipment to be nil")
	}
}

func TestParseListParams_invalidFilters(t *testing.T) {
	tests := []struct {
		name      string
		typeStr   string
		muscle    string
		diff      string
		equipment string
	}{
		{"invalid type", "powerlifting", "", "", ""},
		{"invalid muscle_group", "", "biceps", "", ""},
		{"invalid difficulty", "", "", "impossible", ""},
		{"invalid equipment", "", "", "", "treadmill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseListParams("", "", "", "", "", tt.typeStr, tt.muscle, tt.diff, tt.equipment)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`100%_done\`)
	want := `100\%\_done\\`
	if got != want {
		t.Errorf("escapeLike = %q, want %q", got, want)
	}
}

func TestPaginationMeta(t *testing.T) {
	meta := paginationMeta(2, 20, 45)
	if meta.Page != 2 || meta.Limit != 20 || meta.Total != 45 || meta.TotalPages != 3 {
		t.Errorf("unexpected meta: %+v", meta)
	}
	meta = paginationMeta(1, 20, 0)
	if meta.TotalPages != 0 {
		t.Errorf("empty total should give 0 pages, got %d", meta.TotalPages)
	}
}
