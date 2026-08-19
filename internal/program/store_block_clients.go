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
	// current and ensureDaysKeepSharedBlock below read through qtx — this
	// transaction's own connection — rather than the pool. That is required,
	// not just correct: under READ COMMITTED a plain pool read after the lock
	// is granted would already see everything a competing transaction
	// committed before releasing the lock, so a second connection would still
	// be correct, but every concurrent PUT on this program would then hold two
	// connections at once (one blocked on FOR UPDATE, one for the read) for as
	// long as it holds the lock. With MaxConns capped at max(4, NumCPU), a
	// handful of concurrent requests on the same program exhausts the pool and
	// everything stalls on a fifth connection that never comes, while the
	// program row stays locked the whole time. Reading in-transaction keeps
	// each request to a single connection.
	if err := qtx.LockProgramForUpdate(ctx, pgconv.ToPGUUID(programID)); err != nil {
		return Detail{}, fmt.Errorf("lock program: %w", err)
	}

	current, err := s.getDetail(ctx, qtx, programID)
	if err != nil {
		return Detail{}, err
	}
	wasShared := blockCurrentlyShared(current, blockKey)
	becomesRestricted := len(clientUserIDs) > 0

	// Only the shared → restricted transition can break the day invariant.
	if wasShared && becomesRestricted {
		if err := s.ensureDaysKeepSharedBlock(ctx, qtx, programID, blockKey, current); err != nil {
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

// ensureDaysKeepSharedBlock checks the day invariant in the working copy and
// in every version that matters for who could see this block next: every
// version an active assignment still points at, plus the latest version,
// deduplicated. The latest version is included even with no assignment on it
// yet — SetClientProgramAssignment and SyncProgramAssignments both put a
// newly (re)assigned client on GetLatestProgramVersionByProgramID, not on the
// working copy, so when the working copy has unpublished changes, the latest
// version is exactly where the next assignment lands, and it must already
// satisfy the invariant. Takes the queries object explicitly so it reads
// through qtx, after the program lock, not through the unlocked pool.
func (s *Store) ensureDaysKeepSharedBlock(ctx context.Context, q *sqlc.Queries, programID, blockKey uuid.UUID, working Detail) error {
	trees := []struct {
		label  string
		detail Detail
	}{{label: "working copy", detail: working}}

	// ListAssignedProgramVersionIDs is a SELECT DISTINCT, so assignedRows never
	// contains duplicates on its own; seen only needs to catch the latest
	// version below when it coincides with one already in this list.
	assignedRows, err := q.ListAssignedProgramVersionIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list assigned versions: %w", err)
	}
	versionIDs := make([]uuid.UUID, 0, len(assignedRows)+1)
	seen := make(map[uuid.UUID]struct{}, len(assignedRows)+1)
	for _, row := range assignedRows {
		id := pgconv.FromPGUUID(row)
		seen[id] = struct{}{}
		versionIDs = append(versionIDs, id)
	}

	// A draft program with no published version yet has nothing more to
	// check: GetLatestProgramVersionByProgramID's ErrNoRows is expected, not
	// a failure.
	latest, err := q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("latest program version: %w", err)
	}
	if err == nil {
		id := pgconv.FromPGUUID(latest.ID)
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			versionIDs = append(versionIDs, id)
		}
	}

	for _, versionID := range versionIDs {
		detail, err := s.getVersionDetail(ctx, q, versionID)
		if err != nil {
			return err
		}
		// getVersionDetail already applies the program's rules (Task 3).
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
