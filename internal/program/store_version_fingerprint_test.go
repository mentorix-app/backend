package program

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/sqlc"
)

// missingVersionsDB lists two versions and then reports each as gone, as if both were
// deleted between the listing and the load.
type missingVersionsDB struct {
	ids       []pgtype.UUID
	loadCalls int
}

func (d *missingVersionsDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (d *missingVersionsDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return &versionListRows{ids: d.ids, pos: -1}, nil
}

func (d *missingVersionsDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	d.loadCalls++
	return noRow{}
}

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

type versionListRows struct {
	ids []pgtype.UUID
	pos int
}

func (r *versionListRows) Close()                                       {}
func (r *versionListRows) Err() error                                   { return nil }
func (r *versionListRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *versionListRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *versionListRows) Values() ([]any, error)                       { return nil, nil }
func (r *versionListRows) RawValues() [][]byte                          { return nil }
func (r *versionListRows) Conn() *pgx.Conn                              { return nil }

func (r *versionListRows) Next() bool {
	r.pos++
	return r.pos < len(r.ids)
}

func (r *versionListRows) Scan(dest ...any) error {
	*dest[0].(*pgtype.UUID) = r.ids[r.pos]
	*dest[1].(*string) = "stale"
	return nil
}

func TestRefreshVersionFingerprints_skipsVersionDeletedAfterListing(t *testing.T) {
	db := &missingVersionsDB{ids: []pgtype.UUID{
		{Bytes: [16]byte{1}, Valid: true},
		{Bytes: [16]byte{2}, Valid: true},
	}}
	s := &Store{q: sqlc.New(db)}

	updated, err := s.RefreshVersionFingerprints(context.Background())
	if err != nil {
		t.Fatalf("RefreshVersionFingerprints: %v, want a missing version skipped", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
	if db.loadCalls != 2 {
		t.Fatalf("load attempts = %d, want 2: the run must go on past a missing version", db.loadCalls)
	}
}
