package program

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
