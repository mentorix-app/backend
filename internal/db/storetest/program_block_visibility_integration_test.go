//go:build integration

package storetest

import (
	"context"
	"strings"
	"testing"
)

func TestMigration_BlockKeyColumnsExist(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	// Both block_key columns must be NOT NULL *and* carry a generated default:
	// the existing insert queries do not mention block_key until Tasks 2 and 3,
	// so without the default this migration breaks every block insert.
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		var isNullable string
		var columnDefault *string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = 'mentorix' AND table_name = $1 AND column_name = 'block_key'`,
			table).Scan(&isNullable, &columnDefault)
		if err != nil {
			t.Fatalf("query %s.block_key: %v", table, err)
		}
		if isNullable != "NO" {
			t.Fatalf("%s.block_key is_nullable = %q, want NO", table, isNullable)
		}
		if columnDefault == nil || !strings.Contains(*columnDefault, "gen_random_uuid") {
			t.Fatalf("%s.block_key default = %v, want gen_random_uuid()", table, columnDefault)
		}
	}

	var hasTable bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'mentorix' AND table_name = 'program_block_clients')`).Scan(&hasTable)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if !hasTable {
		t.Fatal("table mentorix.program_block_clients does not exist")
	}

	var hasUniq bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint
			WHERE conname = 'program_block_clients_program_block_key_client_uniq')`).Scan(&hasUniq)
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	if !hasUniq {
		t.Fatal("unique constraint on program_block_clients is missing")
	}
}
