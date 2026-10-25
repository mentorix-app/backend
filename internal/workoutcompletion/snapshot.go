package workoutcompletion

import (
	"encoding/json"

	"mentorix-backend/internal/program"
)

type snapshotExercise struct {
	Name        string  `json:"name"`
	NameRu      string  `json:"name_ru"`
	Sets        *string `json:"sets,omitempty"`
	Reps        *string `json:"reps,omitempty"`
	Instruction string  `json:"instruction,omitempty"`
}

type snapshotBlock struct {
	BlockType   string             `json:"block_type"`
	Instruction string             `json:"instruction,omitempty"`
	Exercises   []snapshotExercise `json:"exercises"`
	Rounds      *int               `json:"rounds,omitempty"`
	DurationSec *int               `json:"duration_sec,omitempty"`
	TimeCapSec  *int               `json:"time_cap_sec,omitempty"`
	WorkSec     *int               `json:"work_sec,omitempty"`
	RestSec     *int               `json:"rest_sec,omitempty"`
}

type daySnapshot struct {
	Blocks []snapshotBlock `json:"blocks"`
}

func buildDaySnapshot(day program.Day) (json.RawMessage, error) {
	blocks := make([]snapshotBlock, 0, len(day.Blocks))
	for _, b := range day.Blocks {
		exs := make([]snapshotExercise, 0, len(b.Exercises))
		for _, ex := range b.Exercises {
			exs = append(exs, snapshotExercise{
				Name:        ex.ExerciseName,
				NameRu:      ex.ExerciseNameRu,
				Sets:        ex.Sets,
				Reps:        ex.Reps,
				Instruction: ex.Instruction,
			})
		}
		blocks = append(blocks, snapshotBlock{
			BlockType:   string(b.BlockType),
			Instruction: b.Instruction,
			Exercises:   exs,
			Rounds:      b.Rounds,
			DurationSec: b.DurationSec,
			TimeCapSec:  b.TimeCapSec,
			WorkSec:     b.WorkSec,
			RestSec:     b.RestSec,
		})
	}
	return json.Marshal(daySnapshot{Blocks: blocks})
}
