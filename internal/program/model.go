package program

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

var (
	ErrValidation              = errors.New("validation failed")
	ErrForbidden               = errors.New("forbidden")
	ErrNotFound                = errors.New("program not found")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrReadOnly                = errors.New("program is read-only")
	ErrClientNotLinked         = errors.New("client is not linked to trainer")
	ErrClientBlocked           = errors.New("client is blocked")
	ErrClientNotFound          = errors.New("client not found")
	ErrProgramNotPublished     = errors.New("program is not published")
	ErrAlreadyAssigned         = errors.New("program already assigned")
	ErrNoUnpublishedChanges    = errors.New("no unpublished changes to publish")
	ErrInvalidSyncRequest      = errors.New("invalid sync request")
	ErrVersionHasAssignments   = errors.New("version has assignments")
	ErrSoleProgramVersion      = errors.New("cannot delete the only program version")
	ErrLastWeek                = errors.New("cannot delete the last week")
	ErrLastDay                 = errors.New("cannot delete the last day in week")
	ErrMaxDaysPerWeek          = errors.New("week cannot have more than 7 days")
	ErrInvalidReorder          = errors.New("invalid reorder")
)

const DefaultWeekDays = 7

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

type Category string

const (
	CategoryWeightLoss     Category = "weight_loss"
	CategoryMuscleGain     Category = "muscle_gain"
	CategoryRehabilitation Category = "rehabilitation"
	CategoryEndurance      Category = "endurance"
	CategoryFunctional     Category = "functional"
)

type Difficulty = exercise.Difficulty

type Program struct {
	ID                     uuid.UUID   `json:"id"`
	CreatedBy              uuid.UUID   `json:"created_by"`
	CreatedByName          string      `json:"created_by_name"`
	ModifiedBy             uuid.UUID   `json:"modified_by"`
	Status                 Status      `json:"status"`
	Name                   string      `json:"name"`
	NameRu                 string      `json:"name_ru"`
	Description            string      `json:"description"`
	DescriptionRu          string      `json:"description_ru"`
	Category               *Category   `json:"category,omitempty"`
	Difficulty             *Difficulty `json:"difficulty,omitempty"`
	PreviewImageURL        string      `json:"preview_image_url"`
	LatestProgramVersionID *uuid.UUID  `json:"latest_program_version_id"`
	LatestClientPlanAt     *time.Time  `json:"latest_client_plan_at"`
	HasUnpublishedChanges  bool        `json:"has_unpublished_changes"`
	AssignmentCount        int         `json:"assignment_count"`
	TrainingDaysCount      int         `json:"training_days_count"`
	CreatedAt              time.Time   `json:"created_at"`
	ModifiedAt             time.Time   `json:"modified_at"`
	DeletedAt              *time.Time  `json:"deleted_at,omitempty"`
}

type DayExercise struct {
	ID             uuid.UUID `json:"id"`
	ExerciseID     uuid.UUID `json:"exercise_id"`
	ExerciseName   string    `json:"exercise_name"`
	ExerciseNameRu string    `json:"exercise_name_ru"`
	SortOrder      int       `json:"sort_order"`
	Sets           *int      `json:"sets,omitempty"`
	Reps           *int      `json:"reps,omitempty"`
	Instruction    string    `json:"instruction"`
	CreatedAt      time.Time `json:"created_at"`
}

type DayBlock struct {
	ID          uuid.UUID     `json:"id"`
	BlockType   BlockType     `json:"block_type"`
	Instruction string        `json:"instruction"`
	SortOrder   int           `json:"sort_order"`
	Exercises   []DayExercise `json:"exercises"`
	CreatedAt   time.Time     `json:"created_at"`
}

type Week struct {
	ID         uuid.UUID `json:"id"`
	WeekNumber int       `json:"week_number"`
	SortOrder  int       `json:"sort_order"`
	Days       []Day     `json:"days"`
	CreatedAt  time.Time `json:"created_at"`
}

type Day struct {
	ID        uuid.UUID  `json:"id"`
	DayKey    uuid.UUID  `json:"-"`
	DayNumber int        `json:"day_number"`
	SortOrder int        `json:"sort_order"`
	Blocks    []DayBlock `json:"blocks"`
	CreatedAt time.Time  `json:"created_at"`
}

type Detail struct {
	Program
	Weeks []Week `json:"weeks"`
}

type UpdateInput struct {
	Name            *string
	NameRu          *string
	Description     *string
	DescriptionRu   *string
	Category        *Category
	Difficulty      *Difficulty
	PreviewImageURL *string
}

