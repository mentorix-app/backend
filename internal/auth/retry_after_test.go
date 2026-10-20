package auth

import (
	"testing"
	"time"
)

func TestRetryAfterSeconds(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"rounds up", 1500 * time.Millisecond, "2"},
		{"sub-second is one", time.Millisecond, "1"},
		{"zero is one", 0, "1"},
		{"negative is one", -5 * time.Second, "1"},
		{"whole seconds stay", 60 * time.Second, "60"},
		{"just over a second", time.Second + time.Nanosecond, "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryAfterSeconds(tt.in); got != tt.want {
				t.Errorf("retryAfterSeconds(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
