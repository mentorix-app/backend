package telegramnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

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

// WorkoutComment carries the completion snapshot and the trainer reply text
// for the "trainer commented on your result" push.
type WorkoutComment struct {
	ProgramName   string
	ProgramNameRu string
	WeekNumber    int
	DayNumber     int
	ResultText    string
	CommentText   string
}

const (
	notifyWorkers  = 4
	notifyQueueCap = 256
	notifyJobTTL   = 30 * time.Second
)

// Notifier queues Telegram notifications and sends them from a fixed pool of
// workers, so a request never waits for the lookups or for Telegram.
// Notify methods are safe for concurrent use and always return nil.
type Notifier struct {
	store  notifyStore
	sender Sender
	log    *slog.Logger

	// baseCtx parents every job context; Close cancels it to stop jobs that
	// outlive the shutdown deadline.
	baseCtx    context.Context
	cancelBase context.CancelFunc

	mu     sync.RWMutex // guards closed and sends on jobs
	closed bool
	jobs   chan notifyJob
	wg     sync.WaitGroup
}

type notifyJob struct {
	kind         string
	clientUserID uuid.UUID
	run          func(ctx context.Context) error
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
	baseCtx, cancelBase := context.WithCancel(context.Background())
	n := &Notifier{
		store:      store,
		sender:     sender,
		log:        log,
		baseCtx:    baseCtx,
		cancelBase: cancelBase,
		jobs:       make(chan notifyJob, notifyQueueCap),
	}
	if sender != nil {
		n.wg.Add(notifyWorkers)
		for i := 0; i < notifyWorkers; i++ {
			go n.worker()
		}
	}
	return n
}

func (n *Notifier) worker() {
	defer n.wg.Done()
	for job := range n.jobs {
		n.runJob(job)
	}
}

// runJob runs one job under a recover: workers have no Echo Recover
// middleware above them, so a panic would otherwise crash the process.
func (n *Notifier) runJob(job notifyJob) {
	// The request context is cancelled once the response is written, so the
	// job context hangs off baseCtx instead.
	ctx, cancel := context.WithTimeout(n.baseCtx, notifyJobTTL)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			n.log.Error("telegram notify job panicked", "kind", job.kind, "client_user_id", job.clientUserID, "panic", r)
		}
	}()
	if err := job.run(ctx); err != nil {
		n.log.Warn("telegram notify lookup failed", "kind", job.kind, "client_user_id", job.clientUserID, "error", err)
	}
}

func (n *Notifier) enqueue(kind string, clientUserID uuid.UUID, run func(ctx context.Context) error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.closed {
		return
	}
	select {
	case n.jobs <- notifyJob{kind: kind, clientUserID: clientUserID, run: run}:
	default:
		n.log.Warn("telegram notify queue full, dropping", "kind", kind, "client_user_id", clientUserID)
	}
}

// Close stops accepting jobs, lets the workers finish the queued ones and
// returns when they are done or ctx expires. When ctx expires first, the
// running and queued jobs are cancelled so they stop touching the database.
func (n *Notifier) Close(ctx context.Context) error {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	if !n.closed {
		n.closed = true
		close(n.jobs)
	}
	n.mu.Unlock()

	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		n.cancelBase()
		return nil
	case <-ctx.Done():
		n.cancelBase()
		return ctx.Err()
	}
}

func (n *Notifier) NotifyProgramAssigned(_ context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error {
	n.queueProgram("program_assigned", clientUserID, trainerID, programVersionID, assignedMessage)
	return nil
}

func (n *Notifier) NotifyProgramSynced(_ context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error {
	n.queueProgram("program_synced", clientUserID, trainerID, programVersionID, syncedMessage)
	return nil
}

func (n *Notifier) NotifyWorkoutCommented(_ context.Context, clientUserID, trainerID uuid.UUID, comment WorkoutComment) error {
	if n == nil || n.sender == nil {
		return nil
	}
	n.enqueue("workout_commented", clientUserID, func(ctx context.Context) error {
		return n.sendWorkoutCommented(ctx, clientUserID, trainerID, comment)
	})
	return nil
}

func (n *Notifier) queueProgram(
	kind string,
	clientUserID, trainerID, programVersionID uuid.UUID,
	format func(trainerName, programName string) string,
) {
	if n == nil || n.sender == nil {
		return
	}
	n.enqueue(kind, clientUserID, func(ctx context.Context) error {
		return n.sendProgram(ctx, clientUserID, trainerID, programVersionID, format)
	})
}

func (n *Notifier) sendWorkoutCommented(ctx context.Context, clientUserID, trainerID uuid.UUID, comment WorkoutComment) error {
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

	text := workoutCommentMessage(trainerName, comment)
	if err := n.sender.SendMessage(ctx, chatID, text); err != nil {
		n.log.Warn("telegram notify failed", "client_user_id", clientUserID, "error", err)
	}
	return nil
}

func (n *Notifier) sendProgram(
	ctx context.Context,
	clientUserID, trainerID, programVersionID uuid.UUID,
	format func(trainerName, programName string) string,
) error {
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
