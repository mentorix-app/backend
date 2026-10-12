//go:build integration

package storetest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	httpx "mentorix-backend/internal/http"
)

const (
	appleAudience    = "com.mentorix.mobile"
	appleCode        = "apple-code-from-the-app"
	appleRedirectURI = "https://api.mentorix.example/auth/apple/callback"
)

// scriptedVerifier answers every token with the same claims or error.
type scriptedVerifier struct {
	claims auth.IDTokenClaims
	err    error
}

func (v scriptedVerifier) Verify(context.Context, string) (auth.IDTokenClaims, error) {
	return v.claims, v.err
}

// scriptedRevoker records its calls and whether the user still existed at each one.
type scriptedRevoker struct {
	mu          sync.Mutex
	err         error
	calls       []string
	userExisted []bool
	check       func() bool
}

func (r *scriptedRevoker) Revoke(_ context.Context, clientID, subject, code, redirectURI string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, strings.Join([]string{clientID, subject, code, redirectURI}, "|"))
	if r.check != nil {
		r.userExisted = append(r.userExisted, r.check())
	}
	return r.err
}

func (r *scriptedRevoker) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// appleClient creates a client with Google, Apple and Telegram sign-ins and a full set of
// data. extraApple lists more Apple subjects for the same account.
func appleClient(t *testing.T, pool *pgxpool.Pool, base deletionBase, tag, appleSubject string, extraApple ...string) uuid.UUID {
	t.Helper()
	store := auth.NewStore(pool)
	id := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-"+tag, "")
	for _, subject := range append([]string{appleSubject}, extraApple...) {
		addIdentity(t, pool, id, auth.ProviderApple, subject)
	}
	if _, err := store.LinkTelegram(context.Background(), id, "tg-"+tag); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	seedClientData(t, pool, base, id, tag)
	return id
}

// deletionService is the real auth service over the test database with the given
// Apple collaborators.
func deletionService(pool *pgxpool.Pool, verifier auth.IDTokenVerifier, revoker auth.AppleRevoker) *auth.Service {
	return auth.NewService(pool, "integration-test-jwt-secret-32chars", 15*time.Minute, 30*24*time.Hour,
		auth.WithAppleVerifier(verifier), auth.WithAppleRevoker(revoker))
}

func TestAccountDeletion_storeDeletesAnAppleAccountWhoseSubjectWasRevoked(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := appleClient(t, pool, base, "apple-ok", "a-ok")
	neighbour := appleClient(t, pool, base, "apple-neighbour", "a-neighbour")
	neighbourBefore := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour)

	telegramID, err := store.DeleteClientAccount(context.Background(), client, "a-ok")

	if err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}
	if telegramID != "tg-apple-ok" {
		t.Errorf("telegram id = %q, want tg-apple-ok", telegramID)
	}
	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
	for key, n := range userReferences(t, pool, client) {
		if n != 0 {
			t.Errorf("%s still holds %d rows of the deleted client", key, n)
		}
	}
	if _, ok := identityOwner(t, pool, auth.ProviderApple, "a-ok"); ok {
		t.Error("the Apple identity is still there")
	}
	if !userExists(t, pool, neighbour) {
		t.Error("another account was removed")
	}
	if got := rowsFingerprint(t, pool, "client_workout_completions", "client_user_id", neighbour); got != neighbourBefore {
		t.Error("the neighbour's workout completions changed")
	}
}

// The known limit: one revoked Apple identity is enough, the other goes with the user.
func TestAccountDeletion_storeDeletesAnAccountWithTwoAppleIdentitiesWhenOneWasRevoked(t *testing.T) {
	for _, revoked := range []string{"a-first", "a-second"} {
		t.Run("revoked "+revoked, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			base := seedDeletionBase(t, pool, "del-trainer")
			client := appleClient(t, pool, base, "two-apple", "a-first", "a-second")

			if _, err := store.DeleteClientAccount(context.Background(), client, revoked); err != nil {
				t.Fatalf("DeleteClientAccount() error = %v", err)
			}

			if userExists(t, pool, client) {
				t.Error("the user row is still there")
			}
			for _, subject := range []string{"a-first", "a-second"} {
				if _, ok := identityOwner(t, pool, auth.ProviderApple, subject); ok {
					t.Errorf("the Apple identity %s is still there", subject)
				}
			}
		})
	}
}

func TestAccountDeletion_storeIgnoresARevokedSubjectOfAnAccountWithoutApple(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-no-apple", "")

	if _, err := store.DeleteClientAccount(context.Background(), client, "a-from-the-token"); err != nil {
		t.Fatalf("DeleteClientAccount() error = %v", err)
	}

	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
}

