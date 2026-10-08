package exercise

import (
	"strings"
	"testing"
)

func validUpsertInput() UpsertInput {
	return UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	}
}

func TestUpsertInput_Validate_success(t *testing.T) {
	if err := validUpsertInput().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestUpsertInput_Validate_errors(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*UpsertInput)
	}{
		{"empty name", func(in *UpsertInput) { in.Name = " " }},
		{"invalid muscle group", func(in *UpsertInput) { in.MuscleGroup = "wings" }},
		{"invalid type", func(in *UpsertInput) { in.Type = "yoga" }},
		{"invalid equipment", func(in *UpsertInput) {
			eq := Equipment("hoverboard")
			in.Equipment = &eq
		}},
		{"invalid difficulty", func(in *UpsertInput) { in.Difficulty = "godmode" }},
		{"invalid video url", func(in *UpsertInput) {
			in.VideoURL = "https://vimeo.com/123456789"
		}},
		{"name over limit", func(in *UpsertInput) { in.Name = strings.Repeat("a", 1001) }},
		{"name_ru over limit in runes", func(in *UpsertInput) { in.NameRu = strings.Repeat("я", 1001) }},
		{"description over limit", func(in *UpsertInput) { in.Description = strings.Repeat("a", 5001) }},
		{"description_ru over limit in runes", func(in *UpsertInput) { in.DescriptionRu = strings.Repeat("я", 5001) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validUpsertInput()
			tt.mut(&in)
			if err := in.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpsertInput_Validate_textAtLimits(t *testing.T) {
	in := validUpsertInput()
	in.Name = strings.Repeat("a", 1000)
	in.NameRu = strings.Repeat("я", 1000)
	in.Description = strings.Repeat("a", 5000)
	in.DescriptionRu = strings.Repeat("я", 5000)
	if err := in.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEquipment_valid(t *testing.T) {
	if !(EquipmentBarbell).valid() {
		t.Fatal("expected barbell valid")
	}
	if (Equipment("nope")).valid() {
		t.Fatal("expected invalid equipment")
	}
}
