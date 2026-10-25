package program

import (
	"context"
	"errors"
	"fmt"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// RefreshVersionFingerprints makes every stored version fingerprint match its stored tree
// and returns how many it rewrote. It is safe to run repeatedly and while the app serves;
// a version deleted while it runs is skipped. A fingerprint is written once at publish, so
// a release that adds a field to DetailFingerprint would otherwise flag every published
// program as changed.
func (s *Store) RefreshVersionFingerprints(ctx context.Context) (int, error) {
	versions, err := s.q.ListProgramVersionFingerprints(ctx)
	if err != nil {
		return 0, fmt.Errorf("list program versions: %w", err)
	}
	updated := 0
	for _, v := range versions {
		tree, err := s.getVersionDetail(ctx, s.q, pgconv.FromPGUUID(v.ID))
		if errors.Is(err, ErrNotFound) {
			// Deleted after the listing: nothing left to refresh.
			continue
		}
		if err != nil {
			return updated, fmt.Errorf("load version %s: %w", pgconv.FromPGUUID(v.ID), err)
		}
		fp, err := DetailFingerprint(tree)
		if err != nil {
			return updated, fmt.Errorf("fingerprint version %s: %w", pgconv.FromPGUUID(v.ID), err)
		}
		if fp == v.ContentFingerprint {
			continue
		}
		if err := s.q.UpdateProgramVersionFingerprint(ctx, sqlc.UpdateProgramVersionFingerprintParams{
			ID:                 v.ID,
			ContentFingerprint: fp,
		}); err != nil {
			return updated, fmt.Errorf("update version %s fingerprint: %w", pgconv.FromPGUUID(v.ID), err)
		}
		updated++
	}
	return updated, nil
}
