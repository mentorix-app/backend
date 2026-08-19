package program

import (
	"testing"

	"github.com/google/uuid"
)

func TestBlockVisibleToClient(t *testing.T) {
	petya := uuid.New()
	masha := uuid.New()
	vasya := uuid.New()

	tests := []struct {
		name   string
		block  DayBlock
		client uuid.UUID
		want   bool
	}{
		{
			name:   "shared block is visible to anyone",
			block:  DayBlock{ClientUserIDs: nil},
			client: vasya,
			want:   true,
		},
		{
			name:   "restricted block is visible to a listed client",
			block:  DayBlock{ClientUserIDs: []uuid.UUID{petya, masha}},
			client: petya,
			want:   true,
		},
		{
			name:   "restricted block is hidden from an unlisted client",
			block:  DayBlock{ClientUserIDs: []uuid.UUID{petya, masha}},
			client: vasya,
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BlockVisibleToClient(tc.block, tc.client); got != tc.want {
				t.Fatalf("BlockVisibleToClient() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterDetailForClient_dropsHiddenBlocks(t *testing.T) {
	petya := uuid.New()
	vasya := uuid.New()
	shared := DayBlock{ID: uuid.New(), BlockKey: uuid.New()}
	personal := DayBlock{ID: uuid.New(), BlockKey: uuid.New(), ClientUserIDs: []uuid.UUID{petya}}

	d := Detail{Weeks: []Week{{
		WeekNumber: 1,
		Days:       []Day{{DayNumber: 1, Blocks: []DayBlock{shared, personal}}},
	}}}

	forPetya := FilterDetailForClient(d, petya)
	if got := len(forPetya.Weeks[0].Days[0].Blocks); got != 2 {
		t.Fatalf("blocks for listed client = %d, want 2", got)
	}

	forVasya := FilterDetailForClient(d, vasya)
	if got := len(forVasya.Weeks[0].Days[0].Blocks); got != 1 {
		t.Fatalf("blocks for unlisted client = %d, want 1", got)
	}
	if forVasya.Weeks[0].Days[0].Blocks[0].ID != shared.ID {
		t.Fatal("unlisted client kept the wrong block")
	}

	// The source detail must not be mutated.
	if got := len(d.Weeks[0].Days[0].Blocks); got != 2 {
		t.Fatalf("source detail mutated: blocks = %d, want 2", got)
	}
}

func TestDaySharedAfterRestrict(t *testing.T) {
	petya := uuid.New()
	keyA := uuid.New()
	keyB := uuid.New()
	keyC := uuid.New()

	tests := []struct {
		name     string
		day      Day
		restrict uuid.UUID
		want     bool
	}{
		{
			name: "another shared block remains",
			day: Day{Blocks: []DayBlock{
				{BlockKey: keyA},
				{BlockKey: keyB, ClientUserIDs: []uuid.UUID{petya}},
				{BlockKey: keyC},
			}},
			restrict: keyA,
			want:     true,
		},
		{
			name: "restricting the last shared block leaves none",
			day: Day{Blocks: []DayBlock{
				{BlockKey: keyA},
				{BlockKey: keyB, ClientUserIDs: []uuid.UUID{petya}},
			}},
			restrict: keyA,
			want:     false,
		},
		{
			name:     "day without the key is unaffected",
			day:      Day{Blocks: []DayBlock{{BlockKey: keyA}}},
			restrict: keyC,
			want:     true,
		},
		{
			name:     "empty day is unaffected",
			day:      Day{},
			restrict: keyA,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := daySharedAfterRestrict(tc.day, tc.restrict); got != tc.want {
				t.Fatalf("daySharedAfterRestrict() = %v, want %v", got, tc.want)
			}
		})
	}
}
