package program

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// SetBlockClients replaces the visibility rules of one block. An empty
// clientUserIDs makes the block shared again. Restricting a block is refused
// when it would leave a day without a shared block — checked against the
// working copy and against every version that still has an active assignment.
func (s *Store) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error) {
	ok, err := s.blockBelongsToWeek(ctx, programID, weekID, blockID)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, pgx.ErrNoRows
	}

	blockRow, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, pgx.ErrNoRows
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	blockKey := pgconv.FromPGUUID(blockRow.BlockKey)

	if err := s.ensureClientsAssigned(ctx, programID, clientUserIDs); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	// Serialize with any other SetBlockClients call on this program: the day
	// invariant reads sibling blocks that a concurrent request can restrict at
	// the same time, so without a lock two requests can each see the other's
	// block as still shared, both pass the check, and both commit — leaving the
	// day with zero shared blocks. This row lock is what makes the two calls
	// serialize instead of racing.
	//
	// GetDetail and ensureDaysKeepSharedBlock below read through s.q — the
	// pool, not qtx/this transaction — on purpose, not by oversight. Under
	// READ COMMITTED, a plain read after the lock is granted already sees
	// everything a competing transaction committed before releasing the lock
	// (PostgreSQL blocks our FOR UPDATE until it does), so the reads are
	// correctly serialized by the lock even though they don't run inside qtx.
	// Do not "fix" this by moving them onto qtx: that would just make an
	// already-correct read part of the same transaction for no benefit.
	if err := qtx.LockProgramForUpdate(ctx, pgconv.ToPGUUID(programID)); err != nil {
		return Detail{}, fmt.Errorf("lock program: %w", err)
	}

	current, err := s.GetDetail(ctx, programID)
	if err != nil {
		return Detail{}, err
	}
	wasShared := blockCurrentlyShared(current, blockKey)
	becomesRestricted := len(clientUserIDs) > 0

	// Only the shared → restricted transition can break the day invariant.
	if wasShared && becomesRestricted {
		if err := s.ensureDaysKeepSharedBlock(ctx, programID, blockKey, current); err != nil {
			return Detail{}, err
		}
	}

	if err := qtx.DeleteProgramBlockClients(ctx, sqlc.DeleteProgramBlockClientsParams{
		ProgramID: pgconv.ToPGUUID(programID),
		BlockKey:  pgconv.ToPGUUID(blockKey),
	}); err != nil {
		return Detail{}, fmt.Errorf("delete block clients: %w", err)
	}
	for _, clientUserID := range clientUserIDs {
		if err := qtx.InsertProgramBlockClient(ctx, sqlc.InsertProgramBlockClientParams{
			ProgramID:    pgconv.ToPGUUID(programID),
			BlockKey:     pgconv.ToPGUUID(blockKey),
			ClientUserID: pgconv.ToPGUUID(clientUserID),
			CreatedBy:    pgconv.ToPGUUID(userID),
		}); err != nil {
			return Detail{}, fmt.Errorf("insert block client: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func blockCurrentlyShared(d Detail, blockKey uuid.UUID) bool {
	for _, week := range d.Weeks {
		for _, day := range week.Days {
			for _, block := range day.Blocks {
				if block.BlockKey == blockKey {
					return len(block.ClientUserIDs) == 0
				}
			}
		}
	}
	return true
}

func (s *Store) ensureClientsAssigned(ctx context.Context, programID uuid.UUID, clientUserIDs []uuid.UUID) error {
	if len(clientUserIDs) == 0 {
		return nil
	}
	rows, err := s.q.ListAssignedClientUserIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list assigned clients: %w", err)
	}
	assigned := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		assigned[pgconv.FromPGUUID(row)] = struct{}{}
	}
	for _, id := range clientUserIDs {
		if _, ok := assigned[id]; !ok {
			return fmt.Errorf("%w: %s", ErrClientNotAssignedToProgram, id)
		}
	}
	return nil
}

// ensureDaysKeepSharedBlock checks the day invariant in the working copy and in
// every version an active assignment still points at.
func (s *Store) ensureDaysKeepSharedBlock(ctx context.Context, programID, blockKey uuid.UUID, working Detail) error {
	trees := []struct {
		label  string
		detail Detail
	}{{label: "working copy", detail: working}}

	versionIDs, err := s.q.ListAssignedProgramVersionIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list assigned versions: %w", err)
	}
	for _, versionPG := range versionIDs {
		versionID := pgconv.FromPGUUID(versionPG)
		detail, err := s.GetVersionDetail(ctx, versionID)
		if err != nil {
			return err
		}
		// GetVersionDetail already applies the program's rules (Task 3).
		trees = append(trees, struct {
			label  string
			detail Detail
		}{label: "version " + versionID.String(), detail: detail})
	}

	for _, tree := range trees {
		for _, week := range tree.detail.Weeks {
			for _, day := range week.Days {
				if !daySharedAfterRestrict(day, blockKey) {
					return fmt.Errorf("%w: week %d day %d in %s",
						ErrLastSharedBlock, week.WeekNumber, day.DayNumber, tree.label)
				}
			}
		}
	}
	return nil
}
