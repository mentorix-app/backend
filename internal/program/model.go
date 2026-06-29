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
)

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
	ID              uuid.UUID   `json:"id"`
	CreatedBy       uuid.UUID   `json:"created_by"`
	CreatedByName   string      `json:"created_by_name"`
	ModifiedBy      uuid.UUID   `json:"modified_by"`
	Status          Status      `json:"status"`
	Name            string      `json:"name"`
	NameRu          string      `json:"name_ru"`
	Description     string      `json:"description"`
	DescriptionRu   string      `json:"description_ru"`
	Category        *Category   `json:"category,omitempty"`
	Difficulty      *Difficulty `json:"difficulty,omitempty"`
	PreviewImageURL string      `json:"preview_image_url"`
	CreatedAt       time.Time   `json:"created_at"`
	ModifiedAt      time.Time   `json:"modified_at"`
	DeletedAt       *time.Time  `json:"deleted_at,omitempty"`
}

type DayExercise struct {
	ID             uuid.UUID `json:"id"`
	ExerciseID     uuid.UUID `json:"exercise_id"`
	ExerciseName   string    `json:"exercise_name"`
	ExerciseNameRu string    `json:"exercise_name_ru"`
	SortOrder      int       `json:"sort_order"`
	Sets           *int      `json:"sets,omitempty"`
	Reps           *int      `json:"reps,omitempty"`
	WeightKg       *float64  `json:"weight_kg,omitempty"`
	Instruction    string    `json:"instruction"`
	CreatedAt      time.Time `json:"created_at"`
}

type Day struct {
	ID        uuid.UUID     `json:"id"`
	DayNumber int           `json:"day_number"`
	SortOrder int           `json:"sort_order"`
	Exercises []DayExercise `json:"exercises"`
	CreatedAt time.Time     `json:"created_at"`
}

type Detail struct {
	Program
	Days []Day `json:"days"`
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
	WeightKg    *float64
	Instruction *string
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
	if in.Sets != nil && *in.Sets <= 0 {
		return fmt.Errorf("%w: sets must be positive", ErrValidation)
	}
	if in.Reps != nil && *in.Reps <= 0 {
		return fmt.Errorf("%w: reps must be positive", ErrValidation)
	}
	if in.WeightKg != nil && *in.WeightKg < 0 {
		return fmt.Errorf("%w: weight_kg cannot be negative", ErrValidation)
	}
	return nil
}

func (in DayExerciseInput) ValidatePublish() error {
	if err := in.ValidateDraft(); err != nil {
		return err
	}
	if in.Sets == nil || *in.Sets <= 0 {
		return fmt.Errorf("%w: sets is required", ErrValidation)
	}
	if in.Reps == nil || *in.Reps <= 0 {
		return fmt.Errorf("%w: reps is required", ErrValidation)
	}
	return nil
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
	if len(d.Days) == 0 {
		return fmt.Errorf("%w: at least one day is required", ErrValidation)
	}
	for _, day := range d.Days {
		if len(day.Exercises) == 0 {
			return fmt.Errorf("%w: day %d must have at least one exercise", ErrValidation, day.DayNumber)
		}
		for _, ex := range day.Exercises {
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
