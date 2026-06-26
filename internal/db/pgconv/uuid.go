package pgconv

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func ToPGUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func FromPGUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	u, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return uuid.Nil
	}
	return u
}

func UUIDSlice(ids []uuid.UUID) []pgtype.UUID {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		out[i] = ToPGUUID(id)
	}
	return out
}
