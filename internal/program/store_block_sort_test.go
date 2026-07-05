package program

import (
	"testing"

	"github.com/google/uuid"
)

func TestInsertUUIDAt(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	got := insertUUIDAt([]uuid.UUID{a, c}, b, 2)
	if len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Fatalf("insertUUIDAt() = %v", got)
	}
}

func TestClampInsertPosition(t *testing.T) {
	if clampInsertPosition(0, 3) != 3 {
		t.Fatal("zero should append")
	}
	if clampInsertPosition(2, 3) != 2 {
		t.Fatal("valid position should stay")
	}
	if clampInsertPosition(9, 3) != 3 {
		t.Fatal("overflow should clamp")
	}
}

func TestSortProgramDetail_ordersBySortOrder(t *testing.T) {
	w1, w2 := uuid.New(), uuid.New()
	d1, d2 := uuid.New(), uuid.New()
	b1, b2 := uuid.New(), uuid.New()
	e1, e2 := uuid.New(), uuid.New()

	d := Detail{
		Weeks: []Week{
			{ID: w2, SortOrder: 2, Days: []Day{
				{ID: d2, SortOrder: 2, Blocks: []DayBlock{
					{ID: b2, SortOrder: 2, Exercises: []DayExercise{
						{ID: e2, SortOrder: 2},
						{ID: e1, SortOrder: 1},
					}},
					{ID: b1, SortOrder: 1, Exercises: nil},
				}},
				{ID: d1, SortOrder: 1, Blocks: nil},
			}},
			{ID: w1, SortOrder: 1, Days: nil},
		},
	}

	sortProgramDetail(&d)

	if d.Weeks[0].ID != w1 || d.Weeks[1].ID != w2 {
		t.Fatalf("weeks order = %v, %v", d.Weeks[0].ID, d.Weeks[1].ID)
	}
	if d.Weeks[1].Days[0].ID != d1 {
		t.Fatalf("days order = %v", d.Weeks[1].Days[0].ID)
	}
	if d.Weeks[1].Days[1].Blocks[0].ID != b1 {
		t.Fatalf("blocks order = %v", d.Weeks[1].Days[1].Blocks[0].ID)
	}
	ex := d.Weeks[1].Days[1].Blocks[1].Exercises
	if len(ex) != 2 || ex[0].ID != e1 || ex[1].ID != e2 {
		t.Fatalf("exercises order = %v", ex)
	}
}
