package trainerclient

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

const pgUniqueViolationCode = "23505"

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) TrainerIDForUser(ctx context.Context, trainerUserID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		return uuid.Nil, err
	}
	return pgconv.FromPGUUID(id), nil
}

func (s *Store) CreateInvite(ctx context.Context, trainerID uuid.UUID, ttl time.Duration) (sqlc.MentorixTrainerInvite, string, error) {
	token, err := newInviteToken()
	if err != nil {
		return sqlc.MentorixTrainerInvite{}, "", err
	}
	now := time.Now().UTC()
	if _, err := s.q.DeleteStaleTrainerInvitesByTrainerID(ctx, pgconv.ToPGUUID(trainerID)); err != nil {
		return sqlc.MentorixTrainerInvite{}, "", fmt.Errorf("delete stale trainer invites: %w", err)
	}
	row, err := s.q.InsertTrainerInvite(ctx, sqlc.InsertTrainerInviteParams{
		TrainerID: pgconv.ToPGUUID(trainerID),
		Token:     token,
		ExpiresAt: now.Add(ttl),
	})
	if err != nil {
		return sqlc.MentorixTrainerInvite{}, "", fmt.Errorf("insert trainer invite: %w", err)
	}
	return row, token, nil
}

func (s *Store) ListClients(ctx context.Context, trainerID uuid.UUID, params ListParams) (ClientListResult, error) {
	qPattern := qPattern(params.Query)
	total, err := s.q.CountTrainerClients(ctx, sqlc.CountTrainerClientsParams{
		TrainerID: pgconv.ToPGUUID(trainerID),
		QPattern:  qPattern,
	})
	if err != nil {
		return ClientListResult{}, fmt.Errorf("count trainer clients: %w", err)
	}

	rows, err := s.q.ListTrainerClients(ctx, sqlc.ListTrainerClientsParams{
		TrainerID: pgconv.ToPGUUID(trainerID),
		QPattern:  qPattern,
		SortBy:    params.SortBy,
		SortOrder: params.SortOrder,
		Limit:     int32(params.Limit),
		Offset:    int32(params.Offset()),
	})
	if err != nil {
		return ClientListResult{}, fmt.Errorf("list trainer clients: %w", err)
	}
	return clientListResult(mapTrainerClientRows(rows), params, int(total)), nil
}

func (s *Store) ListAllClients(ctx context.Context, params ListParams, preferTrainerID *uuid.UUID) (ClientListResult, error) {
	qPattern := qPattern(params.Query)
	total, err := s.q.CountAllTrainerClients(ctx, qPattern)
	if err != nil {
		return ClientListResult{}, fmt.Errorf("count all trainer clients: %w", err)
	}

	var prefer pgtype.UUID
	if preferTrainerID != nil {
		prefer = pgconv.ToPGUUID(*preferTrainerID)
	}

	rows, err := s.q.ListAllTrainerClients(ctx, sqlc.ListAllTrainerClientsParams{
		PreferTrainerID: prefer,
		QPattern:        qPattern,
		SortBy:          params.SortBy,
		SortOrder:       params.SortOrder,
		Limit:           int32(params.Limit),
		Offset:          int32(params.Offset()),
	})
	if err != nil {
		return ClientListResult{}, fmt.Errorf("list all trainer clients: %w", err)
	}
	return clientListResult(mapAllTrainerClientRows(rows), params, int(total)), nil
}

func mapTrainerClientRows(rows []sqlc.ListTrainerClientsRow) []clientListRow {
	out := make([]clientListRow, len(rows))
	for i, row := range rows {
		out[i] = clientListRow{
			clientUserID:   row.ClientUserID,
			trainerUserID:  row.TrainerUserID,
			status:         row.Status,
			linkedAt:       row.CreatedAt,
			displayName:    row.DisplayName,
			avatarFilePath: row.AvatarFilePath,
			assignmentID:   row.AssignmentID,
			programID:      row.ProgramID,
			programVersion: row.ProgramVersionID,
			assignedAt:     row.AssignedAt,
			programName:    stringFromPtr(row.ProgramName),
			programNameRu:  stringFromPtr(row.ProgramNameRu),
			isBehindLatest: row.IsBehindLatest,
		}
	}
	return out
}

