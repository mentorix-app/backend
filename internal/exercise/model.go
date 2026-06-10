package exercise

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrValidation = errors.New("validation failed")

type MuscleGroup string

const (
	MuscleGroupCompound  MuscleGroup = "compound"
	MuscleGroupChest     MuscleGroup = "chest"
	MuscleGroupBack      MuscleGroup = "back"
	MuscleGroupLegs      MuscleGroup = "legs"
	MuscleGroupShoulders MuscleGroup = "shoulders"
	MuscleGroupArms      MuscleGroup = "arms"
	MuscleGroupCore      MuscleGroup = "core"
	MuscleGroupFullBody  MuscleGroup = "full_body"
)

type ExerciseType string

const (
	ExerciseTypeStrength   ExerciseType = "strength"
	ExerciseTypeCardio     ExerciseType = "cardio"
	ExerciseTypeMixedModal ExerciseType = "mixed_modal"
	ExerciseTypeIntervals  ExerciseType = "intervals"
	ExerciseTypeStretching ExerciseType = "stretching"
	ExerciseTypeMetcon     ExerciseType = "metcon"
	ExerciseTypeSkillWork  ExerciseType = "skill_work"
	ExerciseTypeAccessory  ExerciseType = "accessory"
)

type Equipment string

const (
	EquipmentBarbell         Equipment = "barbell"
	EquipmentDumbbells       Equipment = "dumbbells"
	EquipmentKettlebell      Equipment = "kettlebell"
	EquipmentPullUpBar       Equipment = "pull_up_bar"
	EquipmentSquatRack       Equipment = "squat_rack"
	EquipmentRowingMachine   Equipment = "rowing_machine"
	EquipmentAssaultBike     Equipment = "assault_bike"
	EquipmentJumpRope        Equipment = "jump_rope"
	EquipmentPlyoBox         Equipment = "plyo_box"
	EquipmentMedicineBall    Equipment = "medicine_ball"
	EquipmentWallBall        Equipment = "wall_ball"
	EquipmentResistanceBands Equipment = "resistance_bands"
	EquipmentBattleRopes     Equipment = "battle_ropes"
	EquipmentGymnasticRings  Equipment = "gymnastic_rings"
	EquipmentSandbag         Equipment = "sandbag"
	EquipmentSled            Equipment = "sled"
	EquipmentWeightPlates    Equipment = "weight_plates"
)

type Difficulty string

const (
	DifficultyBeginner     Difficulty = "beginner"
	DifficultyIntermediate Difficulty = "intermediate"
	DifficultyAdvanced     Difficulty = "advanced"
	DifficultyExpert       Difficulty = "expert"
)

type Exercise struct {
	ID              uuid.UUID    `json:"id"`
	Name            string       `json:"name"`
	NameRu          string       `json:"name_ru"`
	AddedBy         uuid.UUID    `json:"added_by"`
	ModifiedBy      uuid.UUID    `json:"modified_by"`
	ModifiedAt      time.Time    `json:"modified_at"`
	CreatedAt       time.Time    `json:"created_at"`
	Equipment       *Equipment   `json:"equipment,omitempty"`
	Type            ExerciseType `json:"type"`
	MuscleGroup     MuscleGroup  `json:"muscle_group"`
	Description     string       `json:"description"`
	DescriptionRu   string       `json:"description_ru"`
	Difficulty      Difficulty   `json:"difficulty"`
	VideoURL        string       `json:"video_url"`
	PreviewImageURL string       `json:"preview_image_url"`
}

type UpsertInput struct {
	Name            string
	NameRu          string
	Equipment       *Equipment
	Type            ExerciseType
	MuscleGroup     MuscleGroup
	Description     string
	DescriptionRu   string
	Difficulty      Difficulty
	VideoURL        string
	PreviewImageURL string
}

func (in UpsertInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if !in.MuscleGroup.valid() {
		return fmt.Errorf("%w: invalid muscle_group", ErrValidation)
	}
	if !in.Type.valid() {
		return fmt.Errorf("%w: invalid type", ErrValidation)
	}
	if in.Equipment != nil && !in.Equipment.valid() {
		return fmt.Errorf("%w: invalid equipment", ErrValidation)
	}
	if !in.Difficulty.valid() {
		return fmt.Errorf("%w: invalid difficulty", ErrValidation)
	}
	return nil
}

func (g MuscleGroup) valid() bool {
	switch g {
	case MuscleGroupCompound, MuscleGroupChest, MuscleGroupBack, MuscleGroupLegs,
		MuscleGroupShoulders, MuscleGroupArms, MuscleGroupCore, MuscleGroupFullBody:
		return true
	default:
		return false
	}
}

func (t ExerciseType) valid() bool {
	switch t {
	case ExerciseTypeStrength, ExerciseTypeCardio, ExerciseTypeMixedModal, ExerciseTypeIntervals, ExerciseTypeStretching,
		ExerciseTypeMetcon, ExerciseTypeSkillWork, ExerciseTypeAccessory:
		return true
	default:
		return false
	}
}

func (e Equipment) valid() bool {
	switch e {
	case EquipmentBarbell, EquipmentDumbbells, EquipmentKettlebell, EquipmentPullUpBar,
		EquipmentSquatRack, EquipmentRowingMachine, EquipmentAssaultBike, EquipmentJumpRope,
		EquipmentPlyoBox, EquipmentMedicineBall, EquipmentWallBall, EquipmentResistanceBands,
		EquipmentBattleRopes, EquipmentGymnasticRings, EquipmentSandbag, EquipmentSled, EquipmentWeightPlates:
		return true
	default:
		return false
	}
}

func (d Difficulty) valid() bool {
	switch d {
	case DifficultyBeginner, DifficultyIntermediate, DifficultyAdvanced, DifficultyExpert:
		return true
	default:
		return false
	}
}
