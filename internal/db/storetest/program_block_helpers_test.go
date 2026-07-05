//go:build integration

package storetest

import (
	"context"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

func dayBlockCount(day program.Day) int {
	return len(day.Blocks)
}

func dayExerciseCount(day program.Day) int {
	n := 0
	for _, b := range day.Blocks {
		n += len(b.Exercises)
	}
	return n
}

func firstDayExercise(day program.Day) (program.DayExercise, bool) {
	for _, b := range day.Blocks {
		if len(b.Exercises) > 0 {
			return b.Exercises[0], true
		}
	}
	return program.DayExercise{}, false
}

func firstDayBlock(day program.Day) (program.DayBlock, bool) {
	if len(day.Blocks) == 0 {
		return program.DayBlock{}, false
	}
	return day.Blocks[0], true
}

func findDay(week program.Week, dayID uuid.UUID) (program.Day, bool) {
	for _, day := range week.Days {
		if day.ID == dayID {
			return day, true
		}
	}
	return program.Day{}, false
}

func createSingleBlock(ctx context.Context, svc *program.Service, userID, programID, weekID, dayID uuid.UUID, in program.DayExerciseInput) (program.Detail, error) {
	return svc.CreateDayBlock(ctx, userID, programID, weekID, dayID, program.CreateDayBlockInput{
		BlockType: program.BlockTypeSingle,
		Exercise:  &in,
	})
}

func createSingleBlockStore(ctx context.Context, store *program.Store, userID, programID, weekID, dayID uuid.UUID, in program.DayExerciseInput) (program.Detail, error) {
	return store.CreateDayBlock(ctx, userID, programID, weekID, dayID, program.CreateDayBlockInput{
		BlockType: program.BlockTypeSingle,
		Exercise:  &in,
	})
}
