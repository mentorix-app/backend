package program

type BlockType string

const (
	BlockTypeSingle        BlockType = "single"
	BlockTypeEMOM          BlockType = "emom"
	BlockTypeAMRAP         BlockType = "amrap"
	BlockTypeForTime       BlockType = "for_time"
	BlockTypeIntervals     BlockType = "intervals"
	BlockTypeChipper       BlockType = "chipper"
	BlockTypeSuperset      BlockType = "superset"
	BlockTypeSkillWork     BlockType = "skill_work"
	BlockTypeStrength      BlockType = "strength"
	BlockTypeConditioning  BlockType = "conditioning"
	BlockTypeGymnastics    BlockType = "gymnastics"
	BlockTypeWeightlifting BlockType = "weightlifting"
)

// blockRule says which format settings a block type accepts and whether its
// exercises carry sets.
type blockRule struct {
	noSets      bool
	rounds      bool
	durationSec bool
	timeCapSec  bool
	workSec     bool
	restSec     bool
}

// rule returns the rules of a known block type; ok is false for any other value.
func (t BlockType) rule() (r blockRule, ok bool) {
	switch t {
	case BlockTypeSingle, BlockTypeSkillWork, BlockTypeStrength, BlockTypeConditioning,
		BlockTypeGymnastics, BlockTypeWeightlifting:
		return blockRule{}, true
	case BlockTypeEMOM, BlockTypeSuperset:
		return blockRule{noSets: true, rounds: true}, true
	case BlockTypeAMRAP:
		return blockRule{noSets: true, durationSec: true}, true
	case BlockTypeForTime:
		return blockRule{noSets: true, rounds: true, timeCapSec: true}, true
	case BlockTypeIntervals:
		return blockRule{noSets: true, rounds: true, workSec: true, restSec: true}, true
	case BlockTypeChipper:
		return blockRule{noSets: true}, true
	default:
		return blockRule{}, false
	}
}

func (t BlockType) valid() bool {
	_, ok := t.rule()
	return ok
}

func (t BlockType) isGroup() bool {
	return t != BlockTypeSingle
}

// allowsSets reports whether exercises of a block of this type may carry sets.
// An unknown type counts as allowing them: callers validate the type first.
func (t BlockType) allowsSets() bool {
	r, _ := t.rule()
	return !r.noSets
}
