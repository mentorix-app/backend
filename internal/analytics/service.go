package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/trainerclient"
)

type Service struct {
	store     *Store
	jwtSecret string
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool, jwtSecret string) *Service {
	return &Service{
		store:     NewStore(pool),
		jwtSecret: jwtSecret,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) trainerID(ctx context.Context, trainerUserID uuid.UUID) (uuid.UUID, error) {
	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrForbidden
		}
		return uuid.Nil, fmt.Errorf("trainer id: %w", err)
	}
	return trainerID, nil
}

func (s *Service) ClientAnalytics(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (ClientAnalytics, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return ClientAnalytics{}, err
	}

	header, err := s.store.ClientHeader(ctx, trainerID, clientUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientAnalytics{}, ErrClientNotFound
		}
		return ClientAnalytics{}, fmt.Errorf("client header: %w", err)
	}

	now := s.now()
	var (
		assignment      *sqlc.GetClientCurrentAssignmentAnalyticsRow
		stats           sqlc.GetClientActivityStatsRow
		weekStarts      []time.Time
		programActivity []sqlc.ListClientProgramActivityRow
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		row, err := s.store.CurrentAssignment(gctx, trainerID, clientUserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("current assignment: %w", err)
		}
		assignment = &row
		return nil
	})
	g.Go(func() error {
		var err error
		stats, err = s.store.ActivityStats(gctx, trainerID, clientUserID, now.AddDate(0, 0, -7), now.AddDate(0, 0, -30))
		if err != nil {
			return fmt.Errorf("activity stats: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		weekStarts, err = s.store.CompletionWeeks(gctx, trainerID, clientUserID)
		if err != nil {
			return fmt.Errorf("completion weeks: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		programActivity, err = s.store.ProgramActivity(gctx, trainerID, clientUserID)
		if err != nil {
			return fmt.Errorf("program activity: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return ClientAnalytics{}, err
	}

	result := ClientAnalytics{
		Client: ClientInfo{
			ClientUserID: pgconv.FromPGUUID(header.ClientUserID),
			DisplayName:  header.DisplayName,
			AvatarURL:    trainerclient.BuildAvatarURL(clientUserID, header.AvatarFilePath, s.jwtSecret),
			Status:       header.Status,
			LinkedAt:     header.LinkedAt.UTC(),
			LastActiveAt: timePtrFromAny(header.LastActiveAt),
		},
		Activity: ActivityStats{
			TotalCompletions:      int(stats.TotalCompletions),
			CompletionsLast7Days:  int(stats.CompletionsLast7Days),
			CompletionsLast30Days: int(stats.CompletionsLast30Days),
			FirstCompletedAt:      timePtrFromAny(stats.FirstCompletedAt),
			LastCompletedAt:       timePtrFromAny(stats.LastCompletedAt),
			WeekStreak:            weekStreak(weekStarts, now),
			ByProgram:             mapProgramActivity(programActivity),
		},
	}

	if assignment != nil {
		weekRows, err := s.store.CycleWeekProgress(ctx, pgconv.FromPGUUID(assignment.ProgramVersionID), pgconv.FromPGUUID(assignment.CompletionCycleID))
		if err != nil {
			return ClientAnalytics{}, fmt.Errorf("cycle week progress: %w", err)
		}
		result.CurrentAssignment = &AssignmentAnalytics{
			ProgramID:        pgconv.FromPGUUID(assignment.ProgramID),
			ProgramVersionID: pgconv.FromPGUUID(assignment.ProgramVersionID),
			ProgramName:      assignment.ProgramName,
			ProgramNameRu:    assignment.ProgramNameRu,
			AssignedAt:       assignment.AssignedAt.UTC(),
			IsBehindLatest:   assignment.IsBehindLatest,
			Progress:         buildProgress(weekRows),
		}
	}
	return result, nil
}

func (s *Service) ClientCompletions(ctx context.Context, trainerUserID, clientUserID uuid.UUID, params CompletionsParams) (CompletionsResult, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return CompletionsResult{}, err
	}
	if _, err := s.store.ClientHeader(ctx, trainerID, clientUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CompletionsResult{}, ErrClientNotFound
		}
		return CompletionsResult{}, fmt.Errorf("client header: %w", err)
	}

	total, err := s.store.CountClientCompletions(ctx, trainerID, clientUserID, params)
	if err != nil {
		return CompletionsResult{}, fmt.Errorf("count completions: %w", err)
	}
	rows, err := s.store.ListClientCompletions(ctx, trainerID, clientUserID, params)
	if err != nil {
		return CompletionsResult{}, fmt.Errorf("list completions: %w", err)
	}

	comments, err := s.commentsByCompletionIDs(ctx, completionIDsFromClientRows(rows))
	if err != nil {
		return CompletionsResult{}, err
	}

	items := make([]CompletionItem, 0, len(rows))
	for _, row := range rows {
		id := pgconv.FromPGUUID(row.ID)
		itemComments := comments[id]
		if itemComments == nil {
			itemComments = []CompletionComment{}
		}
		items = append(items, CompletionItem{
			ID:             id,
			CompletedAt:    row.CompletedAt.UTC(),
			ProgramID:      uuidPtrFromPG(row.ProgramID),
			ProgramName:    row.ProgramName,
			ProgramNameRu:  row.ProgramNameRu,
			WeekNumber:     int(row.WeekNumber),
			DayNumber:      int(row.DayNumber),
			ResultText:     row.ResultText,
			IsCurrentCycle: row.IsCurrentCycle,
			Comments:       itemComments,
		})
	}
	return CompletionsResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, total),
	}, nil
}

func completionIDsFromClientRows(rows []sqlc.ListClientCompletionsRow) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, pgconv.FromPGUUID(row.ID))
	}
	return ids
}