func TestAccountDeletion_storeListsTheAppleSubjectsOfAnAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-list", "")
	addIdentity(t, pool, client, auth.ProviderApple, "a-2")
	addIdentity(t, pool, client, auth.ProviderApple, "a-1")
	if _, err := store.LinkTelegram(context.Background(), client, "tg-list"); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	withoutApple := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-list-none", "")

	got, err := store.UserAppleSubjects(context.Background(), client)
	if err != nil {
		t.Fatalf("UserAppleSubjects() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a-1", "a-2"}) {
		t.Errorf("subjects = %v, want only the Apple ones, [a-1 a-2]", got)
	}
	for name, id := range map[string]uuid.UUID{"no Apple sign-in": withoutApple, "unknown user": uuid.New()} {
		got, err := store.UserAppleSubjects(context.Background(), id)
		if err != nil || len(got) != 0 {
			t.Errorf("%s: subjects = %v, error = %v, want none", name, got, err)
		}
	}
}

// snapshot is what a refused deletion must leave as it was.
type snapshot struct {
	references map[string]int
	counts     map[string]int
}

func takeSnapshot(t *testing.T, pool *pgxpool.Pool, client uuid.UUID) snapshot {
	t.Helper()
	return snapshot{references: userReferences(t, pool, client), counts: tableCounts(t, pool)}
}

func (s snapshot) assertUnchanged(t *testing.T, pool *pgxpool.Pool, client uuid.UUID) {
	t.Helper()
	if !userExists(t, pool, client) {
		t.Error("a refused deletion removed the user")
	}
	if got := userReferences(t, pool, client); !reflect.DeepEqual(got, s.references) {
		t.Errorf("rows changed\nbefore: %v\nafter:  %v", s.references, got)
	}
	assertTableDeltas(t, s.counts, tableCounts(t, pool), nil)
}

func validAppleClaims(subject string) auth.IDTokenClaims {
	return auth.IDTokenClaims{Subject: subject, Audience: appleAudience}
}

func TestAccountDeletion_serviceRevokesAtAppleBeforeItDeletes(t *testing.T) {
	pool := NewPool(t)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := appleClient(t, pool, base, "svc-ok", "a-svc-ok")
	revoker := &scriptedRevoker{}
	revoker.check = func() bool { return userExists(t, pool, client) }
	svc := deletionService(pool, scriptedVerifier{claims: validAppleClaims("a-svc-ok")}, revoker)

	err := svc.DeleteAccount(context.Background(), client, auth.AppleProof{IDToken: "id-token", AuthorizationCode: appleCode, RedirectURI: appleRedirectURI})

	if err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	wantCall := strings.Join([]string{appleAudience, "a-svc-ok", appleCode, appleRedirectURI}, "|")
	if revoker.callCount() != 1 || revoker.calls[0] != wantCall {
		t.Fatalf("revoker calls = %q, want exactly [%q]", revoker.calls, wantCall)
	}
	if !reflect.DeepEqual(revoker.userExisted, []bool{true}) {
		t.Errorf("the user existed at the revocation = %v, want [true]: the revocation comes before the delete", revoker.userExisted)
	}
	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
	for key, n := range userReferences(t, pool, client) {
		if n != 0 {
			t.Errorf("%s still holds %d rows of the deleted client", key, n)
		}
	}
}

func TestAccountDeletion_serviceDeletesNothingWhenAppleDoesNotConfirm(t *testing.T) {
	tests := []struct {
		name     string
		verifier scriptedVerifier
		revoker  *scriptedRevoker
		proof    auth.AppleProof
		want     error
		calls    int
	}{
		{"revocation failed", scriptedVerifier{claims: validAppleClaims("a-refused")}, &scriptedRevoker{err: fmt.Errorf("%w: token revocation: apple answered 400 invalid_client", auth.ErrAppleRevocationFailed)},
			auth.AppleProof{IDToken: "t", AuthorizationCode: "c"}, auth.ErrAppleRevocationFailed, 1},
		{"apple refused the code", scriptedVerifier{claims: validAppleClaims("a-refused")}, &scriptedRevoker{err: auth.ErrInvalidAppleAuthorizationCode},
			auth.AppleProof{IDToken: "t", AuthorizationCode: "c"}, auth.ErrInvalidAppleAuthorizationCode, 1},
		{"code of another apple account", scriptedVerifier{claims: validAppleClaims("a-refused")}, &scriptedRevoker{err: fmt.Errorf("%w: token exchange: the code was issued to another account", auth.ErrAppleAccountMismatch)},
			auth.AppleProof{IDToken: "t", AuthorizationCode: "c"}, auth.ErrAppleAccountMismatch, 1},
		{"token of another apple account", scriptedVerifier{claims: validAppleClaims("a-somebody-else")}, &scriptedRevoker{},
			auth.AppleProof{IDToken: "t", AuthorizationCode: "c"}, auth.ErrAppleAccountMismatch, 0},
		{"invalid token", scriptedVerifier{err: fmt.Errorf("%w: expired", auth.ErrInvalidIDToken)}, &scriptedRevoker{},
			auth.AppleProof{IDToken: "t", AuthorizationCode: "c"}, auth.ErrInvalidIDToken, 0},
		{"no proof", scriptedVerifier{claims: validAppleClaims("a-refused")}, &scriptedRevoker{},
			auth.AppleProof{}, auth.ErrAppleAuthorizationRequired, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			base := seedDeletionBase(t, pool, "del-trainer")
			client := appleClient(t, pool, base, "svc-refused", "a-refused")
			before := takeSnapshot(t, pool, client)
			svc := deletionService(pool, tt.verifier, tt.revoker)

			err := svc.DeleteAccount(context.Background(), client, tt.proof)

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if got := tt.revoker.callCount(); got != tt.calls {
				t.Errorf("revoker calls = %d, want %d", got, tt.calls)
			}
			before.assertUnchanged(t, pool, client)
		})
	}
}

