package program

import (
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

func TestDetailFingerprint_stableForSameContent(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets, reps := "3", "10"
	exerciseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	d := Detail{
		Program: Program{
			Name:       "Program",
			Category:   &category,
			Difficulty: (*Difficulty)(&difficulty),
		},
		Weeks: []Week{{
			WeekNumber: 1,
			SortOrder:  1,
			Days: []Day{singleBlockDay(DayExercise{
				ExerciseID: exerciseID,
				SortOrder:  1,
				Sets:       &sets,
				Reps:       &reps,
			})},
		}},
	}

	fp1, err := DetailFingerprint(d)
	if err != nil {
		t.Fatalf("DetailFingerprint: %v", err)
	}
	fp2, err := DetailFingerprint(d)
	if err != nil {
		t.Fatalf("DetailFingerprint: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprints differ for identical detail")
	}
}

func TestDetailFingerprint_changesWhenNameChanges(t *testing.T) {
	d1 := Detail{Program: Program{Name: "A"}}
	d2 := Detail{Program: Program{Name: "B"}}
	fp1, err := DetailFingerprint(d1)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := DetailFingerprint(d2)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatal("expected different fingerprints")
	}
}

func fingerprintFixture(block DayBlock) Detail {
	return Detail{
		Program: Program{Name: "Program"},
		Weeks: []Week{{
			WeekNumber: 1,
			SortOrder:  1,
			Days:       []Day{{DayNumber: 1, SortOrder: 1, Blocks: []DayBlock{block}}},
		}},
	}
}

// The expected value was computed before the block settings existed: a block without
// settings must keep its hash, or every published program would look edited.
func TestDetailFingerprint_blockWithoutSettingsKeepsLegacyHash(t *testing.T) {
	reps := "10"
	d := fingerprintFixture(DayBlock{
		BlockType:   BlockTypeEMOM,
		Instruction: "go",
		SortOrder:   1,
		Exercises: []DayExercise{{
			ExerciseID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			SortOrder:  1,
			Reps:       &reps,
		}},
	})
	got, err := DetailFingerprint(d)
	if err != nil {
		t.Fatal(err)
	}
	const want = "b93c9aa3c9782f073e2ebfca80c6a77e5f6f39af0fbc9e559e35d73405247da0"
	if got != want {
		t.Fatalf("fingerprint = %s, want %s", got, want)
	}
}

func TestDetailFingerprint_changesWhenBlockSettingChanges(t *testing.T) {
	ten, eleven := 10, 11
	base := DayBlock{BlockType: BlockTypeEMOM, SortOrder: 1}
	baseFP, err := DetailFingerprint(fingerprintFixture(base))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		apply func(*DayBlock, *int)
	}{
		{"rounds", func(b *DayBlock, v *int) { b.Rounds = v }},
		{"duration_sec", func(b *DayBlock, v *int) { b.DurationSec = v }},
		{"time_cap_sec", func(b *DayBlock, v *int) { b.TimeCapSec = v }},
		{"work_sec", func(b *DayBlock, v *int) { b.WorkSec = v }},
		{"rest_sec", func(b *DayBlock, v *int) { b.RestSec = v }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTen, withEleven := base, base
			tt.apply(&withTen, &ten)
			tt.apply(&withEleven, &eleven)
			fpTen, err := DetailFingerprint(fingerprintFixture(withTen))
			if err != nil {
				t.Fatal(err)
			}
			fpEleven, err := DetailFingerprint(fingerprintFixture(withEleven))
			if err != nil {
				t.Fatal(err)
			}
			if fpTen == baseFP {
				t.Fatal("setting a value must change the fingerprint")
			}
			if fpTen == fpEleven {
				t.Fatal("changing the value must change the fingerprint")
			}
		})
	}
}

func TestDetailFingerprint_zeroRestSecDiffersFromUnset(t *testing.T) {
	zero := 0
	unset, err := DetailFingerprint(fingerprintFixture(DayBlock{BlockType: BlockTypeIntervals, SortOrder: 1}))
	if err != nil {
		t.Fatal(err)
	}
	withZero, err := DetailFingerprint(fingerprintFixture(DayBlock{BlockType: BlockTypeIntervals, SortOrder: 1, RestSec: &zero}))
	if err != nil {
		t.Fatal(err)
	}
	if unset == withZero {
		t.Fatal("rest_sec 0 must not hash like an unset value")
	}
}