func mapAllTrainerClientRows(rows []sqlc.ListAllTrainerClientsRow) []clientListRow {
	out := make([]clientListRow, len(rows))
	for i, row := range rows {
		out[i] = clientListRow{
			clientUserID:   row.ClientUserID,
			trainerUserID:  row.TrainerUserID,
			status:         row.Status,
			linkedAt:       row.CreatedAt,
			displayName:    row.DisplayName,
			avatarFilePath: row.AvatarFilePath,
			assignmentID:   row.AssignmentID,
			programID:      row.ProgramID,
			programVersion: row.ProgramVersionID,
			assignedAt:     row.AssignedAt,
			programName:    stringFromPtr(row.ProgramName),
			programNameRu:  stringFromPtr(row.ProgramNameRu),
			isBehindLatest: row.IsBehindLatest,
		}
	}
	return out
}

type clientListRow struct {
	clientUserID   pgtype.UUID
	trainerUserID  pgtype.UUID
	status         string
	linkedAt       time.Time
	displayName    string
	avatarFilePath string
	assignmentID   pgtype.UUID
	programID      pgtype.UUID
	programVersion pgtype.UUID
	assignedAt     pgtype.Timestamptz
	programName    string
	programNameRu  string
	isBehindLatest bool
}

func clientListResult(rows []clientListRow, params ListParams, total int) ClientListResult {
	out := make([]Client, 0, len(rows))
	for _, row := range rows {
		client := Client{
			ClientUserID:   pgconv.FromPGUUID(row.clientUserID),
			TrainerUserID:  pgconv.FromPGUUID(row.trainerUserID),
			DisplayName:    row.displayName,
			Status:         row.status,
			LinkedAt:       row.linkedAt.UTC(),
			avatarFilePath: row.avatarFilePath,
		}
		if row.assignmentID.Valid {
			summary := &ClientProgramSummary{
				AssignmentID:     pgconv.FromPGUUID(row.assignmentID),
				ProgramID:        pgconv.FromPGUUID(row.programID),
				ProgramVersionID: pgconv.FromPGUUID(row.programVersion),
				AssignedAt:       row.assignedAt.Time.UTC(),
				ProgramName:      row.programName,
				ProgramNameRu:    row.programNameRu,
			}
			behind := row.isBehindLatest
			summary.IsBehindLatest = &behind
			client.ProgramAssignment = summary
		}
		out = append(out, client)
	}
	return ClientListResult{
		Items:      out,
		Pagination: paginationMeta(params.Page, params.Limit, total),
	}
}

func (s *Store) UserIDByTelegram(ctx context.Context, telegramUserID string) (uuid.UUID, error) {
	userPG, err := s.q.GetAuthIdentityUserID(ctx, sqlc.GetAuthIdentityUserIDParams{
		Provider: auth.ProviderTelegram,
		Subject:  strings.TrimSpace(telegramUserID),
	})
	if err != nil {
		return uuid.Nil, err
	}
	return pgconv.FromPGUUID(userPG), nil
}

func (s *Store) UpdateUserAvatarFilePath(ctx context.Context, userID uuid.UUID, filePath string) error {
	if err := s.q.UpdateUserAvatarFilePath(ctx, sqlc.UpdateUserAvatarFilePathParams{
		ID:             pgconv.ToPGUUID(userID),
		AvatarFilePath: filePath,
	}); err != nil {
		return fmt.Errorf("update user avatar: %w", err)
	}
	return nil
}

func (s *Store) UserAvatarFilePath(ctx context.Context, userID uuid.UUID) (string, error) {
	path, err := s.q.GetUserAvatarFilePath(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get user avatar: %w", err)
	}
	return path, nil
}

