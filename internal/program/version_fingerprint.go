package program

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type detailFingerprint struct {
	Name            string            `json:"name"`
	NameRu          string            `json:"name_ru"`
	Description     string            `json:"description"`
	DescriptionRu   string            `json:"description_ru"`
	Category        *string           `json:"category,omitempty"`
	Difficulty      *string           `json:"difficulty,omitempty"`
	PreviewImageURL string            `json:"preview_image_url"`
	Weeks           []weekFingerprint `json:"weeks"`
}

type weekFingerprint struct {
	WeekNumber int              `json:"week_number"`
	SortOrder  int              `json:"sort_order"`
	Days       []dayFingerprint `json:"days"`
}

type dayFingerprint struct {
	DayNumber int                 `json:"day_number"`
	SortOrder int                 `json:"sort_order"`
	Blocks    []blockFingerprint  `json:"blocks"`
}

type blockFingerprint struct {
	BlockType   string                `json:"block_type"`
	Instruction string                `json:"instruction"`
	SortOrder   int                   `json:"sort_order"`
	Exercises   []exerciseFingerprint `json:"exercises"`
}

type exerciseFingerprint struct {
	ExerciseID  string `json:"exercise_id"`
	SortOrder   int    `json:"sort_order"`
	Sets        *int   `json:"sets,omitempty"`
	Reps        *int   `json:"reps,omitempty"`
	Instruction string `json:"instruction"`
}

func DetailFingerprint(d Detail) (string, error) {
	fp := detailFingerprint{
		Name:            d.Name,
		NameRu:          d.NameRu,
		Description:     d.Description,
		DescriptionRu:   d.DescriptionRu,
		PreviewImageURL: d.PreviewImageURL,
		Weeks:           make([]weekFingerprint, 0, len(d.Weeks)),
	}
	if d.Category != nil {
		c := string(*d.Category)
		fp.Category = &c
	}
	if d.Difficulty != nil {
		diff := string(*d.Difficulty)
		fp.Difficulty = &diff
	}

	weeks := append([]Week(nil), d.Weeks...)
	sort.Slice(weeks, func(i, j int) bool {
		if weeks[i].SortOrder == weeks[j].SortOrder {
			return weeks[i].WeekNumber < weeks[j].WeekNumber
		}
		return weeks[i].SortOrder < weeks[j].SortOrder
	})

	for _, w := range weeks {
		wf := weekFingerprint{
			WeekNumber: w.WeekNumber,
			SortOrder:  w.SortOrder,
			Days:       make([]dayFingerprint, 0, len(w.Days)),
		}
		days := append([]Day(nil), w.Days...)
		sort.Slice(days, func(i, j int) bool {
			if days[i].SortOrder == days[j].SortOrder {
				return days[i].DayNumber < days[j].DayNumber
			}
			return days[i].SortOrder < days[j].SortOrder
		})
		for _, day := range days {
			df := dayFingerprint{
				DayNumber: day.DayNumber,
				SortOrder: day.SortOrder,
				Blocks:    make([]blockFingerprint, 0, len(day.Blocks)),
			}
			blocks := append([]DayBlock(nil), day.Blocks...)
			sort.Slice(blocks, func(i, j int) bool {
				return blocks[i].SortOrder < blocks[j].SortOrder
			})
			for _, block := range blocks {
				bf := blockFingerprint{
					BlockType:   string(block.BlockType),
					Instruction: block.Instruction,
					SortOrder:   block.SortOrder,
					Exercises:   make([]exerciseFingerprint, 0, len(block.Exercises)),
				}
				exercises := append([]DayExercise(nil), block.Exercises...)
				sort.Slice(exercises, func(i, j int) bool {
					return exercises[i].SortOrder < exercises[j].SortOrder
				})
				for _, ex := range exercises {
					bf.Exercises = append(bf.Exercises, exerciseFingerprint{
						ExerciseID:  ex.ExerciseID.String(),
						SortOrder:   ex.SortOrder,
						Sets:        ex.Sets,
						Reps:        ex.Reps,
						Instruction: ex.Instruction,
					})
				}
				df.Blocks = append(df.Blocks, bf)
			}
			wf.Days = append(wf.Days, df)
		}
		fp.Weeks = append(fp.Weeks, wf)
	}

	raw, err := json.Marshal(fp)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
