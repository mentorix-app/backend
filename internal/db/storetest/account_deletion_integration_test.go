//go:build integration

package storetest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/app"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

// deletionBase is a trainer with one published program, the content that a client
// deletion must leave alone.
type deletionBase struct {
	trainerUser uuid.UUID
	trainerID   uuid.UUID
	programID   uuid.UUID
	blockKey    uuid.UUID
}

func seedDeletionBase(t *testing.T, pool *pgxpool.Pool, emailPrefix string) deletionBase {
	t.Helper()
	ctx := context.Background()
	store := program.NewStore(pool)

	trainerUser, exerciseID := seedTrainerAndExercise(t, pool, emailPrefix)
	detail, err := store.CreateDraft(ctx, trainerUser)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week, day := detail.Weeks[0], detail.Weeks[0].Days[0]
	detail, err = createSingleBlockStore(ctx, store, trainerUser, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	published, err := store.PublishFromDraft(ctx, detail.ID, trainerUser, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	var trainerID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM mentorix.trainers WHERE user_id = $1`, trainerUser).Scan(&trainerID); err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	return deletionBase{
		trainerUser: trainerUser,
		trainerID:   trainerID,
		programID:   published.ID,
		blockKey:    published.Weeks[0].Days[0].Blocks[0].BlockKey,
	}
}

// clientData names the rows seedClientData created for one client.
type clientData struct {
	commentID   uuid.UUID
	inviteToken string
}

// seedClientData gives client a link to the trainer of base, an assignment, two
// workout completions with a trainer comment on the first, a block visibility rule,
// a refresh session, a link code and a consumed invite. tag must differ between calls.
func seedClientData(t *testing.T, pool *pgxpool.Pool, base deletionBase, client uuid.UUID, tag string) clientData {
	t.Helper()
	ctx := context.Background()
	exec := func(what, query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}

	exec("trainer link", `INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) VALUES ($1, $2)`, base.trainerID, client)
	programID := base.programID
	if _, err := program.NewStore(pool).SetClientProgramAssignment(ctx, base.trainerUser, base.trainerID, client, &programID); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	var assignmentID uuid.UUID
	if err := pool.QueryRow(ctx, `
SELECT id FROM mentorix.program_assignments WHERE client_user_id = $1 AND trainer_id = $2`, client, base.trainerID).Scan(&assignmentID); err != nil {
		t.Fatalf("assignment id: %v", err)
	}

	var firstCompletion uuid.UUID
	for i := range 2 {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
INSERT INTO mentorix.client_workout_completions
  (client_user_id, trainer_id, completed_at, program_id, program_assignment_id, completion_cycle_id,
   day_key, week_number, day_number, program_name, program_name_ru, day_snapshot, result_text, source)
VALUES ($1, $2, now(), $3, $4, $5, $6, 1, $7, 'Strength', 'Сила', '{}'::jsonb, 'done', 'telegram')
RETURNING id`, client, base.trainerID, base.programID, assignmentID, uuid.New(), uuid.New(), i+1).Scan(&id)
		if err != nil {
			t.Fatalf("seed completion: %v", err)
		}
		if i == 0 {
			firstCompletion = id
		}
	}
	var commentID uuid.UUID
	if err := pool.QueryRow(ctx, `
INSERT INTO mentorix.client_workout_completion_comments (client_workout_completion_id, trainer_id, comment_text)
VALUES ($1, $2, 'well done') RETURNING id`, firstCompletion, base.trainerID).Scan(&commentID); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	exec("block rule", `
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by)
VALUES ($1, $2, $3, $4)`, base.programID, base.blockKey, client, base.trainerUser)
	if err := auth.NewStore(pool).InsertRefreshSession(ctx, client, []byte("session-"+tag), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	exec("link code", `INSERT INTO mentorix.user_link_codes (user_id, code, expires_at) VALUES ($1, $2, now() + interval '1 hour')`, client, "code-"+tag)
	token := "invite-" + tag
	exec("consumed invite", `
INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at, consumed_at, consumed_by)
VALUES ($1, $2, now() + interval '1 day', now(), $3)`, base.trainerID, token, client)

	return clientData{commentID: commentID, inviteToken: token}
}