type DayExerciseInput struct {
	ExerciseID  uuid.UUID
	Sets        *int
	Reps        *int
	Instruction *string
}

type CreateDayBlockInput struct {
	BlockType BlockType
	SortOrder int // 0 = append to day
	Exercise  *DayExerciseInput
}

func (in CreateDayBlockInput) ValidateDraft() error {
	blockType := in.BlockType
	if blockType == "" {
		blockType = BlockTypeSingle
	}
	if blockType != BlockTypeSingle {
		return fmt.Errorf("%w: only single blocks can be created directly; use merge for groups", ErrValidation)
	}
	if in.Exercise == nil {
		return fmt.Errorf("%w: exercise is required for single block", ErrValidation)
	}
	return in.Exercise.ValidateDraft()
}

type BlockPatchInput struct {
	BlockType   *BlockType
	Instruction *string
}

type WeekBlockReorderDay struct {
	DayID    uuid.UUID
	BlockIDs []uuid.UUID
}

type BlockExerciseReorder struct {
	BlockID         uuid.UUID
	ExerciseItemIDs []uuid.UUID
}

func (s Status) valid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusArchived:
		return true
	default:
		return false
	}
}

func (c Category) valid() bool {
	switch c {
	case CategoryWeightLoss, CategoryMuscleGain, CategoryRehabilitation,
		CategoryEndurance, CategoryFunctional:
		return true
	default:
		return false
	}
}

func (in UpdateInput) Validate() error {
	if in.Category != nil && !in.Category.valid() {
		return fmt.Errorf("%w: invalid category", ErrValidation)
	}
	if in.Difficulty != nil && !difficultyValid(*in.Difficulty) {
		return fmt.Errorf("%w: invalid difficulty", ErrValidation)
	}
	return nil
}

func (in DayExerciseInput) ValidateDraft() error {
	if in.ExerciseID == uuid.Nil {
		return fmt.Errorf("%w: exercise_id is required", ErrValidation)
	}
	if in.Sets != nil && *in.Sets < 1 {
		return fmt.Errorf("%w: sets must be >= 1", ErrValidation)
	}
	if in.Reps != nil && *in.Reps < 1 {
		return fmt.Errorf("%w: reps must be >= 1", ErrValidation)
	}
	return nil
}

func (in BlockPatchInput) Validate() error {
	if in.BlockType != nil {
		if !in.BlockType.valid() {
			return fmt.Errorf("%w: invalid block_type", ErrValidation)
		}
		if !in.BlockType.isGroup() {
			return fmt.Errorf("%w: single block_type cannot be set via patch", ErrValidation)
		}
	}
	return nil
}

func (in DayExerciseInput) ValidatePublish() error {
	return in.ValidateDraft()
}

func validatePublishDetail(d Detail) error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if d.Category == nil {
		return fmt.Errorf("%w: category is required", ErrValidation)
	}
	if d.Difficulty == nil {
		return fmt.Errorf("%w: difficulty is required", ErrValidation)
	}
	if len(d.Weeks) == 0 {
		return fmt.Errorf("%w: at least one week is required", ErrValidation)
	}
	for _, week := range d.Weeks {
		hasTrainingDay := false
		for _, day := range week.Days {
			dayHasExercises := false
			for _, block := range day.Blocks {
				if block.BlockType == BlockTypeSingle {
					if len(block.Exercises) != 1 {
						return fmt.Errorf("%w: single block must have exactly one exercise", ErrValidation)
					}
				} else if len(block.Exercises) < 1 {
					return fmt.Errorf("%w: group block must have at least one exercise", ErrValidation)
				}
				if len(block.Exercises) > 0 {
					dayHasExercises = true
				}
				for _, ex := range block.Exercises {
					in := DayExerciseInput{
						ExerciseID: ex.ExerciseID,
						Sets:       ex.Sets,
						Reps:       ex.Reps,
					}
					if err := in.ValidatePublish(); err != nil {
						return err
					}
				}
			}
			if dayHasExercises {
				hasTrainingDay = true
			}
		}
		if !hasTrainingDay {
			return fmt.Errorf("%w: week %d must have at least one day with exercises", ErrValidation, week.WeekNumber)
		}
	}
	return nil
}

func difficultyValid(d Difficulty) bool {
	switch d {
	case exercise.DifficultyBeginner, exercise.DifficultyIntermediate,
		exercise.DifficultyAdvanced, exercise.DifficultyExpert:
		return true
	default:
		return false
	}
}
