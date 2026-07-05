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