func TestAccountDeletion_serviceDeletesAnAccountWithTwoAppleIdentitiesByTheOneProven(t *testing.T) {
	pool := NewPool(t)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := appleClient(t, pool, base, "svc-two", "a-one", "a-two")
	revoker := &scriptedRevoker{}
	svc := deletionService(pool, scriptedVerifier{claims: validAppleClaims("a-two")}, revoker)

	if err := svc.DeleteAccount(context.Background(), client, auth.AppleProof{IDToken: "t", AuthorizationCode: appleCode}); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if revoker.callCount() != 1 || userExists(t, pool, client) {
		t.Errorf("revoker calls = %d, user exists = %v, want 1 and false", revoker.callCount(), userExists(t, pool, client))
	}
}

func TestAccountDeletion_serviceRefusesATrainerWithAnAppleSignInBeforeAskingApple(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	trainer, err := store.RegisterTrainerEmailPassword(context.Background(), "svc-apple-trainer@test.com", "hash", "Coach")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	addIdentity(t, pool, trainer, auth.ProviderApple, "a-svc-trainer")
	before := takeSnapshot(t, pool, trainer)
	revoker := &scriptedRevoker{}
	svc := deletionService(pool, scriptedVerifier{claims: validAppleClaims("a-svc-trainer")}, revoker)

	err = svc.DeleteAccount(context.Background(), trainer, auth.AppleProof{IDToken: "t", AuthorizationCode: appleCode})

	if !errors.Is(err, auth.ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	if revoker.callCount() != 0 {
		t.Errorf("Apple was called %d times for an account that cannot be deleted", revoker.callCount())
	}
	before.assertUnchanged(t, pool, trainer)
}

// The service looked, found no Apple sign-in, and one arrived before the store took the
// lock. The store refuses and the service asks the person for an Apple authorization.
func TestAccountDeletion_serviceMapsALateAppleSignInToAuthorizationRequired(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	base := seedDeletionBase(t, pool, "del-trainer")
	client := newClientWithIdentity(t, store, auth.ProviderGoogle, "g-late-apple", "")
	seedClientData(t, pool, base, client, "late-apple")
	other := newClientWithIdentity(t, store, auth.ProviderApple, "a-late", "")
	before := takeSnapshot(t, pool, client)
	revoker := &scriptedRevoker{}
	svc := deletionService(pool, scriptedVerifier{claims: validAppleClaims("a-late")}, revoker)

	err := deleteBlockedBy(t, pool, func() error {
		return svc.DeleteAccount(context.Background(), client, auth.AppleProof{})
	}, `UPDATE mentorix.auth_identities SET user_id = $1 WHERE user_id = $2 AND provider = 'apple'`, client, other)

	if !errors.Is(err, auth.ErrAppleAuthorizationRequired) {
		t.Fatalf("error = %v, want ErrAppleAuthorizationRequired", err)
	}
	if revoker.callCount() != 0 {
		t.Errorf("revoker calls = %d, want 0", revoker.callCount())
	}
	before.references["auth_identities.user_id"]++ // the moved Apple identity committed
	if !userExists(t, pool, client) || !reflect.DeepEqual(userReferences(t, pool, client), before.references) {
		t.Errorf("the account changed: %v", userReferences(t, pool, client))
	}
}

// The whole chain over HTTP with the real handlers, service, store and database: the
// body of the request is what the verifier and the revoker see.
func TestAccountDeletion_handlersWithTheRealServiceAndDatabase(t *testing.T) {
	pool := NewPool(t)
	base := seedDeletionBase(t, pool, "del-trainer")
	revoker := &scriptedRevoker{}
	verifier := scriptedVerifier{claims: validAppleClaims("a-http")}
	svc := deletionService(pool, verifier, revoker)
	const secret = "integration-test-jwt-secret-32chars"
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	auth.NewHandlers(svc, secret, config.RefreshCookieSettings{Name: "mentorix_refresh", Path: "/auth"}, time.Hour, nil).Mount(e)
	client := appleClient(t, pool, base, "http", "a-http")
	token := mountedEnv{secret: secret}.accessToken(t, client)
	do := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/auth/me", strings.NewReader(body))
		if body != "" {
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	assertErrorBody(t, do(""), http.StatusUnprocessableEntity, "apple_authorization_required")
	assertErrorBody(t, do(`{"apple_id_token":"t"}`), http.StatusBadRequest, "bad_request")
	revoker.err = fmt.Errorf("%w: token exchange: apple answered 400 invalid_client", auth.ErrAppleRevocationFailed)
	assertErrorBody(t, do(`{"apple_id_token":"t","apple_authorization_code":"`+appleCode+`"}`), http.StatusBadGateway, "apple_revocation_failed")
	revoker.err = auth.ErrInvalidAppleAuthorizationCode
	assertErrorBody(t, do(`{"apple_id_token":"t","apple_authorization_code":"`+appleCode+`"}`), http.StatusBadRequest, "invalid_apple_authorization_code")
	if !userExists(t, pool, client) {
		t.Fatal("a refused request removed the user")
	}
	revoker.err = nil

	rec := do(`{"apple_id_token":"t","apple_authorization_code":"` + appleCode + `","apple_redirect_uri":"` + appleRedirectURI + `"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rec.Code, rec.Body.String())
	}
	if last := revoker.calls[len(revoker.calls)-1]; last != strings.Join([]string{appleAudience, "a-http", appleCode, appleRedirectURI}, "|") {
		t.Errorf("last revoker call = %q", last)
	}
	if userExists(t, pool, client) {
		t.Error("the user row is still there")
	}
	assertErrorBody(t, do(""), http.StatusNotFound, "user_not_found")
}

func appleSettings(t *testing.T) func(*config.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return func(c *config.Config) {
		c.AppleTeamID = "TSQ6DL62KG"
		c.AppleKeyID = "AB12CD34EF"
		c.ApplePrivateKey = key
	}
}

func appleClientIDs(c *config.Config) { c.AppleClientIDs = []string{appleAudience} }

// Through app.Mount: with the three settings and the client ids the revoker and the
// verifier are wired, and an Apple account is asked for an authorization instead of
// being refused for a missing configuration. The Apple endpoints are never reached:
// a request without a proof stops first, and a malformed token fails before any key fetch.
func TestAccountDeletion_mountedRouteWithTheAppleSettingsAsksForAnAuthorization(t *testing.T) {
	m := newMountedEnv(t, appleSettings(t), appleClientIDs)
	base := seedDeletionBase(t, m.pool, "del-trainer")
	client := appleClient(t, m.pool, base, "mount-settings", "a-mount-settings")
	before := takeSnapshot(t, m.pool, client)
	token := m.accessToken(t, client)

	assertErrorBody(t, m.do(http.MethodDelete, "/auth/me", token, ""), http.StatusUnprocessableEntity, "apple_authorization_required")
	assertErrorBody(t, m.do(http.MethodDelete, "/auth/me", token, `{"apple_id_token":"not-a-jwt","apple_authorization_code":"c"}`),
		http.StatusUnauthorized, "invalid_id_token")
	before.assertUnchanged(t, m.pool, client)
}

func TestAccountDeletion_mountedRouteStillAnswers503WithoutTheWholeAppleSetup(t *testing.T) {
	partial := func(c *config.Config) { appleSettings(t)(c); c.AppleKeyID = "" }
	tests := []struct {
		name    string
		options []func(*config.Config)
	}{
		{"no settings, no client ids", nil},
		{"client ids only", []func(*config.Config){appleClientIDs}},
		{"the key settings only", []func(*config.Config){appleSettings(t)}},
		{"one of the three settings missing", []func(*config.Config){appleClientIDs, partial}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMountedEnv(t, tt.options...)
			base := seedDeletionBase(t, m.pool, "del-trainer")
			client := appleClient(t, m.pool, base, "mount-503", "a-mount-503")
			before := takeSnapshot(t, m.pool, client)

			rec := m.do(http.MethodDelete, "/auth/me", m.accessToken(t, client),
				`{"apple_id_token":"not-a-jwt","apple_authorization_code":"c"}`)

			assertErrorBody(t, rec, http.StatusServiceUnavailable, "provider_not_configured")
			before.assertUnchanged(t, m.pool, client)
		})
	}
}