func addIdentity(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, provider, subject string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, $2, $3)`, userID, provider, subject); err != nil {
		t.Fatalf("add %s identity: %v", provider, err)
	}
}

// userReferences counts, for every column that references mentorix.users(id), the
// rows that hold userID. The keys are "table.column".
func userReferences(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) map[string]int {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
SELECT cl.relname, a.attname
FROM pg_constraint c
JOIN pg_class cl ON cl.oid = c.conrelid
JOIN pg_namespace n ON n.oid = cl.relnamespace
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
WHERE c.contype = 'f' AND n.nspname = 'mentorix' AND c.confrelid = 'mentorix.users'::regclass
ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("list references to users: %v", err)
	}
	type column struct{ table, name string }
	var columns []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.table, &c.name); err != nil {
			t.Fatalf("scan reference: %v", err)
		}
		columns = append(columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("list references to users: %v", err)
	}
	if len(columns) < 10 {
		t.Fatalf("found %d columns that reference users, the catalog query is wrong", len(columns))
	}

	out := make(map[string]int, len(columns))
	for _, c := range columns {
		query := fmt.Sprintf("SELECT count(*) FROM mentorix.%s WHERE %s = $1",
			pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize())
		out[c.table+"."+c.name] = countRows(t, pool, query, userID)
	}
	return out
}

// tableCounts counts the rows of every table in schema mentorix, so a table added
// later is part of the comparison without anyone listing it.
func tableCounts(t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'mentorix' AND table_type = 'BASE TABLE'
ORDER BY 1`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) < 20 {
		t.Fatalf("found %d tables in schema mentorix, the catalog query is wrong", len(tables))
	}
	out := make(map[string]int, len(tables))
	for _, name := range tables {
		out[name] = countRows(t, pool, "SELECT count(*) FROM mentorix."+pgx.Identifier{name}.Sanitize())
	}
	return out
}

// assertTableDeltas compares the row counts before and after with the expected
// change of each table. A table that is not named must not change.
func assertTableDeltas(t *testing.T, before, after map[string]int, want map[string]int) {
	t.Helper()
	for table := range want {
		if _, ok := before[table]; !ok {
			t.Errorf("expected delta names %q, which is not a table of schema mentorix", table)
		}
	}
	for table, was := range before {
		if got, wantDelta := after[table]-was, want[table]; got != wantDelta {
			t.Errorf("table %s changed by %d rows (%d -> %d), want %d", table, got, was, after[table], wantDelta)
		}
	}
}

// rowsFingerprint hashes the whole content of the rows of one user in one table, so
// an update shows up as well as a delete.
func rowsFingerprint(t *testing.T, pool *pgxpool.Pool, table, column string, userID uuid.UUID) string {
	t.Helper()
	query := fmt.Sprintf(`SELECT coalesce(md5(string_agg(x::text, '' ORDER BY x.id)), '') FROM mentorix.%s x WHERE x.%s = $1`,
		pgx.Identifier{table}.Sanitize(), pgx.Identifier{column}.Sanitize())
	var sum string
	if err := pool.QueryRow(context.Background(), query, userID).Scan(&sum); err != nil {
		t.Fatalf("fingerprint %s: %v", table, err)
	}
	if sum == "" {
		t.Fatalf("no rows in %s for %v, the fingerprint would prove nothing", table, userID)
	}
	return sum
}

