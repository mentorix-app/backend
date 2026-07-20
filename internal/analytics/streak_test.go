package analytics

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestStartOfWeek(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2026-07-20T10:00:00Z", "2026-07-20T00:00:00Z"}, // Monday
		{"2026-07-22T23:59:59Z", "2026-07-20T00:00:00Z"}, // Wednesday
		{"2026-07-26T00:00:00Z", "2026-07-20T00:00:00Z"}, // Sunday belongs to same week
		{"2026-07-27T00:00:00Z", "2026-07-27T00:00:00Z"}, // next Monday
	}
	for _, tt := range tests {
		got := startOfWeek(mustTime(t, tt.in))
		if !got.Equal(mustTime(t, tt.want)) {
			t.Errorf("startOfWeek(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestWeekStreak(t *testing.T) {
	now := mustTime(t, "2026-07-22T12:00:00Z") // Wednesday, week of Jul 20
	wk := func(s string) time.Time { return mustTime(t, s) }

	tests := []struct {
		name       string
		weekStarts []time.Time
		want       int
	}{
		{"empty", nil, 0},
		{"current week only", []time.Time{wk("2026-07-20T00:00:00Z")}, 1},
		{"previous week keeps streak alive", []time.Time{wk("2026-07-13T00:00:00Z")}, 1},
		{"two weeks ago is broken", []time.Time{wk("2026-07-06T00:00:00Z")}, 0},
		{
			"three consecutive weeks",
			[]time.Time{
				wk("2026-07-20T00:00:00Z"),
				wk("2026-07-13T00:00:00Z"),
				wk("2026-07-06T00:00:00Z"),
			},
			3,
		},
		{
			"gap stops the count",
			[]time.Time{
				wk("2026-07-20T00:00:00Z"),
				wk("2026-07-13T00:00:00Z"),
				wk("2026-06-29T00:00:00Z"),
			},
			2,
		},
		{
			"streak ending last week",
			[]time.Time{
				wk("2026-07-13T00:00:00Z"),
				wk("2026-07-06T00:00:00Z"),
			},
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekStreak(tt.weekStarts, now); got != tt.want {
				t.Errorf("weekStreak = %d, want %d", got, tt.want)
			}
		})
	}
}
