package program

import "testing"

func TestBlockType_valid(t *testing.T) {
	valid := []BlockType{
		BlockTypeSingle, BlockTypeEMOM, BlockTypeAMRAP, BlockTypeForTime,
		BlockTypeIntervals, BlockTypeChipper, BlockTypeLadder, BlockTypeDeathBy,
		BlockTypeSuperset, BlockTypeComplex, BlockTypeSkillWork, BlockTypeStrength,
		BlockTypeConditioning, BlockTypeGymnastics, BlockTypeWeightlifting,
	}
	for _, bt := range valid {
		if !bt.valid() {
			t.Fatalf("%q should be valid", bt)
		}
	}
	if BlockType("unknown").valid() {
		t.Fatal("unknown block type should be invalid")
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