func TestAccountDeletion_removesTheClientAndLeavesEverythingElse(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	second := seedDeletionBase(t, pool, "del-trainer-b")
	target := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-target", "target@test.com")
	if _, err := store.LinkTelegram(ctx, target, "tg-del-target"); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	targetData := seedClientData(t, pool, base, target, "target")
	seedClientData(t, pool, second, target, "target-b")
	neighbour := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-neighbour", "neighbour@test.com")
	seedClientData(t, pool, base, neighbour, "neighbour")
	seedClientData(t, pool, second, neighbour, "neighbour-b")

	before := userReferences(t, pool, target)
	for _, key := range []string{
		"auth_identities.user_id", "user_roles.user_id", "auth_refresh_sessions.user_id", "user_link_codes.user_id",
		"trainer_clients.client_user_id", "program_block_clients.client_user_id", "program_assignments.client_user_id",
		"client_workout_completions.client_user_id", "trainer_invites.consumed_by",
	} {
		if before[key] == 0 {
			t.Fatalf("the fixture holds no %s row for the client, so the test would prove nothing: %v", key, before)
		}
	}
	countsBefore := tableCounts(t, pool)
	completionsBefore := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour)
	assignmentsBefore := rowsFingerprint(t, pool, "program_assignments", "client_user_id", neighbour)

	telegramID, err := store.DeleteClientAccount(ctx, target, "")
	if err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}

	if telegramID != "tg-del-target" {
		t.Errorf("telegram id = %q, want tg-del-target", telegramID)
	}
	if userExists(t, pool, target) {
		t.Error("the user row is still there")
	}
	for key, n := range userReferences(t, pool, target) {
		if n != 0 {
			t.Errorf("%s still holds %d rows of the deleted client", key, n)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.client_workout_completion_comments WHERE id = $1`, targetData.commentID); n != 0 {
		t.Errorf("the trainer's comment on a deleted completion is still there")
	}
	// trainer_invites_consumed_check forbids a consumed invite without consumed_by, so
	// the invites the client used go with the client and no other invite is touched.
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_invites WHERE token = $1`, targetData.inviteToken); n != 0 {
		t.Errorf("the invite the client used is still there")
	}
	// The client held a link, an assignment, two completions (one with a comment), a
	// rule, a session, a link code and a consumed invite with each of the two trainers.
	assertTableDeltas(t, countsBefore, tableCounts(t, pool), map[string]int{
		"users":                              -1,
		"auth_identities":                    -2, // Google and Telegram
		"user_roles":                         -1,
		"auth_refresh_sessions":              -2,
		"user_link_codes":                    -2,
		"trainer_clients":                    -2,
		"program_assignments":                -2,
		"client_workout_completions":         -4,
		"client_workout_completion_comments": -2,
		"program_block_clients":              -2,
		"trainer_invites":                    -2,
	})
	if got := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour); got != completionsBefore {
		t.Error("the neighbour's workout completions changed")
	}
	if got := rowsFingerprint(t, pool, "program_assignments", "client_user_id", neighbour); got != assignmentsBefore {
		t.Error("the neighbour's program assignments changed")
	}
}

func TestAccountDeletion_clientWithoutTelegramReturnsAnEmptyID(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-1", "")

	telegramID, err := store.DeleteClientAccount(context.Background(), client, "")

	if err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}
	if telegramID != "" {
		t.Errorf("telegram id = %q, want empty", telegramID)
	}
	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
}

