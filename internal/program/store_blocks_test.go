package program

import "testing"

func Test_mergeBlockInstructions(t *testing.T) {
	blocks := []mergeBlock{
		{BlockType: BlockTypeSingle, Instruction: "ignored"},
		{BlockType: BlockTypeEMOM, Instruction: "  60s work  "},
		{BlockType: BlockTypeAMRAP, Instruction: ""},
		{BlockType: BlockTypeForTime, Instruction: "20 min cap"},
	}
	got := mergeBlockInstructions(blocks)
	want := "60s work\n\n20 min cap"
	if got != want {
		t.Fatalf("mergeBlockInstructions() = %q, want %q", got, want)
	}
	if mergeBlockInstructions(nil) != "" {
		t.Fatal("expected empty string for nil blocks")
	}
}
