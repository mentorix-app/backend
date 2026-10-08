package telegramnotify_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/telegramnotify"
)

// Mirrors the unexported pool limits in notifier.go.
const (
	testWorkers  = 4
	testQueueCap = 256
)

// gatedSender blocks every send until release is closed and records how many
// sends run at once.
type gatedSender struct {
	entered chan struct{}
	release chan struct{}

	running atomic.Int32
	maxSeen atomic.Int32
	total   atomic.Int32
}

func newGatedSender() *gatedSender {
	return &gatedSender{entered: make(chan struct{}, 1024), release: make(chan struct{})}
}

func (g *gatedSender) SendMessage(context.Context, int64, string) error {
	cur := g.running.Add(1)
	for {
		prev := g.maxSeen.Load()
		if cur <= prev || g.maxSeen.CompareAndSwap(prev, cur) {
			break
		}
	}
	g.entered <- struct{}{}
	<-g.release
	g.running.Add(-1)
	g.total.Add(1)
	return nil
}

const testWait = 5 * time.Second

// waitEntered fails fast instead of hanging when a send never starts.
func waitEntered(t *testing.T, sender *gatedSender) {
	t.Helper()
	select {
	case <-sender.entered:
	case <-time.After(testWait):
		t.Fatal("sender was not reached")
	}
}

func okStore() *fakeNotifyStore {
	return &fakeNotifyStore{
		subject:     "42",
		trainerName: "Иван",
		version:     sqlc.GetProgramVersionDisplayByIDRow{NameRu: "Сила"},
	}
}

func notifyOne(n *telegramnotify.Notifier) error {
	return n.NotifyProgramSynced(context.Background(), uuid.New(), uuid.New(), uuid.New())
}

func TestNotifier_notifyReturnsBeforeSenderRuns(t *testing.T) {
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)

	returned := make(chan error, 1)
	go func() { returned <- notifyOne(n) }()

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("notify: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notify blocked on the sender")
	}

	waitEntered(t, sender) // the job does reach the sender, which is still blocked
	close(sender.release)
	drain(t, n)
	if got := sender.total.Load(); got != 1 {
		t.Fatalf("delivered = %d, want 1", got)
	}
}

func TestNotifier_allJobsDeliveredWithBoundedConcurrency(t *testing.T) {
	const jobs = 40
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)

	for i := 0; i < jobs; i++ {
		if err := notifyOne(n); err != nil {
			t.Fatalf("notify: %v", err)
		}
	}
	// All workers are busy before anything is released.
	for i := 0; i < testWorkers; i++ {
		waitEntered(t, sender)
	}
	close(sender.release)
	drain(t, n)

	if got := sender.total.Load(); got != jobs {
		t.Fatalf("delivered = %d, want %d", got, jobs)
	}
	if got := sender.maxSeen.Load(); got != testWorkers {
		t.Fatalf("max concurrent sends = %d, want %d", got, testWorkers)
	}
}

func TestNotifier_fullQueueDropsWithWarning(t *testing.T) {
	log, logs := newTestLogger()
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, log)

	for i := 0; i < testWorkers; i++ {
		if err := notifyOne(n); err != nil {
			t.Fatalf("notify: %v", err)
		}
	}
	for i := 0; i < testWorkers; i++ {
		waitEntered(t, sender)
	}
	for i := 0; i < testQueueCap; i++ {
		if err := notifyOne(n); err != nil {
			t.Fatalf("notify: %v", err)
		}
	}
	if strings.Contains(logs.String(), "queue full") {
		t.Fatalf("dropped before the queue was full: %q", logs.String())
	}

	extra := make(chan error, 1)
	go func() { extra <- notifyOne(n) }()
	select {
	case err := <-extra:
		if err != nil {
			t.Fatalf("notify: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notify blocked on a full queue")
	}
	if got := strings.Count(logs.String(), "queue full"); got != 1 {
		t.Fatalf("queue full warnings = %d, want 1; logs = %q", got, logs.String())
	}
	if !strings.Contains(logs.String(), "program_synced") || !strings.Contains(logs.String(), "client_user_id") {
		t.Fatalf("warning lacks kind or client id: %q", logs.String())
	}

	close(sender.release)
	drain(t, n)
	if got := sender.total.Load(); got != testWorkers+testQueueCap {
		t.Fatalf("delivered = %d, want %d (the extra job is dropped)", got, testWorkers+testQueueCap)
	}
}

func TestNotifier_closeWaitsForQueuedJobs(t *testing.T) {
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)
	for i := 0; i < 10; i++ {
		_ = notifyOne(n)
	}
	waitEntered(t, sender)

	closed := make(chan error, 1)
	go func() { closed <- n.Close(context.Background()) }()

	// Close has not returned while the sender is still blocked.
	select {
	case err := <-closed:
		t.Fatalf("Close returned before jobs finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(sender.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(testWait):
		t.Fatal("Close did not return after the jobs were released")
	}
	if got := sender.total.Load(); got != 10 {
		t.Fatalf("delivered = %d, want 10", got)
	}
}