// Every refusal leaves the account, its rows and its roles as they were. The cases
// that must fail on the role rule have nothing else that would stop the delete.
func TestAccountDeletion_refusalsChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		want error
		// revoked is the Apple subject passed to the store.
		revoked string
		make    func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase) uuid.UUID
	}{
		{"admin", auth.ErrAccountDeletionUnavailable, "", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase) uuid.UUID {
			id := newTrainer(t, pool, "bare-admin@test.com", "Admin")
			promoteToAdminOnly(t, pool, id)
			return id
		}},
		{"no role row", auth.ErrAccountDeletionUnavailable, "", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase) uuid.UUID {
			id := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-no-role", "")
			seedClientData(t, pool, base, id, "no-role")
			if _, err := pool.Exec(context.Background(), `DELETE FROM mentorix.user_roles WHERE user_id = $1`, id); err != nil {
				t.Fatalf("drop roles: %v", err)
			}
			return id
		}},
		// A client whose Apple authorization is not revoked: the store refuses before it deletes anything.
		{"client with an Apple sign-in, no revoked subject", auth.ErrAppleSignInNotRevoked, "", withAppleSignIn},
		{"client with an Apple sign-in, another subject", auth.ErrAppleSignInNotRevoked, "a-someone-else", withAppleSignIn},
		{"client with an Apple sign-in, the subject in other case", auth.ErrAppleSignInNotRevoked, "A-CLIENT", withAppleSignIn},
		// Only an Apple identity counts: the Google subject of the same account does not.
		{"client with an Apple sign-in, the Google subject", auth.ErrAppleSignInNotRevoked, "g-with-apple", withAppleSignIn},
		{"client with a Telegram and an Apple sign-in, the Telegram id", auth.ErrAppleSignInNotRevoked, "tg-with-apple", withAppleSignIn},
		// A trainer without data is deletable, so the Apple rule applies to it like to a client.
		{"trainer with an Apple sign-in, no revoked subject", auth.ErrAppleSignInNotRevoked, "", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase) uuid.UUID {
			id := newTrainer(t, pool, "apple-trainer@test.com", "Coach")
			addIdentity(t, pool, id, auth.ProviderApple, "a-trainer")
			return id
		}},
		// The role and data rules come before the Apple rule even when the subject matches.
		{"admin with an Apple sign-in and its subject", auth.ErrAccountDeletionUnavailable, "a-admin", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, _ deletionBase) uuid.UUID {
			id := newTrainer(t, pool, "apple-admin@test.com", "Admin")
			promoteToAdminOnly(t, pool, id)
			addIdentity(t, pool, id, auth.ProviderApple, "a-admin")
			return id
		}},
		{"trainer with data, an Apple sign-in and its subject", auth.ErrAccountDeletionUnavailable, "a-trainer-data", func(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase) uuid.UUID {
			addIdentity(t, pool, base.trainerUser, auth.ProviderApple, "a-trainer-data")
			return base.trainerUser
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			ctx := context.Background()
			base := seedDeletionBase(t, pool, "del-trainer")
			account := tt.make(t, pool, store, base)
			roles, err := store.UserRoles(ctx, account)
			if err != nil {
				t.Fatalf("UserRoles: %v", err)
			}
			references := userReferences(t, pool, account)
			counts := tableCounts(t, pool)

			telegramID, err := store.DeleteClientAccount(ctx, account, tt.revoked)

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			for _, other := range []error{auth.ErrAccountDeletionUnavailable, auth.ErrAppleSignInNotRevoked} {
				if other != tt.want && errors.Is(err, other) {
					t.Errorf("error %v is also %v, the two refusals must differ", err, other)
				}
			}
			if telegramID != "" {
				t.Errorf("telegram id = %q, want empty", telegramID)
			}
			if !userExists(t, pool, account) {
				t.Error("a refused deletion removed the user")
			}
			if got := userReferences(t, pool, account); !reflect.DeepEqual(got, references) {
				t.Errorf("rows changed\nbefore: %v\nafter:  %v", references, got)
			}
			assertTableDeltas(t, counts, tableCounts(t, pool), nil)
			if got, err := store.UserRoles(ctx, account); err != nil || !reflect.DeepEqual(got, roles) {
				t.Errorf("roles = %v (%v), want %v", got, err, roles)
			}
		})
	}
}

// withAppleSignIn is a client with Google, Apple (subject a-client) and Telegram
// sign-ins and a full set of data.
func withAppleSignIn(t *testing.T, pool *pgxpool.Pool, store *auth.Store, base deletionBase) uuid.UUID {
	t.Helper()
	id := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-with-apple", "")
	addIdentity(t, pool, id, auth.ProviderApple, "a-client")
	if _, err := store.LinkTelegram(context.Background(), id, "tg-with-apple"); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	seedClientData(t, pool, base, id, "with-apple")
	return id
}

// A client who authored a program cannot be deleted: the database refuses the user
// row. The refusal rolls back the completions and assignments deleted before it.
func TestAccountDeletion_foreignKeyViolationRollsBackAndIsUnavailable(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-author", "")
	seedClientData(t, pool, base, client, "author")
	if _, err := pool.Exec(ctx, `INSERT INTO mentorix.programs (created_by, modified_by) VALUES ($1, $1)`, client); err != nil {
		t.Fatalf("insert restricting row: %v", err)
	}
	before := userReferences(t, pool, client)
	counts := tableCounts(t, pool)

	_, err := store.DeleteClientAccount(ctx, client, "")

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	if !userExists(t, pool, client) {
		t.Error("the user row was removed")
	}
	if got := userReferences(t, pool, client); !reflect.DeepEqual(got, before) {
		t.Errorf("rows changed, the transaction did not roll back\nbefore: %v\nafter:  %v", before, got)
	}
	assertTableDeltas(t, counts, tableCounts(t, pool), nil)
}

