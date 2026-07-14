package telegrambot

import (
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

func dayHasExercises(day program.Day) bool {
	for _, block := range day.Blocks {
		if len(block.Exercises) > 0 {
			return true
		}
	}
	return false
}

func weekHasSelectableDays(week program.Week) bool {
	for _, day := range week.Days {
		if dayHasExercises(day) {
			return true
		}
	}
	return false
}

func weekFullyCompleted(week program.Week, completed map[uuid.UUID]struct{}) bool {
	if len(completed) == 0 {
		return false
	}
	any := false
	for _, day := range week.Days {
		if !dayHasExercises(day) {
			continue
		}
		any = true
		if _, ok := completed[day.DayKey]; !ok {
			return false
		}
	}
	return any
}

func countSelectableWeeks(weeks []program.Week) int {
	n := 0
	for _, w := range weeks {
		if weekHasSelectableDays(w) {
			n++
		}
	}
	return n
}

func countSelectableDays(weeks []program.Week) int {
	n := 0
	for _, w := range weeks {
		for _, d := range w.Days {
			if dayHasExercises(d) {
				n++
			}
		}
	}
	return n
}

func findWeek(weeks []program.Week, weekNumber int) (*program.Week, bool) {
	for i := range weeks {
		if weeks[i].WeekNumber == weekNumber {
			return &weeks[i], true
		}
	}
	return nil, false
}

func findDay(days []program.Day, dayNumber int) (*program.Day, bool) {
	for i := range days {
		if days[i].DayNumber == dayNumber {
			return &days[i], true
		}
	}
	return nil, false
}

func nextSelectableDayInWeek(week program.Week, currentDayNumber int) (int, bool) {
	foundCurrent := false
	for _, d := range week.Days {
		if d.DayNumber == currentDayNumber {
			foundCurrent = true
			continue
		}
		if foundCurrent && dayHasExercises(d) {
			return d.DayNumber, true
		}
	}
	return 0, false
}

func nextSelectableWeek(weeks []program.Week, currentWeekNumber int) (int, bool) {
	foundCurrent := false
	for _, w := range weeks {
		if w.WeekNumber == currentWeekNumber {
			foundCurrent = true
			continue
		}
		if foundCurrent && weekHasSelectableDays(w) {
			return w.WeekNumber, true
		}
	}
	return 0, false
}
