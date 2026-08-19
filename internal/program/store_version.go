package program

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func (s *Store) PublishFromDraft(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	programPG := pgconv.ToPGUUID(id)

	rows, err := qtx.SetProgramStatus(ctx, sqlc.SetProgramStatusParams{
		ID:         programPG,
		Status:     string(StatusPublished),
		ModifiedBy: pgconv.ToPGUUID(userID),
		ModifiedAt: now,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("set program status: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}

	if err := s.freezeVersion(ctx, qtx, d, userID, now); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.enrichedDetail(ctx, id)
}

func (s *Store) FreezePublishedVersion(ctx context.Context, id, userID uuid.UUID, d Detail) (Detail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	if err := s.freezeVersion(ctx, qtx, d, userID, now); err != nil {
		return Detail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.enrichedDetail(ctx, id)
}

// RestoreWorkingTreeFromLatestVersion replaces the editable program tree with the
// latest frozen program_version. Week/day/block/exercise row IDs are regenerated;
// day_key values from the version are preserved.
func (s *Store) RestoreWorkingTreeFromLatestVersion(ctx context.Context, id, userID uuid.UUID) (Detail, error) {
	latest, err := s.q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("latest program version: %w", err)
	}
	versionDetail, err := s.GetVersionDetail(ctx, pgconv.FromPGUUID(latest.ID))
	if err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	now := time.Now().UTC()
	programPG := pgconv.ToPGUUID(id)
	userPG := pgconv.ToPGUUID(userID)

	var category, difficulty *string
	if versionDetail.Category != nil {
		c := string(*versionDetail.Category)
		category = &c
	}
	if versionDetail.Difficulty != nil {
		d := string(*versionDetail.Difficulty)
		difficulty = &d
	}

	rows, err := qtx.ReplaceProgramContent(ctx, sqlc.ReplaceProgramContentParams{
		ID:              programPG,
		ModifiedBy:      userPG,
		ModifiedAt:      now,
		Name:            versionDetail.Name,
		NameRu:          versionDetail.NameRu,
		Description:     versionDetail.Description,
		DescriptionRu:   versionDetail.DescriptionRu,
		Category:        category,
		Difficulty:      difficulty,
		PreviewImageUrl: versionDetail.PreviewImageURL,
	})
	if err != nil {
		return Detail{}, fmt.Errorf("replace program content: %w", err)
	}
	if rows == 0 {
		return Detail{}, pgx.ErrNoRows
	}

	if err := qtx.DeleteProgramWeeksByProgramID(ctx, programPG); err != nil {
		return Detail{}, fmt.Errorf("delete program weeks: %w", err)
	}

	for _, week := range versionDetail.Weeks {
		weekID, err := qtx.InsertProgramWeek(ctx, sqlc.InsertProgramWeekParams{
			ProgramID:  programPG,
			WeekNumber: int32(week.WeekNumber),
			SortOrder:  int32(week.SortOrder),
			ModifiedAt: now,
			ModifiedBy: userPG,
		})
		if err != nil {
			return Detail{}, fmt.Errorf("insert program week: %w", err)
		}
		for _, day := range week.Days {
			dayKey := day.DayKey
			if dayKey == uuid.Nil {
				dayKey = uuid.New()
			}
			dayRow, err := qtx.InsertProgramDayWithKey(ctx, sqlc.InsertProgramDayWithKeyParams{
				ProgramID: programPG,
				WeekID:    weekID,
				DayNumber: int32(day.DayNumber),
				SortOrder: int32(day.SortOrder),
				DayKey:    pgconv.ToPGUUID(dayKey),
			})
			if err != nil {
				return Detail{}, fmt.Errorf("insert program day: %w", err)
			}
			for _, block := range day.Blocks {
				blockKey := block.BlockKey
				if blockKey == uuid.Nil {
					blockKey = uuid.New()
				}
				blockRow, err := qtx.InsertDayBlockWithKey(ctx, sqlc.InsertDayBlockWithKeyParams{
					ProgramWeekDayID: dayRow.ID,
					BlockKey:         pgconv.ToPGUUID(blockKey),
					BlockType:        string(block.BlockType),
					Instruction:      block.Instruction,
					SortOrder:        int32(block.SortOrder),
					ModifiedAt:       now,
					ModifiedBy:       userPG,
				})
				if err != nil {
					return Detail{}, fmt.Errorf("insert day block: %w", err)
				}
				for _, ex := range block.Exercises {
					if err := qtx.InsertBlockExercise(ctx, sqlc.InsertBlockExerciseParams{
						ProgramWeekDayBlockID: blockRow.ID,
						ExerciseID:            pgconv.ToPGUUID(ex.ExerciseID),
						SortOrder:             int32(ex.SortOrder),
						Sets:                  ex.Sets,
						Reps:                  ex.Reps,
						Instruction:           ex.Instruction,
						ModifiedAt:            now,
						ModifiedBy:            userPG,
					}); err != nil {
						return Detail{}, fmt.Errorf("insert block exercise: %w", err)
					}
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.enrichedDetail(ctx, id)
}

func (s *Store) freezeVersion(ctx context.Context, q *sqlc.Queries, d Detail, userID uuid.UUID, now time.Time) error {
	fp, err := DetailFingerprint(d)
	if err != nil {
		return fmt.Errorf("fingerprint detail: %w", err)
	}

	programPG := pgconv.ToPGUUID(d.ID)
	maxNum, err := q.GetMaxProgramVersionNumber(ctx, programPG)
	if err != nil {
		return fmt.Errorf("max version number: %w", err)
	}

	var category, difficulty *string
	if d.Category != nil {
		c := string(*d.Category)
		category = &c
	}
	if d.Difficulty != nil {
		diff := string(*d.Difficulty)
		difficulty = &diff
	}

	version, err := q.InsertProgramVersion(ctx, sqlc.InsertProgramVersionParams{
		ProgramID:          programPG,
		VersionNumber:      maxNum + 1,
		PublishedAt:        now,
		PublishedBy:        pgconv.ToPGUUID(userID),
		Name:               d.Name,
		NameRu:             d.NameRu,
		Description:        d.Description,
		DescriptionRu:      d.DescriptionRu,
		Category:           category,
		Difficulty:         difficulty,
		PreviewImageUrl:    d.PreviewImageURL,
		ContentFingerprint: fp,
	})
	if err != nil {
		return fmt.Errorf("insert program version: %w", err)
	}

	versionPG := version.ID
	for _, week := range d.Weeks {
		weekRow, err := q.InsertProgramVersionWeek(ctx, sqlc.InsertProgramVersionWeekParams{
			ProgramVersionID: versionPG,
			WeekNumber:       int32(week.WeekNumber),
			SortOrder:        int32(week.SortOrder),
		})
		if err != nil {
			return fmt.Errorf("insert program version week: %w", err)
		}
		for _, day := range week.Days {
			dayKey := day.DayKey
			if dayKey == uuid.Nil {
				dayKey = uuid.New()
			}
			dayRow, err := q.InsertProgramVersionDay(ctx, sqlc.InsertProgramVersionDayParams{
				ProgramVersionID:     versionPG,
				ProgramVersionWeekID: weekRow.ID,
				DayNumber:            int32(day.DayNumber),
				SortOrder:            int32(day.SortOrder),
				DayKey:               pgconv.ToPGUUID(dayKey),
			})
			if err != nil {
				return fmt.Errorf("insert program version day: %w", err)
			}
			for _, block := range day.Blocks {
				blockKey := block.BlockKey
				if blockKey == uuid.Nil {
					blockKey = uuid.New()
				}
				blockRow, err := q.InsertProgramVersionDayBlock(ctx, sqlc.InsertProgramVersionDayBlockParams{
					ProgramVersionWeekDayID: dayRow.ID,
					BlockKey:                pgconv.ToPGUUID(blockKey),
					BlockType:               string(block.BlockType),
					Instruction:             block.Instruction,
					SortOrder:               int32(block.SortOrder),
				})
				if err != nil {
					return fmt.Errorf("insert program version day block: %w", err)
				}
				for _, ex := range block.Exercises {
					if _, err := q.InsertProgramVersionDayExercise(ctx, sqlc.InsertProgramVersionDayExerciseParams{
						ProgramVersionWeekDayBlockID: blockRow.ID,
						ExerciseID:                   pgconv.ToPGUUID(ex.ExerciseID),
						SortOrder:                    int32(ex.SortOrder),
						Sets:                         ex.Sets,
						Reps:                         ex.Reps,
						Instruction:                  ex.Instruction,
					}); err != nil {
						return fmt.Errorf("insert program version day exercise: %w", err)
					}
				}
			}
		}
	}
	return nil
}

func (s *Store) enrichedDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	return s.GetDetail(ctx, id)
}

// enrichProgram takes the queries object explicitly for the same reason as
// listProgramBlockClients: a caller inside a locked transaction passes qtx to
// read a serialized view instead of reaching back into the pool.
func (s *Store) enrichProgram(ctx context.Context, q *sqlc.Queries, p Program, detail *Detail) (Program, error) {
	count, err := q.CountActiveProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(p.ID))
	if err != nil {
		return Program{}, fmt.Errorf("count assignments: %w", err)
	}
	p.AssignmentCount = int(count)

	if detail != nil {
		p.TrainingDaysCount = CountTrainingDays(detail.Weeks)
	}

	latest, err := q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(p.ID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, nil
		}
		return Program{}, fmt.Errorf("latest program version: %w", err)
	}

	latestID := pgconv.FromPGUUID(latest.ID)
	p.LatestProgramVersionID = &latestID
	publishedAt := latest.PublishedAt.UTC()
	p.LatestClientPlanAt = &publishedAt

	if p.Status == StatusPublished {
		d := detail
		if d == nil {
			loaded, loadErr := s.loadDetail(ctx, q, p.ID)
			if loadErr != nil {
				return Program{}, loadErr
			}
			d = &loaded
		}
		p.TrainingDaysCount = CountTrainingDays(d.Weeks)
		fp, err := DetailFingerprint(*d)
		if err != nil {
			return Program{}, err
		}
		p.HasUnpublishedChanges = fp != latest.ContentFingerprint
	}
	return p, nil
}

func (s *Store) ListProgramVersions(ctx context.Context, programID uuid.UUID) (VersionListResult, error) {
	rows, err := s.q.ListProgramVersionsByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return VersionListResult{}, fmt.Errorf("list program versions: %w", err)
	}

	total := len(rows)
	items := make([]VersionSummary, 0, total)
	for _, row := range rows {
		items = append(items, VersionSummary{
			ID:              pgconv.FromPGUUID(row.ID),
			VersionNumber:   int(row.VersionNumber),
			PublishedAt:     row.PublishedAt.UTC(),
			CreatedAt:       row.CreatedAt.UTC(),
			AssignmentCount: int(row.AssignmentCount),
			CanDelete:       row.AssignmentCount == 0 && total > 1,
		})
	}
	return VersionListResult{Items: items}, nil
}

func (s *Store) DeleteProgramVersion(ctx context.Context, programID, versionID uuid.UUID) error {
	version, err := s.q.GetProgramVersionByID(ctx, pgconv.ToPGUUID(versionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get program version: %w", err)
	}
	if pgconv.FromPGUUID(version.ProgramID) != programID {
		return ErrNotFound
	}

	versionCount, err := s.q.CountProgramVersionsByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("count program versions: %w", err)
	}
	if versionCount <= 1 {
		return ErrSoleProgramVersion
	}

	activeCount, err := s.q.CountActiveProgramAssignmentsByVersionID(ctx, pgconv.ToPGUUID(versionID))
	if err != nil {
		return fmt.Errorf("count active assignments by version: %w", err)
	}
	if activeCount > 0 {
		return ErrVersionHasAssignments
	}

	rows, err := s.q.DeleteProgramVersion(ctx, pgconv.ToPGUUID(versionID))
	if err != nil {
		return fmt.Errorf("delete program version: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CleanupProgramVersions(ctx context.Context, programID uuid.UUID) (VersionCleanupResult, error) {
	rows, err := s.q.ListProgramVersionsByProgramID(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return VersionCleanupResult{}, fmt.Errorf("list program versions: %w", err)
	}

	result := VersionCleanupResult{
		DeletedVersionIDs: make([]uuid.UUID, 0),
		Skipped:           make([]VersionCleanupSkipped, 0),
	}
	if len(rows) <= 1 {
		for _, row := range rows {
			result.Skipped = append(result.Skipped, VersionCleanupSkipped{
				VersionID: pgconv.FromPGUUID(row.ID),
				Reason:    "sole_version",
			})
		}
	} else {
		for _, row := range rows {
			versionID := pgconv.FromPGUUID(row.ID)
			if row.AssignmentCount > 0 {
				result.Skipped = append(result.Skipped, VersionCleanupSkipped{
					VersionID: versionID,
					Reason:    "has_assignments",
				})
				continue
			}
			if err := s.DeleteProgramVersion(ctx, programID, versionID); err != nil {
				if errors.Is(err, ErrSoleProgramVersion) {
					result.Skipped = append(result.Skipped, VersionCleanupSkipped{
						VersionID: versionID,
						Reason:    "sole_version",
					})
					continue
				}
				return VersionCleanupResult{}, err
			}
			result.DeletedVersionIDs = append(result.DeletedVersionIDs, versionID)
		}
	}

	// A version deleted just above can be the last surviving place a block_key
	// lived outside the working copy, so this is exactly the moment an orphan
	// rule can appear. Purge unconditionally: even the "nothing deleted" path
	// above can follow a working-copy-only block deletion that already
	// orphaned a rule on its own.
	//
	// Swallow the error rather than returning it: this function's callers
	// include DELETE /programs/{id} (service.go), which already committed the
	// assignment delete before reaching here — a transient purge failure must
	// not leave that request stuck with assignments gone but the program still
	// active. Failing to purge on time is harmless (the rule just survives
	// longer, cleaned up by the next best-effort pass or the janitor);
	// over-deleting on a false purge is not, which is why the version-deletion
	// errors above still propagate untouched.
	if _, err := s.q.PurgeOrphanProgramBlockClientsForProgram(ctx, pgconv.ToPGUUID(programID)); err != nil {
		_ = err
	}
	return result, nil
}

func (s *Store) cleanupUnusedVersionsBestEffort(ctx context.Context, programID uuid.UUID) {
	if _, err := s.CleanupProgramVersions(ctx, programID); err != nil {
		// Best-effort: assignment change already committed; log would need a logger on Store.
		_ = err
		// CleanupProgramVersions swallows its own purge error, so on success it
		// already attempted the purge — a second call here would be pure
		// redundancy. Retry only on error: CleanupProgramVersions can fail
		// before it ever reaches its internal purge call (e.g.
		// ListProgramVersionsByProgramID or a hard DeleteProgramVersion error),
		// and this is the only path that still gives the purge a chance in
		// that case.
		if _, err := s.q.PurgeOrphanProgramBlockClientsForProgram(ctx, pgconv.ToPGUUID(programID)); err != nil {
			_ = err
		}
	}
}
