package telegrambot

import "mentorix-backend/internal/program"

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
