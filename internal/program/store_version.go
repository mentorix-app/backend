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
			dayRow, err := q.InsertProgramVersionDay(ctx, sqlc.InsertProgramVersionDayParams{
				ProgramVersionID:     versionPG,
				ProgramVersionWeekID: weekRow.ID,
				DayNumber:            int32(day.DayNumber),
				SortOrder:            int32(day.SortOrder),
			})
			if err != nil {
				return fmt.Errorf("insert program version day: %w", err)
			}
			for _, ex := range day.Exercises {
				if _, err := q.InsertProgramVersionDayExercise(ctx, sqlc.InsertProgramVersionDayExerciseParams{
					ProgramVersionDayID: dayRow.ID,
					ExerciseID:          pgconv.ToPGUUID(ex.ExerciseID),
					SortOrder:           int32(ex.SortOrder),
					Sets:                intPtrToInt32(ex.Sets),
					Reps:                intPtrToInt32(ex.Reps),
					WeightKg:            pgconv.ToNumeric(ex.WeightKg),
					Instruction:         ex.Instruction,
				}); err != nil {
					return fmt.Errorf("insert program version day exercise: %w", err)
				}
			}
		}
	}
	return nil
}

func (s *Store) enrichedDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	return s.GetDetail(ctx, id)
}

func (s *Store) enrichProgram(ctx context.Context, p Program, detail *Detail) (Program, error) {
	count, err := s.q.CountActiveProgramAssignmentsByProgramID(ctx, pgconv.ToPGUUID(p.ID))
	if err != nil {
		return Program{}, fmt.Errorf("count assignments: %w", err)
	}
	p.AssignmentCount = int(count)

	latest, err := s.q.GetLatestProgramVersionByProgramID(ctx, pgconv.ToPGUUID(p.ID))
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
			loaded, loadErr := s.loadDetail(ctx, p.ID)
			if loadErr != nil {
				return Program{}, loadErr
			}
			d = &loaded
		}
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
		return result, nil
	}

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
	return result, nil
}
