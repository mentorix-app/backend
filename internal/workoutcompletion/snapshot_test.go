package workoutcompletion

import (
	"encoding/json"
	"testing"

	"mentorix-backend/internal/program"
)

func intPtr(v int) *int { return &v }

func decodeSnapshot(t *testing.T, raw json.RawMessage) []map[string]any {
	t.Helper()
	var out struct {
		Blocks []map[string]any `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Blocks
}

func TestBuildDaySnapshot_carriesBlockSettings(t *testing.T) {
	raw, err := buildDaySnapshot(program.Day{Blocks: []program.DayBlock{{
		BlockType:   program.BlockTypeIntervals,
		Rounds:      intPtr(6),
		WorkSec:     intPtr(40),
		RestSec:     intPtr(0),
		DurationSec: nil,
		TimeCapSec:  nil,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	blocks := decodeSnapshot(t, raw)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	b := blocks[0]
	for key, want := range map[string]float64{"rounds": 6, "work_sec": 40, "rest_sec": 0} {
		if got, ok := b[key].(float64); !ok || got != want {
			t.Fatalf("%s = %v, want %v", key, b[key], want)
		}
	}
	for _, key := range []string{"duration_sec", "time_cap_sec"} {
		if _, ok := b[key]; ok {
			t.Fatalf("%s must be omitted when unset, got %v", key, b[key])
		}
	}
}

func TestBuildDaySnapshot_blockWithoutSettingsKeepsItsShape(t *testing.T) {
	sets := "3"
	raw, err := buildDaySnapshot(program.Day{Blocks: []program.DayBlock{{
		BlockType: program.BlockTypeStrength,
		Exercises: []program.DayExercise{{ExerciseName: "Squat", Sets: &sets}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"blocks":[{"block_type":"strength","exercises":[{"name":"Squat","name_ru":"","sets":"3"}]}]}`
	if string(raw) != want {
		t.Fatalf("snapshot = %s, want %s", raw, want)
	}
}
