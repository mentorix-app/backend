package analytics

import "time"

// weekStreak counts consecutive calendar weeks (Mon-Sun, UTC) with at least
// one completion. weekStarts must be sorted newest first. The streak is alive
// if the newest active week is the current week or the one before it (the
// current week is not counted as broken while it is still in progress).
func weekStreak(weekStarts []time.Time, now time.Time) int {
	if len(weekStarts) == 0 {
		return 0
	}
	currentWeek := startOfWeek(now.UTC())
	latest := startOfWeek(weekStarts[0].UTC())
	if latest.Before(currentWeek.AddDate(0, 0, -7)) {
		return 0
	}

	streak := 1
	prev := latest
	for _, ws := range weekStarts[1:] {
		week := startOfWeek(ws.UTC())
		if !week.Equal(prev.AddDate(0, 0, -7)) {
			break
		}
		streak++
		prev = week
	}
	return streak
}

func startOfWeek(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday belongs to the week started the previous Monday
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -(weekday - 1))
}
