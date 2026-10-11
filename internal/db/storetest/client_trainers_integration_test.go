//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/trainerclient"
)

const clientTrainersSecret = "test-jwt-secret-at-least-32-chars-long"

// insertLinkAt links a client to a trainer with a chosen created_at, so the
// order of the list does not depend on how fast the test runs.
func insertLinkAt(t *testing.T, pool *pgxpool.Pool, trainerUserID, clientUserID uuid.UUID, status string, createdAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status, created_at)
SELECT id, $2, $3, $4 FROM mentorix.trainers WHERE user_id = $1`, trainerUserID, clientUserID, status, createdAt); err != nil {
		t.Fatalf("insert %s link: %v", status, err)
	}
}

// getClientTrainers calls GET /client/trainers on the real handler, service and
// store with an access token for userID.
func getClientTrainers(t *testing.T, env appInviteEnv, userID uuid.UUID) (int, string) {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(clientTrainersSecret))
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	trainerclient.NewHandlers(env.svc, env.pool, clientTrainersSecret).Mount(e)

	req := httptest.NewRequest(http.MethodGet, "/client/trainers", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signed)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func trainersOf(t *testing.T, env appInviteEnv, userID uuid.UUID) []trainerclient.ClientTrainer {
	t.Helper()
	code, body := getClientTrainers(t, env, userID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", code, body)
	}
	var list trainerclient.ClientTrainerList
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return list.Items
}

func trainerIDOf(t *testing.T, env appInviteEnv, trainerUserID uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := env.store.TrainerIDForUser(context.Background(), trainerUserID)
	if err != nil {
		t.Fatalf("trainer id for %v: %v", trainerUserID, err)
	}
	return id
}

func TestClientTrainers_noLinksIsAnEmptyArray(t *testing.T) {
	env := newAppInviteEnv(t)
	client := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-none", "ct-none@test.com")

	code, body := getClientTrainers(t, env, client)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", code, body)
	}
	if body != `{"items":[]}` {
		t.Fatalf("body = %s, want exactly {\"items\":[]}", body)
	}
}

func TestClientTrainers_storeReturnsANonNilSliceWithoutLinks(t *testing.T) {
	env := newAppInviteEnv(t)
	client := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-store-none", "ct-store-none@test.com")

	got, err := env.store.ListClientTrainers(context.Background(), client)
	if err != nil {
		t.Fatalf("ListClientTrainers: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want a non-nil empty slice", got)
	}
}

// A query failure must reach the handler as an error. Swallowing it would turn a
// database outage into an empty list and send a client who has trainers to the
// invite screen.
func TestClientTrainers_storeSurfacesAQueryError(t *testing.T) {
	env := newAppInviteEnv(t)
	trainer := env.trainer(t, "ct-err-trainer@test.com", "Trainer")
	client := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-err", "ct-err@test.com")
	insertLinkAt(t, env.pool, trainer, client, "active", time.Now().UTC().Add(-time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := env.store.ListClientTrainers(ctx, client)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v (items %+v), want context.Canceled", err, got)
	}
}

func TestClientTrainers_listsActiveLinksNewestFirst(t *testing.T) {
	env := newAppInviteEnv(t)
	trainerA := env.trainer(t, "ct-order-a@test.com", "Trainer A")
	trainerB := env.trainer(t, "ct-order-b@test.com", "Trainer B")
	client := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-order", "ct-order@test.com")
	now := time.Now().UTC().Truncate(time.Microsecond)
	linkedA, linkedB := now.Add(-48*time.Hour), now.Add(-time.Hour)
	// The older link goes in first, so insertion order is the opposite of the expected order.
	insertLinkAt(t, env.pool, trainerA, client, "active", linkedA)
	insertLinkAt(t, env.pool, trainerB, client, "active", linkedB)

	code, body := getClientTrainers(t, env, client)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", code, body)
	}
	for _, linked := range []time.Time{linkedA, linkedB} {
		if want := `"linked_at":"` + linked.Format(time.RFC3339Nano) + `"`; !strings.Contains(body, want) {
			t.Errorf("body %s lacks %s", body, want)
		}
	}

	got := trainersOf(t, env, client)
	want := []trainerclient.ClientTrainer{
		{TrainerID: trainerIDOf(t, env, trainerB), TrainerDisplayName: "Trainer B", LinkedAt: linkedB},
		{TrainerID: trainerIDOf(t, env, trainerA), TrainerDisplayName: "Trainer A", LinkedAt: linkedA},
	}
	if len(got) != len(want) {
		t.Fatalf("items = %+v, want %d", got, len(want))
	}
	for i := range want {
		if got[i].TrainerID != want[i].TrainerID || got[i].TrainerDisplayName != want[i].TrainerDisplayName || !got[i].LinkedAt.Equal(want[i].LinkedAt) {
			t.Errorf("items[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// On a host in UTC the body prints "Z" with or without the conversion, so
	// check the location of the value the store returns.
	stored, err := env.store.ListClientTrainers(context.Background(), client)
	if err != nil {
		t.Fatalf("ListClientTrainers: %v", err)
	}
	for i, item := range stored {
		if item.LinkedAt.Location() != time.UTC {
			t.Errorf("stored[%d].LinkedAt location %q is not the time.UTC value, the store must call .UTC()", i, item.LinkedAt.Location())
		}
	}
}

func TestClientTrainers_sameLinkTimeIsOrderedByTrainerID(t *testing.T) {
	env := newAppInviteEnv(t)
	client := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-tie", "ct-tie@test.com")
	linked := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	type pair struct{ userID, trainerID uuid.UUID }
	var trainers []pair
	for _, email := range []string{"ct-tie-a@test.com", "ct-tie-b@test.com", "ct-tie-c@test.com"} {
		userID := env.trainer(t, email, "Trainer "+email)
		trainers = append(trainers, pair{userID, trainerIDOf(t, env, userID)})
	}
	// Insert in descending id order so the heap order differs from the expected one.
	sort.Slice(trainers, func(i, j int) bool { return trainers[i].trainerID.String() > trainers[j].trainerID.String() })
	for _, p := range trainers {
		insertLinkAt(t, env.pool, p.userID, client, "active", linked)
	}

	got := trainersOf(t, env, client)

	if len(got) != len(trainers) {
		t.Fatalf("items = %+v, want %d", got, len(trainers))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].TrainerID.String() >= got[i].TrainerID.String() {
			t.Fatalf("items = %+v, want ascending trainer_id for equal link times", got)
		}
	}
}

func TestClientTrainers_blockedLinkIsNotListed(t *testing.T) {
	env := newAppInviteEnv(t)
	active := env.trainer(t, "ct-blocked-active@test.com", "Active Trainer")
	blocked := env.trainer(t, "ct-blocked-blocked@test.com", "Blocked Trainer")
	mixed := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-mixed", "ct-mixed@test.com")
	onlyBlocked := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-only-blocked", "ct-only-blocked@test.com")
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertLinkAt(t, env.pool, active, mixed, "active", now.Add(-2*time.Hour))
	// The blocked link is the newer one, so a missing status filter would put it first.
	insertLinkAt(t, env.pool, blocked, mixed, "blocked", now.Add(-time.Hour))
	insertLinkAt(t, env.pool, blocked, onlyBlocked, "blocked", now.Add(-time.Hour))

	got := trainersOf(t, env, mixed)
	if len(got) != 1 || got[0].TrainerID != trainerIDOf(t, env, active) || got[0].TrainerDisplayName != "Active Trainer" {
		t.Fatalf("items = %+v, want only the active trainer", got)
	}

	code, body := getClientTrainers(t, env, onlyBlocked)
	if code != http.StatusOK || body != `{"items":[]}` {
		t.Fatalf("blocked client got %d %s, want 200 {\"items\":[]}", code, body)
	}
}

func TestClientTrainers_otherClientsLinksAreNotVisible(t *testing.T) {
	env := newAppInviteEnv(t)
	trainerOne := env.trainer(t, "ct-iso-one@test.com", "Trainer One")
	trainerTwo := env.trainer(t, "ct-iso-two@test.com", "Trainer Two")
	clientX := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-iso-x", "ct-iso-x@test.com")
	clientY := newClientWithIdentity(t, env.auth, auth.ProviderGoogle, "g-ct-iso-y", "ct-iso-y@test.com")
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertLinkAt(t, env.pool, trainerOne, clientX, "active", now.Add(-time.Hour))
	insertLinkAt(t, env.pool, trainerTwo, clientY, "active", now.Add(-time.Hour))

	for _, tt := range []struct {
		name    string
		caller  uuid.UUID
		trainer uuid.UUID
		display string
	}{
		{"x", clientX, trainerOne, "Trainer One"},
		{"y", clientY, trainerTwo, "Trainer Two"},
	} {
		got := trainersOf(t, env, tt.caller)
		if len(got) != 1 || got[0].TrainerID != trainerIDOf(t, env, tt.trainer) || got[0].TrainerDisplayName != tt.display {
			t.Errorf("client %s: items = %+v, want only %s", tt.name, got, tt.display)
		}
	}
}

func TestClientTrainers_aTrainerAccountSeesItsOwnTrainers(t *testing.T) {
	env := newAppInviteEnv(t)
	ctx := context.Background()
	trainerA := env.trainer(t, "ct-dual-a@test.com", "Trainer A")
	trainerB := env.trainer(t, "ct-dual-b@test.com", "Trainer B")
	accepted, err := env.store.AcceptInviteAsUser(ctx, env.invite(t, trainerA), trainerB)
	if err != nil {
		t.Fatalf("AcceptInviteAsUser: %v", err)
	}

	got := trainersOf(t, env, trainerB)
	if len(got) != 1 || got[0].TrainerID != accepted.TrainerID || got[0].TrainerDisplayName != "Trainer A" {
		t.Fatalf("items = %+v, want only Trainer A with the id the accept returned", got)
	}

	// Trainer A has a client but no trainer of its own.
	if own := trainersOf(t, env, trainerA); len(own) != 0 {
		t.Fatalf("trainer A items = %+v, want none", own)
	}
}

func TestClientTrainers_unknownUserGetsAnEmptyList(t *testing.T) {
	env := newAppInviteEnv(t)

	code, body := getClientTrainers(t, env, uuid.New())

	if code != http.StatusOK || body != `{"items":[]}` {
		t.Fatalf("got %d %s, want 200 {\"items\":[]}", code, body)
	}
}
