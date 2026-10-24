package program

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestOptionalInt_unmarshal(t *testing.T) {
	type body struct {
		Rounds OptionalInt `json:"rounds"`
	}
	tests := []struct {
		name        string
		json        string
		wantPresent bool
		wantValue   *int
		wantErr     bool
	}{
		{"absent key", `{}`, false, nil, false},
		{"null clears", `{"rounds":null}`, true, nil, false},
		{"integer sets", `{"rounds":8}`, true, intPtr(8), false},
		{"zero sets", `{"rounds":0}`, true, intPtr(0), false},
		{"negative sets", `{"rounds":-3}`, true, intPtr(-3), false},
		{"string is refused", `{"rounds":"8"}`, false, nil, true},
		{"fraction is refused", `{"rounds":8.5}`, false, nil, true},
		{"bool is refused", `{"rounds":true}`, false, nil, true},
		{"object is refused", `{"rounds":{}}`, false, nil, true},
		{"array is refused", `{"rounds":[1]}`, false, nil, true},
		{"overflow is refused", `{"rounds":1e30}`, false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b body
			err := json.Unmarshal([]byte(tt.json), &b)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", b.Rounds)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if b.Rounds.Present != tt.wantPresent {
				t.Fatalf("Present = %v, want %v", b.Rounds.Present, tt.wantPresent)
			}
			if !reflect.DeepEqual(b.Rounds.Value, tt.wantValue) {
				t.Fatalf("Value = %v, want %v", b.Rounds.Value, tt.wantValue)
			}
		})
	}
}

