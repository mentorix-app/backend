package program

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
)

func (s *Store) GetVersionDetail(ctx context.Context, versionID uuid.UUID) (Detail, error) {
	versionPG := pgconv.ToPGUUID(versionID)
	version, err := s.q.GetProgramVersionByID(ctx, versionPG)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get program version: %w", err)
	}

	weekRows, err := s.q.ListProgramVersionWeeksByVersionID(ctx, versionPG)
	if err != nil {
		return Detail{}, fmt.Errorf("list version weeks: %w", err)
	}
	dayRows, err := s.q.ListProgramVersionDaysByVersionID(ctx, versionPG)
	if err != nil {
		return Detail{}, fmt.Errorf("list version days: %w", err)
	}
	blockRows, err := s.q.ListProgramVersionDayBlocksByVersionID(ctx, versionPG)
	if err != nil {
		return Detail{}, fmt.Errorf("list version blocks: %w", err)
	}
	exerciseRows, err := s.q.ListProgramVersionDayExercisesWithNamesByVersionID(ctx, versionPG)
	if err != nil {
		return Detail{}, fmt.Errorf("list version exercises: %w", err)
	}

	daysByWeek := make(map[uuid.UUID][]Day, len(weekRows))
	for _, dayRow := range dayRows {
		weekID := pgconv.FromPGUUID(dayRow.ProgramVersionWeekID)
		daysByWeek[weekID] = append(daysByWeek[weekID], Day{
			ID:        pgconv.FromPGUUID(dayRow.ID),
			DayKey:    pgconv.FromPGUUID(dayRow.DayKey),
			DayNumber: int(dayRow.DayNumber),
			SortOrder: int(dayRow.SortOrder),
			Blocks:    nil,
			CreatedAt: dayRow.CreatedAt.UTC(),
		})
	}

	blocksByDay := make(map[uuid.UUID][]DayBlock, len(blockRows))
	for _, blockRow := range blockRows {
		dayID := pgconv.FromPGUUID(blockRow.ProgramVersionWeekDayID)
		blocksByDay[dayID] = append(blocksByDay[dayID], DayBlock{
			ID:          pgconv.FromPGUUID(blockRow.ID),
			BlockType:   BlockType(blockRow.BlockType),
			Instruction: blockRow.Instruction,
			SortOrder:   int(blockRow.SortOrder),
			Exercises:   nil,
			CreatedAt:   blockRow.CreatedAt.UTC(),
		})
	}

	exercisesByBlock := make(map[uuid.UUID][]DayExercise, len(exerciseRows))
	for _, exRow := range exerciseRows {
		blockID := pgconv.FromPGUUID(exRow.ProgramVersionWeekDayBlockID)
		var sets, reps *int
		if exRow.Sets != nil {
			v := int(*exRow.Sets)
			sets = &v
		}
		if exRow.Reps != nil {
			v := int(*exRow.Reps)
			reps = &v
		}
		exercisesByBlock[blockID] = append(exercisesByBlock[blockID], DayExercise{
			ID:             pgconv.FromPGUUID(exRow.ID),
			ExerciseID:     pgconv.FromPGUUID(exRow.ExerciseID),
			ExerciseName:   exRow.ExerciseName,
			ExerciseNameRu: exRow.ExerciseNameRu,
			SortOrder:      int(exRow.SortOrder),
			Sets:           sets,
			Reps:           reps,
			Instruction:    exRow.Instruction,
			CreatedAt:      exRow.CreatedAt.UTC(),
		})
	}

	for weekID, days := range daysByWeek {
		for i := range days {
			dayID := days[i].ID
			blocks := blocksByDay[dayID]
			for j := range blocks {
				blockID := blocks[j].ID
				blocks[j].Exercises = exercisesByBlock[blockID]
			}
			days[i].Blocks = blocks
		}
		daysByWeek[weekID] = days
	}

	weeks := make([]Week, 0, len(weekRows))
	for _, weekRow := range weekRows {
		weekID := pgconv.FromPGUUID(weekRow.ID)
		weeks = append(weeks, Week{
			ID:         weekID,
			WeekNumber: int(weekRow.WeekNumber),
			SortOrder:  int(weekRow.SortOrder),
			Days:       daysByWeek[weekID],
			CreatedAt:  weekRow.CreatedAt.UTC(),
		})
	}

	var category *Category
	if version.Category != nil {
		c := Category(*version.Category)
		category = &c
	}
	var difficulty *Difficulty
	if version.Difficulty != nil {
		d := Difficulty(*version.Difficulty)
		difficulty = &d
	}

	return Detail{
		Program: Program{
			ID:              pgconv.FromPGUUID(version.ProgramID),
			Status:          StatusPublished,
			Name:            version.Name,
			NameRu:          version.NameRu,
			Description:     version.Description,
			DescriptionRu:   version.DescriptionRu,
			Category:        category,
			Difficulty:      difficulty,
			PreviewImageURL: version.PreviewImageUrl,
			CreatedAt:       version.CreatedAt.UTC(),
			ModifiedAt:      version.PublishedAt.UTC(),
		},
		Weeks: weeks,
	}, nil
}
