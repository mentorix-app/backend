package program

import "testing"

func TestBlockType_valid(t *testing.T) {
	valid := []BlockType{
		BlockTypeSingle, BlockTypeEMOM, BlockTypeAMRAP, BlockTypeForTime,
		BlockTypeIntervals, BlockTypeChipper, BlockTypeSuperset, BlockTypeSkillWork,
		BlockTypeStrength, BlockTypeConditioning, BlockTypeGymnastics, BlockTypeWeightlifting,
	}
	for _, bt := range valid {
		if !bt.valid() {
			t.Fatalf("%q should be valid", bt)
		}
		if _, ok := bt.rule(); !ok {
			t.Fatalf("%q has no rule", bt)
		}
	}
}

func TestBlockType_removedAndUnknownAreInvalid(t *testing.T) {
	for _, bt := range []BlockType{"ladder", "death_by", "complex", "unknown", ""} {
		if bt.valid() {
			t.Fatalf("%q should be invalid", bt)
		}
	}
}

func TestBlockType_unknownHasNoRule(t *testing.T) {
	if r, ok := BlockType("complex").rule(); ok || r != (blockRule{}) {
		t.Fatalf("rule = %+v, ok = %v, want zero rule and ok false", r, ok)
	}
}

func TestBlockType_isGroup(t *testing.T) {
	if BlockTypeSingle.isGroup() {
		t.Fatal("single block should not be group")
	}
	if !BlockTypeEMOM.isGroup() {
		t.Fatal("emom block should be group")
	}
}

func TestBlockRules(t *testing.T) {
	tests := []struct {
		blockType BlockType
		noSets    bool
		want      blockRule
	}{
		{BlockTypeEMOM, true, blockRule{noSets: true, rounds: true}},
		{BlockTypeAMRAP, true, blockRule{noSets: true, durationSec: true}},
		{BlockTypeForTime, true, blockRule{noSets: true, rounds: true, timeCapSec: true}},
		{BlockTypeIntervals, true, blockRule{noSets: true, rounds: true, workSec: true, restSec: true}},
		{BlockTypeSuperset, true, blockRule{noSets: true, rounds: true}},
		{BlockTypeChipper, true, blockRule{noSets: true}},
		{BlockTypeSingle, false, blockRule{}},
		{BlockTypeSkillWork, false, blockRule{}},
		{BlockTypeStrength, false, blockRule{}},
		{BlockTypeConditioning, false, blockRule{}},
		{BlockTypeGymnastics, false, blockRule{}},
		{BlockTypeWeightlifting, false, blockRule{}},
	}
	for _, tt := range tests {
		t.Run(string(tt.blockType), func(t *testing.T) {
			got, ok := tt.blockType.rule()
			if !ok {
				t.Fatalf("%q has no rule", tt.blockType)
			}
			if got != tt.want {
				t.Fatalf("rule = %+v, want %+v", got, tt.want)
			}
			if got := tt.blockType.allowsSets(); got == tt.noSets {
				t.Fatalf("allowsSets = %v, want %v", got, !tt.noSets)
			}
		})
	}
}
