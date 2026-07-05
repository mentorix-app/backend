package program

type BlockType string

const (
	BlockTypeSingle    BlockType = "single"
	BlockTypeEMOM      BlockType = "emom"
	BlockTypeAMRAP     BlockType = "amrap"
	BlockTypeForTime   BlockType = "for_time"
	BlockTypeIntervals BlockType = "intervals"
	BlockTypeChipper   BlockType = "chipper"
	BlockTypeLadder    BlockType = "ladder"
	BlockTypeDeathBy   BlockType = "death_by"
	BlockTypeSuperset  BlockType = "superset"
	BlockTypeComplex   BlockType = "complex"
)

func (t BlockType) valid() bool {
	switch t {
	case BlockTypeSingle, BlockTypeEMOM, BlockTypeAMRAP, BlockTypeForTime,
		BlockTypeIntervals, BlockTypeChipper, BlockTypeLadder, BlockTypeDeathBy,
		BlockTypeSuperset, BlockTypeComplex:
		return true
	default:
		return false
	}
}

func (t BlockType) isGroup() bool {
	return t != BlockTypeSingle
}