func (s *Store) AcceptInvite(ctx context.Context, token, telegramUserID, displayName string) (AcceptInviteResult, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AcceptInviteResult{}, ErrInviteNotFound
	}

	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AcceptInviteResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	invite, err := qtx.GetTrainerInviteByTokenForUpdate(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AcceptInviteResult{}, ErrInviteNotFound
		}
		return AcceptInviteResult{}, fmt.Errorf("get invite: %w", err)
	}

	trainerID := pgconv.FromPGUUID(invite.TrainerID)
	trainerName, err := qtx.GetTrainerUserDisplayName(ctx, invite.TrainerID)
	if err != nil {
		return AcceptInviteResult{}, fmt.Errorf("trainer display name: %w", err)
	}

	userID, _, err := s.resolveTelegramUser(ctx, qtx, telegramUserID, displayName)
	if err != nil {
		return AcceptInviteResult{}, err
	}

	if invite.ConsumedAt.Valid {
		consumedBy := pgconv.FromPGUUID(invite.ConsumedBy)
		if consumedBy != userID {
			return AcceptInviteResult{}, ErrInviteConsumed
		}
		result, err := s.ensureClientLink(ctx, qtx, trainerID, userID, trainerName)
		if err != nil {
			return AcceptInviteResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return AcceptInviteResult{}, fmt.Errorf("commit: %w", err)
		}
		return result, nil
	}

	if !invite.ExpiresAt.After(now) {
		return AcceptInviteResult{}, ErrInviteExpired
	}

	result, err := s.ensureClientLink(ctx, qtx, trainerID, userID, trainerName)
	if err != nil {
		return AcceptInviteResult{}, err
	}

	rows, err := qtx.ConsumeTrainerInvite(ctx, sqlc.ConsumeTrainerInviteParams{
		ID:         invite.ID,
		ConsumedAt: pgtype.Timestamptz{Time: now, Valid: true},
		ConsumedBy: pgconv.ToPGUUID(userID),
	})
	if err != nil {
		return AcceptInviteResult{}, fmt.Errorf("consume invite: %w", err)
	}
	if rows == 0 {
		return AcceptInviteResult{}, ErrInviteConsumed
	}

	if err := tx.Commit(ctx); err != nil {
		return AcceptInviteResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func (s *Store) resolveTelegramUser(ctx context.Context, q *sqlc.Queries, telegramUserID, displayName string) (uuid.UUID, bool, error) {
	subject := strings.TrimSpace(telegramUserID)
	if subject == "" {
		return uuid.Nil, false, fmt.Errorf("telegram user id required")
	}

	userPG, err := q.GetAuthIdentityUserID(ctx, sqlc.GetAuthIdentityUserIDParams{
		Provider: auth.ProviderTelegram,
		Subject:  subject,
	})
	if err == nil {
		return pgconv.FromPGUUID(userPG), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("lookup telegram identity: %w", err)
	}

	userPG, err = q.InsertUser(ctx, sqlc.InsertUserParams{
		PrimaryEmail: nil,
		DisplayName:  normalizeDisplayName(displayName),
	})
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("insert client user: %w", err)
	}

	if err := q.InsertAuthIdentity(ctx, sqlc.InsertAuthIdentityParams{
		UserID:       userPG,
		Provider:     auth.ProviderTelegram,
		Subject:      subject,
		PasswordHash: nil,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode {
			userPG, err = q.GetAuthIdentityUserID(ctx, sqlc.GetAuthIdentityUserIDParams{
				Provider: auth.ProviderTelegram,
				Subject:  subject,
			})
			if err != nil {
				return uuid.Nil, false, fmt.Errorf("lookup telegram identity after race: %w", err)
			}
			return pgconv.FromPGUUID(userPG), false, nil
		}
		return uuid.Nil, false, fmt.Errorf("insert telegram identity: %w", err)
	}

	if err := q.InsertUserRole(ctx, sqlc.InsertUserRoleParams{
		UserID: userPG,
		Role:   auth.RoleClient,
	}); err != nil {
		return uuid.Nil, false, fmt.Errorf("grant client role: %w", err)
	}

	return pgconv.FromPGUUID(userPG), true, nil
}

func (s *Store) ensureClientLink(ctx context.Context, q *sqlc.Queries, trainerID, clientUserID uuid.UUID, trainerName string) (AcceptInviteResult, error) {
	link, err := q.GetTrainerClient(ctx, sqlc.GetTrainerClientParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	alreadyLinked := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AcceptInviteResult{}, fmt.Errorf("get trainer client: %w", err)
	}
	if alreadyLinked && link.Status == "blocked" {
		return AcceptInviteResult{}, program.ErrClientBlocked
	}

	if !alreadyLinked {
		if err := q.InsertTrainerClient(ctx, sqlc.InsertTrainerClientParams{
			TrainerID:    pgconv.ToPGUUID(trainerID),
			ClientUserID: pgconv.ToPGUUID(clientUserID),
		}); err != nil {
			return AcceptInviteResult{}, fmt.Errorf("insert trainer client: %w", err)
		}
	}

	return AcceptInviteResult{
		UserID:             clientUserID,
		TrainerID:          trainerID,
		TrainerDisplayName: trainerName,
		AlreadyLinked:      alreadyLinked,
	}, nil
}

func newInviteToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func normalizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Client"
	}
	return name
}

func inviteURL(botUsername, token string) string {
	username := strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	return fmt.Sprintf("https://t.me/%s?start=inv_%s", username, token)
}

func stringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