// commentsByCompletionIDs loads trainer replies in one query and groups by completion id.
func (s *Service) commentsByCompletionIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]CompletionComment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	commentRows, err := s.store.ListCommentsForCompletions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list completion comments: %w", err)
	}
	out := make(map[uuid.UUID][]CompletionComment, len(commentRows))
	for _, row := range commentRows {
		completionID := pgconv.FromPGUUID(row.ClientWorkoutCompletionID)
		out[completionID] = append(out[completionID], CompletionComment{
			ID:        pgconv.FromPGUUID(row.ID),
			Text:      row.CommentText,
			CreatedAt: row.CreatedAt.UTC(),
		})
	}
	return out, nil
}

func (s *Service) ProgramsAnalytics(ctx context.Context, trainerUserID uuid.UUID, params ProgramsParams) (ProgramsAnalyticsResult, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return ProgramsAnalyticsResult{}, err
	}

	total, err := s.store.CountTrainerPrograms(ctx, trainerUserID)
	if err != nil {
		return ProgramsAnalyticsResult{}, fmt.Errorf("count programs: %w", err)
	}
	rows, err := s.store.ListProgramsAnalytics(ctx, trainerUserID, trainerID, s.now().AddDate(0, 0, -30), params)
	if err != nil {
		return ProgramsAnalyticsResult{}, fmt.Errorf("list programs analytics: %w", err)
	}

	items := make([]ProgramAnalyticsItem, 0, len(rows))
	for _, row := range rows {
		item := ProgramAnalyticsItem{
			ProgramID:             pgconv.FromPGUUID(row.ProgramID),
			Name:                  row.Name,
			NameRu:                row.NameRu,
			Status:                row.Status,
			TrainingDaysCount:     int(row.TrainingDaysCount),
			ActiveClientsCount:    int(row.ActiveClientsCount),
			TotalCompletions:      int(row.TotalCompletions),
			CompletionsLast30Days: int(row.CompletionsLast30Days),
			LastActivityAt:        timePtrFromAny(row.LastActivityAt),
		}
		if row.ActiveClientsCount > 0 {
			pct := roundPercent(row.AvgCompletionPercent)
			item.AvgCompletionPercent = &pct
		}
		items = append(items, item)
	}
	return ProgramsAnalyticsResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, total),
	}, nil
}