// A separate notifier: Close timing out cancels the jobs, so the drain
// assertion above must not share an instance with this one.
func TestNotifier_closeWithShortDeadlineWhileSenderBlocked(t *testing.T) {
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)
	_ = notifyOne(n)
	waitEntered(t, sender)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := n.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v, want context.DeadlineExceeded", err)
	}

	close(sender.release)
	drain(t, n)
	if got := sender.total.Load(); got != 1 {
		t.Fatalf("delivered = %d, want 1", got)
	}
}

func TestNotifier_closeWithExpiredContextReturnsContextError(t *testing.T) {
	sender := newGatedSender()
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)
	_ = notifyOne(n)
	waitEntered(t, sender)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := n.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close = %v, want context.Canceled", err)
	}

	close(sender.release)
	drain(t, n) // a second Close still waits for the workers
}

func TestNotifier_notifyAfterCloseDoesNothing(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, nil)
	drain(t, n)

	if err := notifyOne(n); err != nil {
		t.Fatalf("NotifyProgramSynced after Close: %v", err)
	}
	err := n.NotifyWorkoutCommented(context.Background(), uuid.New(), uuid.New(), telegramnotify.WorkoutComment{CommentText: "ok"})
	if err != nil {
		t.Fatalf("NotifyWorkoutCommented after Close: %v", err)
	}
	drain(t, n)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
}

func TestNotifier_concurrentNotifyAndClose(t *testing.T) {
	n := telegramnotify.NewNotifierWithStore(okStore(), &recordingSender{}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = notifyOne(n)
			}
		}()
	}
	drain(t, n)
	wg.Wait()
}

func TestNotifier_noGoroutineOutlivesClose(t *testing.T) {
	before := runtime.NumGoroutine()
	n := telegramnotify.NewNotifierWithStore(okStore(), &recordingSender{}, nil)
	for i := 0; i < 20; i++ {
		_ = notifyOne(n)
	}
	drain(t, n)

	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines = %d, want <= %d", runtime.NumGoroutine(), before)
		}
		runtime.Gosched()
	}
}

func TestNotifier_closeNilIsNoop(t *testing.T) {
	var n *telegramnotify.Notifier
	if err := n.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// panicSender panics on its first call and records later ones.
type panicSender struct {
	calls     atomic.Int32
	delivered atomic.Int32
}

func (p *panicSender) SendMessage(context.Context, int64, string) error {
	if p.calls.Add(1) == 1 {
		panic("boom")
	}
	p.delivered.Add(1)
	return nil
}

func TestNotifier_panicInJobIsLoggedAndWorkerContinues(t *testing.T) {
	log, logs := newTestLogger()
	sender := &panicSender{}
	n := telegramnotify.NewNotifierWithStore(okStore(), sender, log)

	firstClient := uuid.New()
	if err := n.NotifyProgramSynced(context.Background(), firstClient, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	// Wait for the panicking job so the second one is the "next job".
	deadline := time.Now().Add(testWait)
	for !strings.Contains(logs.String(), "boom") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not logged: %q", logs.String())
		}
		runtime.Gosched()
	}
	if err := notifyOne(n); err != nil {
		t.Fatalf("notify: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	if err := n.Close(ctx); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
	if got := sender.delivered.Load(); got != 1 {
		t.Fatalf("delivered after the panic = %d, want 1", got)
	}
	out := logs.String()
	for _, want := range []string{"level=ERROR", "program_synced", firstClient.String(), "boom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q: %q", want, out)
		}
	}
}

// ctxBlockingStore blocks the first lookup until its ctx is done.
type ctxBlockingStore struct {
	fakeNotifyStore
	entered  chan struct{}
	returned chan error
}

func (s *ctxBlockingStore) GetTelegramSubjectByUserID(ctx context.Context, _ sqlc.GetTelegramSubjectByUserIDParams) (string, error) {
	s.entered <- struct{}{}
	<-ctx.Done()
	s.returned <- ctx.Err()
	return "", ctx.Err()
}

func TestNotifier_closeTimeoutCancelsInFlightJobs(t *testing.T) {
	store := &ctxBlockingStore{entered: make(chan struct{}, 1), returned: make(chan error, 1)}
	n := telegramnotify.NewNotifierWithStore(store, &recordingSender{}, nil)
	_ = notifyOne(n)
	select {
	case <-store.entered:
	case <-time.After(testWait):
		t.Fatal("lookup was not reached")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := n.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close = %v, want context.Canceled", err)
	}

	select {
	case err := <-store.returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lookup err = %v, want context.Canceled", err)
		}
	case <-time.After(testWait):
		t.Fatal("lookup kept running after Close timed out")
	}
	drain(t, n) // the workers exit once the job is cancelled
}
