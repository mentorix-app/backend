package program

import "time"

type FlatDay struct {
	WeekNumber int
	DayNumber  int
	Day        Day
}

func FlattenDays(weeks []Week) []FlatDay {
	out := make([]FlatDay, 0)
	for _, week := range weeks {
		for _, day := range week.Days {
			out = append(out, FlatDay{
				WeekNumber: week.WeekNumber,
				DayNumber:  day.DayNumber,
				Day:        day,
			})
		}
	}
	return out
}

func DayOffsetFromAssignment(assignedAt, now time.Time) int {
	start := assignedAt.UTC().Truncate(24 * time.Hour)
	today := now.UTC().Truncate(24 * time.Hour)
	days := int(today.Sub(start).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func IsRestDay(day Day) bool {
	for _, block := range day.Blocks {
		if len(block.Exercises) > 0 {
			return false
		}
	}
	return true
}

// IsTrainingDay is true when the day has at least one block or at least one exercise in any block.
func IsTrainingDay(day Day) bool {
	if len(day.Blocks) == 0 {
		return false
	}
	for _, block := range day.Blocks {
		if len(block.Exercises) > 0 {
			return true
		}
	}
	return len(day.Blocks) > 0
}

func CountTrainingDays(weeks []Week) int {
	n := 0
	for _, week := range weeks {
		for _, day := range week.Days {
			if IsTrainingDay(day) {
				n++
			}
		}
	}
	return n
}

func TodayFlatDay(assignedAt, now time.Time, weeks []Week) (FlatDay, int, bool) {
	flat := FlattenDays(weeks)
	if len(flat) == 0 {
		return FlatDay{}, 0, false
	}
	offset := DayOffsetFromAssignment(assignedAt, now)
	if offset >= len(flat) {
		offset = len(flat) - 1
	}
	return flat[offset], offset + 1, true
}
