package pgconv

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDRoundTrip(t *testing.T) {
	id := uuid.New()
	pg := ToPGUUID(id)
	if got := FromPGUUID(pg); got != id {
		t.Errorf("FromPGUUID() = %v, want %v", got, id)
	}
}

func TestFromPGUUID_invalid(t *testing.T) {
	if got := FromPGUUID(pgtype.UUID{}); got != uuid.Nil {
		t.Errorf("invalid uuid = %v, want Nil", got)
	}
	if got := FromPGUUID(pgtype.UUID{Valid: true, Bytes: [16]byte{}}); got != uuid.Nil {
		t.Errorf("bad bytes = %v, want Nil", got)
	}
}

func TestUUIDSlice(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	got := UUIDSlice(ids)
	if len(got) != len(ids) {
		t.Fatalf("len = %d, want %d", len(got), len(ids))
	}
	for i, pg := range got {
		if FromPGUUID(pg) != ids[i] {
			t.Errorf("index %d mismatch", i)
		}
	}
}

func TestNumericRoundTrip(t *testing.T) {
	f := 12.5
	pg := ToNumeric(&f)
	if !pg.Valid {
		t.Fatal("expected valid numeric")
	}
	out := FromNumeric(pg)
	if out == nil || *out != f {
		t.Fatalf("FromNumeric() = %v, want %v", out, f)
	}
}

func TestNumeric_nil(t *testing.T) {
	if got := FromNumeric(ToNumeric(nil)); got != nil {
		t.Errorf("nil numeric = %v, want nil", got)
	}
	if got := FromNumeric(pgtype.Numeric{}); got != nil {
		t.Errorf("invalid numeric = %v, want nil", got)
	}
}
