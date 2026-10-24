//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

// publishedTwice publishes a program with an emom group (settings set) and a single, then
// freezes a second version with one more block. Returns the fixture and both version ids.
func publishedTwice(t *testing.T, prefix string) (*settingsFixture, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	f := newSettingsFixture(t, prefix)
	group := f.addGroup(t)
	if _, err := f.patch(group.ID, program.BlockPatchInput{
		BlockType:   blockType(program.BlockTypeEMOM),
		Instruction: strPtr("every minute"),
		Rounds:      setInt(6),
	}); err != nil {
		t.Fatalf("patch group: %v", err)
	}
	f.addSingle(t, "3", "10")

	if _, err := f.store.PublishFromDraft(ctx, f.programID, f.trainer, f.detail(t)); err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	f.addSingle(t, "5", "5")
	if _, err := f.store.FreezePublishedVersion(ctx, f.programID, f.trainer, f.detail(t)); err != nil {
		t.Fatalf("FreezePublishedVersion: %v", err)
	}

	rows, err := f.pool.Query(ctx, `SELECT id FROM mentorix.program_versions WHERE program_id = $1 ORDER BY version_number`, f.programID)
	if err != nil {
		t.Fatalf("select versions: %v", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan version id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("versions = %d, want 2", len(ids))
	}
	return f, ids
}

func TestRefreshVersionFingerprints_noChangeRightAfterPublish(t *testing.T) {
	f, _ := publishedTwice(t, "fp-fresh")
	ctx := context.Background()

	n, err := f.store.RefreshVersionFingerprints(ctx)
	if err != nil {
		t.Fatalf("RefreshVersionFingerprints: %v", err)
	}
	if n != 0 {
		t.Fatalf("updated = %d, want 0: the version loader must reproduce the fingerprint written at publish", n)
	}
	if f.detail(t).HasUnpublishedChanges {
		t.Fatal("has_unpublished_changes = true right after publish")
	}
}

func TestRefreshVersionFingerprints_repairsStoredValues(t *testing.T) {
	f, versionIDs := publishedTwice(t, "fp-repair")
	ctx := context.Background()

	if _, err := f.pool.Exec(ctx, `UPDATE mentorix.program_versions SET content_fingerprint = 'stale'`); err != nil {
		t.Fatalf("corrupt fingerprints: %v", err)
	}
	if !f.detail(t).HasUnpublishedChanges {
		t.Fatal("has_unpublished_changes = false with a stale fingerprint, want true")
	}

	n, err := f.store.RefreshVersionFingerprints(ctx)
	if err != nil {
		t.Fatalf("RefreshVersionFingerprints: %v", err)
	}
	if n != len(versionIDs) {
		t.Fatalf("updated = %d, want %d", n, len(versionIDs))
	}
	for _, id := range versionIDs {
		tree, err := f.store.GetVersionDetail(ctx, id)
		if err != nil {
			t.Fatalf("GetVersionDetail: %v", err)
		}
		want, err := program.DetailFingerprint(tree)
		if err != nil {
			t.Fatalf("DetailFingerprint: %v", err)
		}
		var got string
		if err := f.pool.QueryRow(ctx, `SELECT content_fingerprint FROM mentorix.program_versions WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("select fingerprint: %v", err)
		}
		if got != want {
			t.Fatalf("version %s fingerprint = %q, want %q", id, got, want)
		}
	}
	// The draft equals the latest version, so the flag clears once the value is right.
	if f.detail(t).HasUnpublishedChanges {
		t.Fatal("has_unpublished_changes = true after the refresh, want false")
	}

	n, err = f.store.RefreshVersionFingerprints(ctx)
	if err != nil {
		t.Fatalf("second RefreshVersionFingerprints: %v", err)
	}
	if n != 0 {
		t.Fatalf("second run updated = %d, want 0", n)
	}
}