func TestAccountDeletion_unknownAndRepeatedCallsAreNoRows(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	if _, err := store.DeleteClientAccount(ctx, uuid.New(), ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown id: error = %v, want pgx.ErrNoRows", err)
	}

	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-twice", "")
	if _, err := store.DeleteClientAccount(ctx, client, ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := store.DeleteClientAccount(ctx, client, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second call: error = %v, want pgx.ErrNoRows", err)
	}
}

// deleteWithoutRevocation is the store deletion of client with no revoked Apple subject.
func deleteWithoutRevocation(store *auth.Store, client uuid.UUID) func() error {
	return func() error {
		_, err := store.DeleteClientAccount(context.Background(), client, "")
		return err
	}
}

// lockWaiters counts the other sessions of the test database that wait for a lock.
func lockWaiters(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countRows(t, pool, `
SELECT count(*) FROM pg_stat_activity
WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`)
}

// waitForLockWait returns once another session of the test database waits for a lock.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if lockWaiters(t, pool) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no session waited for a lock within 5 seconds")
}

// deleteBlockedBy runs statement in a transaction that stays open, starts deletion,
// waits until it waits for a lock, commits the statement and returns the result of the
// deletion. The statement has to touch the client's user row so that the deletion has
// something to wait for.
func deleteBlockedBy(t *testing.T, pool *pgxpool.Pool, deletion func() error, statement string, args ...any) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, statement, args...); err != nil {
		t.Fatalf("hold statement: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- deletion() }()
	waitForLockWait(t, pool)
	select {
	case err := <-done:
		t.Fatalf("the deletion finished while the other transaction was open: %v", err)
	default:
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit the held statement: %v", err)
	}

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the deletion did not finish after the other transaction committed")
		return nil
	}
}

// An assignment that a trainer is still committing holds a share lock on the
// client's user row. The deletion waits for it on the row lock, then sees the
// committed assignment and deletes it too. Without the lock the user delete would
// find the assignment late and the database would refuse it.
func TestAccountDeletion_waitsForAnAssignmentInFlight(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-race", "")
	var versionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM mentorix.program_versions WHERE program_id = $1`, base.programID).Scan(&versionID); err != nil {
		t.Fatalf("version id: %v", err)
	}

	err := deleteBlockedBy(t, pool, deleteWithoutRevocation(store, client), `
INSERT INTO mentorix.program_assignments (program_id, program_version_id, trainer_id, client_user_id, created_by, modified_by)
VALUES ($1, $2, $3, $4, $5, $5)`, base.programID, versionID, base.trainerID, client, base.trainerUser)

	if err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}
	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.program_assignments WHERE client_user_id = $1`, client); n != 0 {
		t.Errorf("assignments of the deleted client = %d, want 0", n)
	}
}

