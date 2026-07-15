package trainerclient

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestTimePtrFromAny(t *testing.T) {
	if timePtrFromAny(nil) != nil {
		t.Fatal("nil -> nil")
	}
	if timePtrFromAny(time.Time{}) != nil {
		t.Fatal("zero time -> nil")
	}

	raw := time.Date(2026, 7, 14, 12, 0, 0, 0, time.FixedZone("X", 5*3600))
	got := timePtrFromAny(raw)
	if got == nil || !got.Equal(raw.UTC()) {
		t.Fatalf("time.Time = %v", got)
	}

	ptr := &raw
	got = timePtrFromAny(ptr)
	if got == nil || !got.Equal(raw.UTC()) {
		t.Fatalf("*time.Time = %v", got)
	}

	got = timePtrFromAny(pgtype.Timestamptz{Time: raw, Valid: true})
	if got == nil || !got.Equal(raw.UTC()) {
		t.Fatalf("pgtype valid = %v", got)
	}
	if timePtrFromAny(pgtype.Timestamptz{}) != nil {
		t.Fatal("pgtype invalid -> nil")
	}
}