func (s *Service) ProgramAnalytics(ctx context.Context, trainerUserID, programID uuid.UUID) (ProgramAnalytics, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return ProgramAnalytics{}, err
	}

	header, err := s.store.ProgramHeader(ctx, programID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProgramAnalytics{}, ErrProgramNotFound
		}
		return ProgramAnalytics{}, fmt.Errorf("program header: %w", err)
	}
	if pgconv.FromPGUUID(header.CreatedBy) != trainerUserID {
		return ProgramAnalytics{}, ErrProgramNotFound
	}

	var (
		totals     sqlc.GetProgramCompletionTotalsRow
		clientRows []sqlc.ListProgramAnalyticsClientsRow
		weekRows   []sqlc.ListProgramAnalyticsWeeksRow
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		totals, err = s.store.ProgramCompletionTotals(gctx, trainerID, programID, s.now().AddDate(0, 0, -30))
		if err != nil {
			return fmt.Errorf("completion totals: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		clientRows, err = s.store.ProgramClients(gctx, programID)
		if err != nil {
			return fmt.Errorf("program clients: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		weekRows, err = s.store.ProgramWeeks(gctx, programID)
		if err != nil {
			return fmt.Errorf("program weeks: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return ProgramAnalytics{}, err
	}

	clients := make([]ProgramClient, 0, len(clientRows))
	var percentSum float64
	var percentCount int
	for _, row := range clientRows {
		pct := completionPercent(int(row.CompletedDays), int(row.TotalTrainingDays))
		if row.TotalTrainingDays > 0 {
			percentSum += pct
			percentCount++
		}
		clients = append(clients, ProgramClient{
			ClientUserID:      pgconv.FromPGUUID(row.ClientUserID),
			DisplayName:       row.DisplayName,
			AvatarURL:         trainerclient.BuildAvatarURL(pgconv.FromPGUUID(row.ClientUserID), row.AvatarFilePath, s.jwtSecret),
			AssignedAt:        row.AssignedAt.UTC(),
			IsBehindLatest:    row.IsBehindLatest,
			CompletedDays:     int(row.CompletedDays),
			TotalTrainingDays: int(row.TotalTrainingDays),
			CompletionPercent: pct,
			LastCompletedAt:   timePtrFromAny(row.LastCompletedAt),
		})
	}

	summary := ProgramSummary{
		ActiveClientsCount:    len(clients),
		TotalCompletions:      int(totals.TotalCompletions),
		CompletionsLast30Days: int(totals.CompletionsLast30Days),
		LastActivityAt:        timePtrFromAny(totals.LastActivityAt),
	}
	if percentCount > 0 {
		avg := roundPercent(percentSum / float64(percentCount))
		summary.AvgCompletionPercent = &avg
	}

	weeks := make([]ProgramWeekStats, 0, len(weekRows))
	for _, row := range weekRows {
		weeks = append(weeks, ProgramWeekStats{
			WeekNumber:           int(row.WeekNumber),
			CompletionsCount:     int(row.CompletionsCount),
			DistinctClientsCount: int(row.DistinctClientsCount),
		})
	}

	return ProgramAnalytics{
		Program: ProgramHeader{
			ProgramID:           pgconv.FromPGUUID(header.ProgramID),
			Name:                header.Name,
			NameRu:              header.NameRu,
			Status:              header.Status,
			LatestVersionNumber: int(header.LatestVersionNumber),
			TrainingDaysCount:   int(header.TrainingDaysCount),
		},
		Summary: summary,
		Clients: clients,
		Weeks:   weeks,
	}, nil
}

func (s *Service) ProgramWeekResults(ctx context.Context, trainerUserID, programID uuid.UUID, weekNumber int) (ProgramWeekResults, error) {
	if weekNumber < 1 {
		return ProgramWeekResults{}, fmt.Errorf("%w: week_number must be >= 1", ErrValidation)
	}
	if _, err := s.trainerID(ctx, trainerUserID); err != nil {
		return ProgramWeekResults{}, err
	}

	header, err := s.store.ProgramHeader(ctx, programID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProgramWeekResults{}, ErrProgramNotFound
		}
		return ProgramWeekResults{}, fmt.Errorf("program header: %w", err)
	}
	if pgconv.FromPGUUID(header.CreatedBy) != trainerUserID {
		return ProgramWeekResults{}, ErrProgramNotFound
	}

	dayRows, err := s.store.LatestVersionWeekTrainingDays(ctx, programID, weekNumber)
	if err != nil {
		return ProgramWeekResults{}, fmt.Errorf("week training days: %w", err)
	}
	if len(dayRows) == 0 {
		return ProgramWeekResults{}, ErrWeekNotFound
	}

	clientRows, err := s.store.ProgramWeekMatrixClients(ctx, programID)
	if err != nil {
		return ProgramWeekResults{}, fmt.Errorf("matrix clients: %w", err)
	}
	completionRows, err := s.store.ProgramWeekCompletions(ctx, programID, weekNumber)
	if err != nil {
		return ProgramWeekResults{}, fmt.Errorf("week completions: %w", err)
	}

	completionIDs := make([]uuid.UUID, 0, len(completionRows))
	// Map client_user_id → day_key → completion
	byClientDayKey := make(map[uuid.UUID]map[uuid.UUID]sqlc.ListProgramWeekCompletionsRow, len(clientRows))
	for _, row := range completionRows {
		clientID := pgconv.FromPGUUID(row.ClientUserID)
		dayKey := pgconv.FromPGUUID(row.DayKey)
		if byClientDayKey[clientID] == nil {
			byClientDayKey[clientID] = map[uuid.UUID]sqlc.ListProgramWeekCompletionsRow{}
		}
		byClientDayKey[clientID][dayKey] = row
		completionIDs = append(completionIDs, pgconv.FromPGUUID(row.ID))
	}
	comments, err := s.commentsByCompletionIDs(ctx, completionIDs)
	if err != nil {
		return ProgramWeekResults{}, err
	}

	columns := make([]ProgramWeekDayColumn, 0, len(dayRows))
	dayKeys := make([]uuid.UUID, 0, len(dayRows))
	for _, row := range dayRows {
		columns = append(columns, ProgramWeekDayColumn{DayNumber: int(row.DayNumber)})
		dayKeys = append(dayKeys, pgconv.FromPGUUID(row.DayKey))
	}

	totalDays := len(columns)
	var submittedCount, behindCount int
	clients := make([]ProgramWeekMatrixClient, 0, len(clientRows))
	for _, row := range clientRows {
		clientID := pgconv.FromPGUUID(row.ClientUserID)
		clientCompletions := byClientDayKey[clientID]
		cells := make([]ProgramWeekMatrixCell, 0, totalDays)
		completed := 0
		for i, dayKey := range dayKeys {
			cell := ProgramWeekMatrixCell{
				DayNumber: columns[i].DayNumber,
				Status:    MatrixCellNoResult,
				Comments:  []CompletionComment{},
			}
			if c, ok := clientCompletions[dayKey]; ok {
				id := pgconv.FromPGUUID(c.ID)
				completedAt := c.CompletedAt.UTC()
				cellComments := comments[id]
				if cellComments == nil {
					cellComments = []CompletionComment{}
				}
				cell.Status = MatrixCellSubmitted
				cell.CompletionID = &id
				cell.ResultText = c.ResultText
				cell.CompletedAt = &completedAt
				cell.Comments = cellComments
				completed++
				submittedCount++
			}
			cells = append(cells, cell)
		}
		if row.IsBehindLatest {
			behindCount++
		}
		clients = append(clients, ProgramWeekMatrixClient{
			ClientUserID:   clientID,
			DisplayName:    row.DisplayName,
			AvatarURL:      trainerclient.BuildAvatarURL(clientID, row.AvatarFilePath, s.jwtSecret),
			IsBehindLatest: row.IsBehindLatest,
			CompletedDays:  completed,
			TotalDays:      totalDays,
			Days:           cells,
		})
	}

	totalSlots := len(clients) * totalDays
	missing := totalSlots - submittedCount
	return ProgramWeekResults{
		ProgramID:     pgconv.FromPGUUID(header.ProgramID),
		ProgramName:   header.Name,
		ProgramNameRu: header.NameRu,
		WeekNumber:    weekNumber,
		Days:          columns,
		Summary: ProgramWeekMatrixSummary{
			TotalTrainingSlots: totalSlots,
			SubmittedCount:     submittedCount,
			MissingCount:       missing,
			CompletionPercent:  completionPercent(submittedCount, totalSlots),
			BehindClientsCount: behindCount,
		},
		Clients: clients,
	}, nil
}

func buildProgress(rows []sqlc.ListCycleWeekProgressRow) Progress {
	weeks := make([]WeekProgress, 0, len(rows))
	var completed, total int
	for _, row := range rows {
		weeks = append(weeks, WeekProgress{
			WeekNumber:    int(row.WeekNumber),
			TotalDays:     int(row.TotalDays),
			CompletedDays: int(row.CompletedDays),
		})
		completed += int(row.CompletedDays)
		total += int(row.TotalDays)
	}
	return Progress{
		CompletedDays:     completed,
		TotalTrainingDays: total,
		CompletionPercent: completionPercent(completed, total),
		Weeks:             weeks,
	}
}

func mapProgramActivity(rows []sqlc.ListClientProgramActivityRow) []ProgramActivity {
	out := make([]ProgramActivity, 0, len(rows))
	for _, row := range rows {
		item := ProgramActivity{
			ProgramID:        uuidPtrFromPG(row.ProgramID),
			ProgramName:      row.ProgramName,
			ProgramNameRu:    row.ProgramNameRu,
			TotalCompletions: int(row.TotalCompletions),
		}
		if t := timePtrFromAny(row.FirstCompletedAt); t != nil {
			item.FirstCompletedAt = *t
		}
		if t := timePtrFromAny(row.LastCompletedAt); t != nil {
			item.LastCompletedAt = *t
		}
		out = append(out, item)
	}
	return out
}

// completionPercent clamps at 100: the journal may contain completions for
// days that no longer count as training days in the assigned version.
func completionPercent(completed, total int) float64 {
	if total <= 0 {
		return 0
	}
	pct := float64(completed) / float64(total) * 100
	if pct > 100 {
		pct = 100
	}
	return roundPercent(pct)
}

func roundPercent(pct float64) float64 {
	return float64(int(pct*10+0.5)) / 10
}