// A trainer side that appears while the deletion waits for the lock must be seen: the roles
// are read after the lock, so a client who became a trainer and got a client of their own
// is refused and nothing is deleted. The held statement does what the app does for a client
// who becomes a trainer, then adds the first client.
func TestAccountDeletion_readsRolesAfterTheLock(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-role-race", "")
	seedClientData(t, pool, base, client, "role-race")
	invitee := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-role-race-invitee", "")
	completions := countRows(t, pool, `SELECT count(*) FROM mentorix.client_workout_completions WHERE client_user_id = $1`, client)
	assignments := countRows(t, pool, `SELECT count(*) FROM mentorix.program_assignments WHERE client_user_id = $1`, client)

	err := deleteBlockedBy(t, pool, deleteWithoutRevocation(store, client), `
WITH role AS (INSERT INTO mentorix.user_roles (user_id, role) VALUES ($1, 'trainer')),
     trainer AS (INSERT INTO mentorix.trainers (user_id) VALUES ($1) RETURNING id)
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id) SELECT id, $2 FROM trainer`, client, invitee)

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	if !userExists(t, pool, client) {
		t.Fatal("the user row was removed")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM mentorix.trainer_clients WHERE client_user_id = $1`, invitee); n != 1 {
		t.Errorf("the trainer's new client link = %d rows, want 1", n)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM mentorix.client_workout_completions WHERE client_user_id = $1`, client); got != completions {
		t.Errorf("completions = %d, want %d", got, completions)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM mentorix.program_assignments WHERE client_user_id = $1`, client); got != assignments {
		t.Errorf("assignments = %d, want %d", got, assignments)
	}
}

// An Apple sign-in that arrives while the deletion waits for the lock must be seen:
// it is read after the lock, so the account is refused instead of being deleted
// without a revocation. The held statement is the one a Telegram link runs
// (MoveSignInIdentitiesToUser): it moves the Apple identity of another account onto
// the client.
func TestAccountDeletion_readsAppleSignInAfterTheLock(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-del-apple-race", "")
	seedClientData(t, pool, base, client, "apple-race")
	other := newClientWithIdentity(t, store, auth.ProviderApple, "a-race", "")
	references := userReferences(t, pool, client)

	err := deleteBlockedBy(t, pool, deleteWithoutRevocation(store, client),
		`UPDATE mentorix.auth_identities SET user_id = $1 WHERE user_id = $2 AND provider = 'apple'`, client, other)

	if !errors.Is(err, auth.ErrAppleSignInNotRevoked) {
		t.Fatalf("error = %v, want ErrAppleSignInNotRevoked", err)
	}
	if !userExists(t, pool, client) {
		t.Fatal("the user row was removed")
	}
	if owner, ok := identityOwner(t, pool, auth.ProviderApple, "a-race"); !ok || owner != client {
		t.Errorf("moved Apple identity owner = %v, %v, want the client %v", owner, ok, client)
	}
	references["auth_identities.user_id"]++ // the moved Apple identity committed
	if got := userReferences(t, pool, client); !reflect.DeepEqual(got, references) {
		t.Errorf("rows changed\nbefore: %v\nafter:  %v", references, got)
	}
}

// mountedEnv is app.Mount over the test database and a fake Redis, with the real
// auth service and store behind DELETE /auth/me.
type mountedEnv struct {
	e          *echo.Echo
	pool       *pgxpool.Pool
	store      *auth.Store
	rdb        *redis.Client
	secret     string
	cookieName string
}

// newMountedEnv mounts the API on the test database. Each option edits the config.
func newMountedEnv(t *testing.T, options ...func(*config.Config)) mountedEnv {
	t.Helper()
	pool := NewPool(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	const secret = "mount-deletion-jwt-secret-min-32-chars"
	const cookieName = "mentorix_refresh"
	cfg := config.Config{
		JWTSecret:            secret,
		AccessTTLMinutes:     15,
		RefreshTTLDays:       30,
		TrainerInviteTTLDays: 7,
		RefreshCookie:        config.RefreshCookieSettings{Name: cookieName, Path: "/auth"},
	}
	for _, option := range options {
		option(&cfg)
	}
	e := echo.New()
	app.Mount(e, app.Deps{
		Config: cfg,
		Pool:   pool,
		Redis:  rdb,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return mountedEnv{e: e, pool: pool, store: auth.NewStore(pool), rdb: rdb, secret: secret, cookieName: cookieName}
}

func (m mountedEnv) accessToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(m.secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func (m mountedEnv) do(method, path, accessToken, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if accessToken != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+accessToken)
	}
	rec := httptest.NewRecorder()
	m.e.ServeHTTP(rec, req)
	return rec
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body.String())
	}
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body.String())
	}
	if body.Code != code {
		t.Fatalf("code = %q, want %q; body = %s", body.Code, code, rec.Body.String())
	}
}

// DELETE /auth/me through app.Mount, with the real store and a Redis: the bot's
// active_trainer key of the deleted Telegram goes away, which only the wiring in
// Mount can do.
func TestAccountDeletion_mountedRouteRemovesTheActiveTrainerKey(t *testing.T) {
	m := newMountedEnv(t)
	ctx := context.Background()
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-del", "")
	other := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-other", "")
	for id, telegram := range map[uuid.UUID]string{client: "tg-mount-del", other: "tg-mount-other"} {
		if _, err := m.store.LinkTelegram(ctx, id, telegram); err != nil {
			t.Fatalf("link telegram %s: %v", telegram, err)
		}
	}
	active := trainerclient.NewActiveTrainerStore(m.rdb)
	for _, telegram := range []string{"tg-mount-del", "tg-mount-other"} {
		if err := active.Set(ctx, telegram, uuid.New()); err != nil {
			t.Fatalf("set active trainer: %v", err)
		}
	}
	token := m.accessToken(t, client)

	rec := m.do(http.MethodDelete, "/auth/me", token, "")

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, body = %q, want 204 and no body", rec.Code, rec.Body.String())
	}
	if userExists(t, m.pool, client) {
		t.Error("the user row is still there")
	}
	if _, found, err := active.Get(ctx, "tg-mount-del"); err != nil || found {
		t.Errorf("active trainer of the deleted Telegram: found = %v, error = %v, want removed", found, err)
	}
	if _, found, err := active.Get(ctx, "tg-mount-other"); err != nil || !found {
		t.Errorf("active trainer of another Telegram: found = %v, error = %v, want kept", found, err)
	}
	if !userExists(t, m.pool, other) {
		t.Error("another account was removed")
	}
	assertErrorBody(t, m.do(http.MethodDelete, "/auth/me", token, ""), http.StatusNotFound, "user_not_found")
}

// After a 204 the old tokens are useless: the access token finds no user and the
// refresh token no session.
func TestAccountDeletion_mountedRouteEndsEverySession(t *testing.T) {
	m := newMountedEnv(t)
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-session", "")
	const refresh = "a-refresh-token-of-the-deleted-client"
	sum := sha256.Sum256([]byte(refresh))
	if err := m.store.InsertRefreshSession(context.Background(), client, sum[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	token := m.accessToken(t, client)
	if rec := m.do(http.MethodGet, "/auth/me", token, ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me before the deletion = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	if rec := m.do(http.MethodDelete, "/auth/me", token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /auth/me = %d, want 204; body = %s", rec.Code, rec.Body.String())
	}

	assertErrorBody(t, m.do(http.MethodGet, "/auth/me", token, ""), http.StatusNotFound, "user_not_found")
	assertErrorBody(t, m.do(http.MethodPost, "/auth/refresh", "", `{"refresh_token":"`+refresh+`"}`), http.StatusUnauthorized, "invalid_refresh_token")
}

// The refresh cookie is not a credential for this endpoint: a valid session cookie
// without a Bearer token must not delete anything.
func TestAccountDeletion_mountedRouteIgnoresTheRefreshCookie(t *testing.T) {
	m := newMountedEnv(t)
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-cookie", "")
	const refresh = "a-live-refresh-token"
	sum := sha256.Sum256([]byte(refresh))
	if err := m.store.InsertRefreshSession(context.Background(), client, sum[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: m.cookieName, Value: refresh})
	rec := httptest.NewRecorder()

	m.e.ServeHTTP(rec, req)

	assertErrorBody(t, rec, http.StatusUnauthorized, "missing_authorization")
	if !userExists(t, m.pool, client) {
		t.Error("the user row was removed")
	}
	if n := countRows(t, m.pool, `SELECT count(*) FROM mentorix.auth_refresh_sessions WHERE user_id = $1`, client); n != 1 {
		t.Errorf("refresh sessions = %d, want 1", n)
	}
}

func TestAccountDeletion_mountedRouteRefusesAnAppleAccount(t *testing.T) {
	m := newMountedEnv(t)
	base := seedDeletionBase(t, m.pool, "del-trainer")
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-mount-apple", "")
	addIdentity(t, m.pool, client, auth.ProviderApple, "a-mount-apple")
	seedClientData(t, m.pool, base, client, "mount-apple")
	references := userReferences(t, m.pool, client)
	counts := tableCounts(t, m.pool)

	rec := m.do(http.MethodDelete, "/auth/me", m.accessToken(t, client), "")

	assertErrorBody(t, rec, http.StatusServiceUnavailable, "provider_not_configured")
	if got := userReferences(t, m.pool, client); !reflect.DeepEqual(got, references) {
		t.Errorf("rows changed\nbefore: %v\nafter:  %v", references, got)
	}
	assertTableDeltas(t, counts, tableCounts(t, m.pool), nil)
}

func TestAccountDeletion_mountedRouteRefusesATrainer(t *testing.T) {
	m := newMountedEnv(t)
	base := seedDeletionBase(t, m.pool, "del-trainer")
	references := userReferences(t, m.pool, base.trainerUser)
	counts := tableCounts(t, m.pool)

	rec := m.do(http.MethodDelete, "/auth/me", m.accessToken(t, base.trainerUser), "")

	assertErrorBody(t, rec, http.StatusConflict, "account_deletion_unavailable")
	if got := userReferences(t, m.pool, base.trainerUser); !reflect.DeepEqual(got, references) {
		t.Errorf("rows changed\nbefore: %v\nafter:  %v", references, got)
	}
	assertTableDeltas(t, counts, tableCounts(t, m.pool), nil)
}
