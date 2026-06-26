package pgconv

import (
	"github.com/jackc/pgx/v5/pgtype"
)

func ToNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(*f)
	return n
}

func FromNumeric(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	fv, err := n.Float64Value()
	if err != nil || !fv.Valid {
		return nil
	}
	f := fv.Float64
	return &f
}
