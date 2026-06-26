package exercise

import "testing"

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

func TestEquipment_valid(t *testing.T) {
	if !(EquipmentBarbell).valid() {
		t.Fatal("expected barbell valid")
	}
	if (Equipment("nope")).valid() {
		t.Fatal("expected invalid equipment")
	}
}
