package telegramnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

type ProgramNotifier interface {
	NotifyProgramAssigned(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error
	NotifyProgramSynced(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error
}

type Notifier struct {
	store  notifyStore
	sender Sender
	log    *slog.Logger
}

type notifyStore interface {
	GetTelegramSubjectByUserID(ctx context.Context, arg sqlc.GetTelegramSubjectByUserIDParams) (string, error)
	GetTrainerDisplayNameByTrainerID(ctx context.Context, id pgtype.UUID) (string, error)
	GetProgramVersionDisplayByID(ctx context.Context, id pgtype.UUID) (sqlc.GetProgramVersionDisplayByIDRow, error)
}

func NewNotifier(pool *pgxpool.Pool, sender Sender, log *slog.Logger) *Notifier {
	return NewNotifierWithStore(sqlc.New(pool), sender, log)
}

func NewNotifierWithStore(store notifyStore, sender Sender, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{store: store, sender: sender, log: log}
}

func (n *Notifier) NotifyProgramAssigned(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error {
	return n.notify(ctx, clientUserID, trainerID, programVersionID, assignedMessage)
}

func (n *Notifier) NotifyProgramSynced(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error {
	return n.notify(ctx, clientUserID, trainerID, programVersionID, syncedMessage)
}

func (n *Notifier) notify(
	ctx context.Context,
	clientUserID, trainerID, programVersionID uuid.UUID,
	format func(trainerName, programName string) string,
) error {
	if n == nil || n.sender == nil {
		return nil
	}

	chatID, err := n.telegramChatID(ctx, clientUserID)
	if err != nil {
		if errors.Is(err, errNoTelegramIdentity) {
			return nil
		}
		return err
	}

	trainerName, err := n.trainerDisplayName(ctx, trainerID)
	if err != nil {
		return err
	}

	programName, err := n.programDisplayName(ctx, programVersionID)
	if err != nil {
		return err
	}

	text := format(trainerName, programName)
	if err := n.sender.SendMessage(ctx, chatID, text); err != nil {
		n.log.Warn("telegram notify failed", "client_user_id", clientUserID, "error", err)
		return err
	}
	return nil
}

var errNoTelegramIdentity = errors.New("client has no telegram identity")

func (n *Notifier) telegramChatID(ctx context.Context, clientUserID uuid.UUID) (int64, error) {
	subject, err := n.store.GetTelegramSubjectByUserID(ctx, sqlc.GetTelegramSubjectByUserIDParams{
		UserID:   pgconv.ToPGUUID(clientUserID),
		Provider: auth.ProviderTelegram,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errNoTelegramIdentity
		}
		return 0, fmt.Errorf("telegram subject: %w", err)
	}
	chatID, err := strconv.ParseInt(subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse telegram subject %q: %w", subject, err)
	}
	return chatID, nil
}

func (n *Notifier) trainerDisplayName(ctx context.Context, trainerID uuid.UUID) (string, error) {
	name, err := n.store.GetTrainerDisplayNameByTrainerID(ctx, pgconv.ToPGUUID(trainerID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "ваш тренер", nil
		}
		return "", fmt.Errorf("trainer display name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "ваш тренер", nil
	}
	return name, nil
}

func (n *Notifier) programDisplayName(ctx context.Context, programVersionID uuid.UUID) (string, error) {
	row, err := n.store.GetProgramVersionDisplayByID(ctx, pgconv.ToPGUUID(programVersionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "программа", nil
		}
		return "", fmt.Errorf("program version display: %w", err)
	}
	name := programDisplayName(row.Name, row.NameRu)
	if name == "" {
		return "программа", nil
	}
	return name, nil
}