func TestBlockPatchInput_Validate_bounds(t *testing.T) {
	set := func(v int) OptionalInt { return OptionalInt{Present: true, Value: &v} }
	tests := []struct {
		name    string
		in      BlockPatchInput
		wantErr bool
	}{
		{"rounds min", BlockPatchInput{Rounds: set(1)}, false},
		{"rounds max", BlockPatchInput{Rounds: set(100)}, false},
		{"rounds zero", BlockPatchInput{Rounds: set(0)}, true},
		{"rounds above max", BlockPatchInput{Rounds: set(101)}, true},
		{"duration min", BlockPatchInput{DurationSec: set(1)}, false},
		{"duration max", BlockPatchInput{DurationSec: set(86400)}, false},
		{"duration zero", BlockPatchInput{DurationSec: set(0)}, true},
		{"duration above max", BlockPatchInput{DurationSec: set(86401)}, true},
		{"time cap zero", BlockPatchInput{TimeCapSec: set(0)}, true},
		{"time cap above max", BlockPatchInput{TimeCapSec: set(86401)}, true},
		{"work zero", BlockPatchInput{WorkSec: set(0)}, true},
		{"work above max", BlockPatchInput{WorkSec: set(86401)}, true},
		{"rest zero is fine", BlockPatchInput{RestSec: set(0)}, false},
		{"rest max", BlockPatchInput{RestSec: set(86400)}, false},
		{"rest negative", BlockPatchInput{RestSec: set(-1)}, true},
		{"rest above max", BlockPatchInput{RestSec: set(86401)}, true},
		{"null is fine", BlockPatchInput{Rounds: OptionalInt{Present: true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBlockPatchInput_Validate_refusesRemovedTypes(t *testing.T) {
	for _, bt := range []BlockType{"ladder", "death_by", "complex"} {
		in := BlockPatchInput{BlockType: &bt}
		if err := in.Validate(); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s: err = %v, want ErrValidation", bt, err)
		}
	}
}

func TestBlockPatchInput_resolveSettings(t *testing.T) {
	set := func(v int) OptionalInt { return OptionalInt{Present: true, Value: &v} }
	null := OptionalInt{Present: true}
	current := blockSettings{Rounds: intPtr(5), DurationSec: intPtr(600), TimeCapSec: intPtr(900), WorkSec: intPtr(30), RestSec: intPtr(10)}

	tests := []struct {
		name      string
		in        BlockPatchInput
		resulting BlockType
		current   blockSettings
		want      blockSettings
		wantErr   bool
	}{
		{
			name:      "absent allowed field keeps the current value",
			in:        BlockPatchInput{},
			resulting: BlockTypeEMOM,
			current:   blockSettings{Rounds: intPtr(5)},
			want:      blockSettings{Rounds: intPtr(5)},
		},
		{
			name:      "integer sets an allowed field",
			in:        BlockPatchInput{Rounds: set(8)},
			resulting: BlockTypeEMOM,
			current:   blockSettings{Rounds: intPtr(5)},
			want:      blockSettings{Rounds: intPtr(8)},
		},
		{
			name:      "null clears an allowed field",
			in:        BlockPatchInput{Rounds: null},
			resulting: BlockTypeEMOM,
			current:   blockSettings{Rounds: intPtr(5)},
			want:      blockSettings{},
		},
		{
			name:      "fields a type does not allow are reset",
			in:        BlockPatchInput{},
			resulting: BlockTypeEMOM,
			current:   current,
			want:      blockSettings{Rounds: intPtr(5)},
		},
		{
			name:      "rounds carry over from emom to for_time and keep its own fields unset",
			in:        BlockPatchInput{},
			resulting: BlockTypeForTime,
			current:   blockSettings{Rounds: intPtr(5)},
			want:      blockSettings{Rounds: intPtr(5)},
		},
		{
			name:      "intervals keep rounds, work and rest",
			in:        BlockPatchInput{},
			resulting: BlockTypeIntervals,
			current:   current,
			want:      blockSettings{Rounds: intPtr(5), WorkSec: intPtr(30), RestSec: intPtr(10)},
		},
		{
			name:      "a type without fields resets everything",
			in:        BlockPatchInput{},
			resulting: BlockTypeChipper,
			current:   current,
			want:      blockSettings{},
		},
		{
			name:      "null for a not allowed field is accepted",
			in:        BlockPatchInput{DurationSec: null},
			resulting: BlockTypeEMOM,
			current:   blockSettings{Rounds: intPtr(5)},
			want:      blockSettings{Rounds: intPtr(5)},
		},
		{
			name:      "value for a not allowed field is refused",
			in:        BlockPatchInput{DurationSec: set(600)},
			resulting: BlockTypeEMOM,
			wantErr:   true,
		},
		{
			name:      "value for a type without fields is refused",
			in:        BlockPatchInput{Rounds: set(3)},
			resulting: BlockTypeSkillWork,
			wantErr:   true,
		},
		{
			name:      "zero rest stays zero",
			in:        BlockPatchInput{RestSec: set(0)},
			resulting: BlockTypeIntervals,
			want:      blockSettings{RestSec: intPtr(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.resolveSettings(tt.resulting, tt.current)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
				if !strings.Contains(err.Error(), "not allowed") {
					t.Fatalf("err = %v, want a not allowed message", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("settings = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnsureSetsAllowed(t *testing.T) {
	sets := "3"
	tests := []struct {
		name      string
		blockType BlockType
		sets      *string
		wantErr   bool
	}{
		{"emom with sets", BlockTypeEMOM, &sets, true},
		{"chipper with sets", BlockTypeChipper, &sets, true},
		{"emom without sets", BlockTypeEMOM, nil, false},
		{"skill work with sets", BlockTypeSkillWork, &sets, false},
		{"single with sets", BlockTypeSingle, &sets, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureSetsAllowed(tt.blockType, tt.sets)
			if tt.wantErr != errors.Is(err, ErrValidation) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInt32Conversion(t *testing.T) {
	if intFromInt32(nil) != nil || int32FromInt(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	v := int32(7)
	if got := intFromInt32(&v); got == nil || *got != 7 {
		t.Fatalf("intFromInt32 = %v", got)
	}
	if got := int32FromInt(intPtr(0)); got == nil || *got != 0 {
		t.Fatalf("int32FromInt(0) = %v", got)
	}
}

func TestBlockPatchInput_resolveSettings_unknownTypeIsRefused(t *testing.T) {
	_, err := BlockPatchInput{}.resolveSettings(BlockType("complex"), blockSettings{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
