package pgconv

import (
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func ToNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(*f, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}
	}
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
