package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	testAppleSubject  = "000123.apple.subject"
	testAppleAudience = "com.mentorix.mobile"
	testProofIDToken  = "PROOF-ID-TOKEN-SECRET"
	testProofCode     = "PROOF-CODE-SECRET"
)

// recordingIDTokenVerifier records the tokens it was asked to verify.
type recordingIDTokenVerifier struct {
	claims IDTokenClaims
	err    error
	tokens []string
}

func (v *recordingIDTokenVerifier) Verify(_ context.Context, raw string) (IDTokenClaims, error) {
	v.tokens = append(v.tokens, raw)
	return v.claims, v.err
}

type revokeCall struct{ clientID, subject, code, redirectURI string }

// fakeAppleRevoker records its calls and how many deletes the store had seen by then.
type fakeAppleRevoker struct {
	store         *fakeAuthStore
	err           error
	calls         []revokeCall
	deletesBefore int
	// cancelRequest, when set, runs during the revocation: the client dropped the connection.
	cancelRequest func()
}

func (r *fakeAppleRevoker) Revoke(_ context.Context, clientID, subject, code, redirectURI string) error {
	r.calls = append(r.calls, revokeCall{clientID, subject, code, redirectURI})
	r.deletesBefore = len(r.store.deleteCalls)
	if r.cancelRequest != nil {
		r.cancelRequest()
	}
	return r.err
}

// appleDeletion is a client with one Apple sign-in and a valid proof for it.
type appleDeletion struct {
	userID   uuid.UUID
	store    *fakeAuthStore
	verifier *recordingIDTokenVerifier
	revoker  *fakeAppleRevoker
	svc      *Service
	proof    AppleProof
	cleanup  *cleanupRecorder
	logs     fmt.Stringer
}

func newAppleDeletion(t *testing.T) *appleDeletion {
	t.Helper()
	store := &fakeAuthStore{
		roles:             []string{RoleClient},
		appleSubjects:     []string{testAppleSubject},
		deletedTelegramID: "tg-apple-1",
	}
	verifier := &recordingIDTokenVerifier{claims: IDTokenClaims{Subject: testAppleSubject, Audience: testAppleAudience}}
	revoker := &fakeAppleRevoker{store: store}
	cleanup := &cleanupRecorder{}
	svc, logs := deletionService(store, cleanup)
	svc.appleVerifier = verifier
	svc.appleRevoker = revoker
	return &appleDeletion{
		userID:   uuid.New(),
		store:    store,
		verifier: verifier,
		revoker:  revoker,
		svc:      svc,
		proof:    AppleProof{IDToken: testProofIDToken, AuthorizationCode: testProofCode, RedirectURI: "https://api.mentorix.example/auth/apple/callback"},
		cleanup:  cleanup,
		logs:     logs,
	}
}

func (d *appleDeletion) run() error {
	return d.svc.DeleteAccount(context.Background(), d.userID, d.proof)
}

// assertUntouched fails when the deletion went any further than the given steps.
func (d *appleDeletion) assertUntouched(t *testing.T, verifies, revokes int) {
	t.Helper()
	if len(d.verifier.tokens) != verifies {
		t.Errorf("verifier calls = %d, want %d", len(d.verifier.tokens), verifies)
	}
	if len(d.revoker.calls) != revokes {
		t.Errorf("revoker calls = %d, want %d", len(d.revoker.calls), revokes)
	}
	if len(d.store.deleteCalls) != 0 {
		t.Errorf("store delete calls = %d, want 0", len(d.store.deleteCalls))
	}
	if len(d.cleanup.calls) != 0 {
		t.Errorf("cleanup calls = %v, want none", d.cleanup.calls)
	}
	if strings.Contains(d.logs.String(), "account deleted") {
		t.Errorf("logged a deletion that did not happen: %s", d.logs)
	}
}

func TestService_DeleteAccount_appleAccountIsRevokedAndThenDeleted(t *testing.T) {
	d := newAppleDeletion(t)

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if want := []string{testProofIDToken}; fmt.Sprint(d.verifier.tokens) != fmt.Sprint(want) {
		t.Errorf("verified tokens = %q, want %q", d.verifier.tokens, want)
	}
	if want := []revokeCall{{testAppleAudience, testAppleSubject, testProofCode, d.proof.RedirectURI}}; fmt.Sprint(d.revoker.calls) != fmt.Sprint(want) {
		t.Errorf("revoker calls = %+v, want %+v", d.revoker.calls, want)
	}
	if d.revoker.deletesBefore != 0 {
		t.Errorf("the store had deleted %d times when Apple was called, want the revocation first", d.revoker.deletesBefore)
	}
	if len(d.store.deleteCalls) != 1 || d.store.deleteCalls[0] != d.userID {
		t.Fatalf("store delete calls = %v, want exactly [%v]", d.store.deleteCalls, d.userID)
	}
	if len(d.cleanup.calls) != 1 || d.cleanup.calls[0] != "tg-apple-1" {
		t.Errorf("cleanup calls = %q, want [tg-apple-1]", d.cleanup.calls)
	}
}

func TestService_DeleteAccount_theRevokerGetsTheAudienceOfTheToken(t *testing.T) {
	for _, audience := range []string{"com.mentorix.mobile", "com.mentorix.service"} {
		t.Run(audience, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.verifier.claims.Audience = audience
			d.proof.RedirectURI = ""

			if err := d.run(); err != nil {
				t.Fatalf("DeleteAccount() error = %v", err)
			}

			if len(d.revoker.calls) != 1 || d.revoker.calls[0] != (revokeCall{audience, testAppleSubject, testProofCode, ""}) {
				t.Fatalf("revoker calls = %+v, want one for client %q with no redirect address", d.revoker.calls, audience)
			}
		})
	}
}

// The revoker is told which Apple account the code has to belong to: the one the
// verified token proved, not whatever the request or the account lists first.
func TestService_DeleteAccount_theRevokerGetsTheSubjectOfTheToken(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.appleSubjects = []string{"apple-sub-a", "apple-sub-b"}
	d.verifier.claims.Subject = "apple-sub-b"

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(d.revoker.calls) != 1 || d.revoker.calls[0].subject != "apple-sub-b" {
		t.Fatalf("revoker calls = %+v, want one for subject apple-sub-b", d.revoker.calls)
	}
}

func TestService_DeleteAccount_theStoreGetsTheSubjectOfTheToken(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.appleSubjects = []string{"apple-sub-a", "apple-sub-b"}
	d.verifier.claims.Subject = "apple-sub-b"

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(d.store.deleteSubjects) != 1 || d.store.deleteSubjects[0] != "apple-sub-b" {
		t.Fatalf("revoked subject passed to the store = %q, want [apple-sub-b]", d.store.deleteSubjects)
	}
}

// The role rule comes first and Apple is not called for an account that cannot be deleted.
func TestService_DeleteAccount_appleAccountWithOtherRolesIsRefusedBeforeApple(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{"admin", []string{RoleAdmin}},
		{"trainer and admin", []string{RoleTrainer, RoleAdmin}},
		{"client and admin", []string{RoleClient, RoleAdmin}},
		{"no role", nil},
		{"two client rows", []string{RoleClient, RoleClient}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.roles = tt.roles

			err := d.run()

			if !errors.Is(err, ErrAccountDeletionUnavailable) {
				t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
			}
			d.assertUntouched(t, 0, 0)
		})
	}
}

func TestService_DeleteAccount_theRoleRuleComesBeforeTheConfigurationCheck(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.roles = []string{RoleAdmin}
	d.svc.appleRevoker = nil

	err := d.run()

	if !errors.Is(err, ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	d.assertUntouched(t, 0, 0)
}

func TestService_DeleteAccount_appleAccountNeedsARevokerAndAVerifier(t *testing.T) {
	tests := []struct {
		name  string
		apply func(d *appleDeletion)
	}{
		{"no revoker", func(d *appleDeletion) { d.svc.appleRevoker = nil }},
		{"no verifier", func(d *appleDeletion) { d.svc.appleVerifier = nil }},
		{"neither", func(d *appleDeletion) { d.svc.appleRevoker, d.svc.appleVerifier = nil, nil }},
	}
	for _, tt := range tests {
		for _, withProof := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, proof %v", tt.name, withProof), func(t *testing.T) {
				d := newAppleDeletion(t)
				tt.apply(d)
				if !withProof {
					d.proof = AppleProof{}
				}

				err := d.run()

				if !errors.Is(err, ErrProviderNotConfigured) {
					t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
				}
				d.assertUntouched(t, 0, 0)
			})
		}
	}
}

func TestService_DeleteAccount_appleAccountNeedsTheWholeProof(t *testing.T) {
	tests := []struct {
		name  string
		proof AppleProof
	}{
		{"nothing", AppleProof{}},
		{"token only", AppleProof{IDToken: testProofIDToken}},
		{"code only", AppleProof{AuthorizationCode: testProofCode}},
		{"redirect address only", AppleProof{RedirectURI: "https://api.mentorix.example/auth/apple/callback"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.proof = tt.proof

			err := d.run()

			if !errors.Is(err, ErrAppleAuthorizationRequired) {
				t.Fatalf("error = %v, want ErrAppleAuthorizationRequired", err)
			}
			d.assertUntouched(t, 0, 0)
		})
	}
}

func TestService_DeleteAccount_aTokenThatFailsVerificationIsInvalid(t *testing.T) {
	d := newAppleDeletion(t)
	d.verifier.err = fmt.Errorf("%w: token is expired", ErrInvalidIDToken)

	err := d.run()

	if !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("error = %v, want ErrInvalidIDToken", err)
	}
	if !strings.Contains(err.Error(), "token is expired") {
		t.Errorf("error %q lost the cause", err)
	}
	d.assertUntouched(t, 1, 0)
}

func TestService_DeleteAccount_aTokenOfAnotherAppleAccountIsAMismatch(t *testing.T) {
	tests := []struct {
		name     string
		subjects []string
		token    string
	}{
		{"another account", []string{testAppleSubject}, "somebody-elses-subject"},
		{"none of two", []string{"apple-sub-a", "apple-sub-b"}, "apple-sub-c"},
		{"the prefix of a subject", []string{testAppleSubject}, testAppleSubject[:6]},
		{"the subject in other case", []string{"Apple-Sub"}, "apple-sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.appleSubjects = tt.subjects
			d.verifier.claims.Subject = tt.token

			err := d.run()

			if !errors.Is(err, ErrAppleAccountMismatch) {
				t.Fatalf("error = %v, want ErrAppleAccountMismatch", err)
			}
			d.assertUntouched(t, 1, 0)
		})
	}
}

func TestService_DeleteAccount_aFailedRevocationDeletesNothing(t *testing.T) {
	boom := errors.New("connection reset")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"apple refused the code", fmt.Errorf("%w: token exchange: apple answered 400 invalid_grant", ErrInvalidAppleAuthorizationCode), ErrInvalidAppleAuthorizationCode},
		{"apple refused the revocation", fmt.Errorf("%w: token exchange: apple answered 400 invalid_client", ErrAppleRevocationFailed), ErrAppleRevocationFailed},
		{"transport failure", fmt.Errorf("%w: token exchange: %w", ErrAppleRevocationFailed, boom), ErrAppleRevocationFailed},
		{"bare sentinel", ErrInvalidAppleAuthorizationCode, ErrInvalidAppleAuthorizationCode},
		{"the code belongs to another apple account", fmt.Errorf("%w: token exchange: the code was issued to another account", ErrAppleAccountMismatch), ErrAppleAccountMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.revoker.err = tt.err

			err := d.run()

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, ErrProviderNotConfigured) || errors.Is(err, ErrAppleAuthorizationRequired) {
				t.Errorf("error %v is mapped to another refusal", err)
			}
			if !strings.Contains(err.Error(), tt.err.Error()) {
				t.Errorf("error %q lost the cause %q", err, tt.err)
			}
			d.assertUntouched(t, 1, 1)
		})
	}
}

func TestService_DeleteAccount_failedReadsStopBeforeApple(t *testing.T) {
	boom := errors.New("db down")
	tests := []struct {
		name  string
		apply func(d *appleDeletion)
	}{
		{"apple sign-ins", func(d *appleDeletion) { d.store.appleSubjectsErr = boom }},
		{"roles", func(d *appleDeletion) { d.store.rolesErr = boom }},
		{"trainer data", func(d *appleDeletion) { d.store.businessDataErr = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			tt.apply(d)

			err := d.run()

			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want the database error", err)
			}
			d.assertUntouched(t, 0, 0)
		})
	}
}

// Apple confirmed the revocation and then the database failed: the account is still
// there, the error is the database's, and the person repeats the call after a new sign-in.
func TestService_DeleteAccount_databaseFailureAfterTheRevocationIsReturned(t *testing.T) {
	d := newAppleDeletion(t)
	boom := errors.New("db down")
	d.store.deleteErr = boom

	err := d.run()

	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the database error", err)
	}
	if len(d.revoker.calls) != 1 || len(d.store.deleteCalls) != 1 {
		t.Errorf("revoker calls = %d, store delete calls = %d, want 1 and 1", len(d.revoker.calls), len(d.store.deleteCalls))
	}
	if len(d.cleanup.calls) != 0 || strings.Contains(d.logs.String(), "account deleted") {
		t.Errorf("cleanup %v, log %s: nothing may follow a failed delete", d.cleanup.calls, d.logs)
	}
}

func TestService_DeleteAccount_accountWithoutAppleIgnoresTheProof(t *testing.T) {
	tests := []struct {
		name  string
		proof AppleProof
	}{
		{"no proof", AppleProof{}},
		{"a whole proof", AppleProof{IDToken: testProofIDToken, AuthorizationCode: testProofCode, RedirectURI: "https://api.mentorix.example/cb"}},
		{"a half proof", AppleProof{IDToken: testProofIDToken}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.appleSubjects = nil
			d.store.roles = nil // the store decides the role rule for these accounts
			d.proof = tt.proof

			if err := d.run(); err != nil {
				t.Fatalf("DeleteAccount() error = %v", err)
			}

			if len(d.verifier.tokens) != 0 || len(d.revoker.calls) != 0 {
				t.Errorf("verifier calls = %d, revoker calls = %d, want none", len(d.verifier.tokens), len(d.revoker.calls))
			}
			if len(d.store.deleteSubjects) != 1 || d.store.deleteSubjects[0] != "" {
				t.Errorf("revoked subject passed to the store = %q, want one empty value", d.store.deleteSubjects)
			}
		})
	}
}

func TestService_DeleteAccount_accountWithoutAppleNeedsNoAppleConfiguration(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.appleSubjects = nil
	d.svc.appleRevoker, d.svc.appleVerifier = nil, nil

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if len(d.store.deleteCalls) != 1 {
		t.Errorf("store delete calls = %d, want 1", len(d.store.deleteCalls))
	}
}

// The store refuses under its lock an account that has an Apple sign-in the revocation
// did not cover: one that arrived after the service looked.
func TestService_DeleteAccount_storeRefusalIsMappedByWhatIsConfigured(t *testing.T) {
	for _, storeErr := range []error{ErrAppleSignInNotRevoked, fmt.Errorf("lock: %w", ErrAppleSignInNotRevoked)} {
		t.Run("with a revoker, "+storeErr.Error(), func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.appleSubjects = nil // no Apple sign-in yet when the service looked
			d.store.deleteErr = storeErr

			err := d.run()

			if !errors.Is(err, ErrAppleAuthorizationRequired) {
				t.Fatalf("error = %v, want ErrAppleAuthorizationRequired", err)
			}
			if errors.Is(err, ErrProviderNotConfigured) {
				t.Errorf("error %v is also ErrProviderNotConfigured", err)
			}
			if len(d.cleanup.calls) != 0 || strings.Contains(d.logs.String(), "account deleted") {
				t.Errorf("cleanup %v, log %s: nothing may follow a refused delete", d.cleanup.calls, d.logs)
			}
		})
		t.Run("without a revoker, "+storeErr.Error(), func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.appleSubjects = nil
			d.store.deleteErr = storeErr
			d.svc.appleRevoker = nil

			err := d.run()

			if !errors.Is(err, ErrProviderNotConfigured) {
				t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
			}
			if errors.Is(err, ErrAppleAuthorizationRequired) {
				t.Errorf("error %v is also ErrAppleAuthorizationRequired", err)
			}
		})
	}
}

// A second Apple sign-in arrived after the service checked and after the first one was
// revoked: the person asks Apple again and repeats.
func TestService_DeleteAccount_aLateAppleSignInAfterTheRevocationAsksAgain(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.deleteErr = ErrAppleSignInNotRevoked

	err := d.run()

	if !errors.Is(err, ErrAppleAuthorizationRequired) {
		t.Fatalf("error = %v, want ErrAppleAuthorizationRequired", err)
	}
	if len(d.revoker.calls) != 1 {
		t.Errorf("revoker calls = %d, want 1", len(d.revoker.calls))
	}
}

func TestService_DeleteAccount_otherStoreErrorsOfAnAppleAccountAreReturned(t *testing.T) {
	for _, storeErr := range []error{ErrAccountDeletionUnavailable, errors.New("boom")} {
		d := newAppleDeletion(t)
		d.store.deleteErr = storeErr

		err := d.run()

		if !errors.Is(err, storeErr) {
			t.Errorf("store error %v: error = %v, want it returned", storeErr, err)
		}
		if errors.Is(err, ErrAppleAuthorizationRequired) || errors.Is(err, ErrProviderNotConfigured) {
			t.Errorf("store error %v: error %v is mapped to an Apple refusal", storeErr, err)
		}
	}
}

func TestService_DeleteAccount_nothingFromTheProofReachesTheLog(t *testing.T) {
	d := newAppleDeletion(t)

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	for _, secret := range []string{testProofIDToken, testProofCode, d.proof.RedirectURI, testAppleSubject} {
		if strings.Contains(d.logs.String(), secret) {
			t.Errorf("log carries %q: %s", secret, d.logs)
		}
	}
}

func TestService_DeleteAccount_unknownAccountFallsThroughToTheStore(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.appleSubjects = nil // an account that does not exist has no sign-ins
	d.store.deleteErr = pgx.ErrNoRows

	err := d.run()

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("error = %v, want the store's no-rows error", err)
	}
	if len(d.revoker.calls) != 0 || len(d.verifier.tokens) != 0 {
		t.Errorf("Apple was asked about an account that does not exist")
	}
}

// Apple confirmed the revocation, and the revocation cannot be undone: the delete must
// not depend on the client's connection any more, and it still has a deadline.
func TestService_DeleteAccount_aDeleteAfterARevocationOutlivesTheRequest(t *testing.T) {
	d := newAppleDeletion(t)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextProbeKey{}, "request"))
	defer cancel()
	d.revoker.cancelRequest = cancel

	if err := d.svc.DeleteAccount(ctx, d.userID, d.proof); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if ctx.Err() == nil {
		t.Fatal("the fixture did not cancel the request context")
	}
	if len(d.store.deleteContexts) != 1 {
		t.Fatalf("store delete calls = %d, want 1", len(d.store.deleteContexts))
	}
	got := d.store.deleteContexts[0]
	if got.err != nil {
		t.Errorf("the store got a context that was already done: %v", got.err)
	}
	if !got.hasDeadline {
		t.Fatal("the store got a context without a deadline")
	}
	if left := time.Until(got.deadline); left <= 0 || left > 10*time.Second {
		t.Errorf("the store's deadline is in %v, want within 10s", left)
	}
	if got.probe != "request" {
		t.Errorf("the store's context lost the request's values: %v", got.probe)
	}
}

// Without a revocation nothing irreversible has happened: the delete keeps the request's
// context, so a client that is gone cancels it as it did before.
func TestService_DeleteAccount_aDeleteWithoutARevocationKeepsTheRequestContext(t *testing.T) {
	t.Run("live request without a deadline", func(t *testing.T) {
		d := newAppleDeletion(t)
		d.store.appleSubjects = nil
		ctx := context.WithValue(context.Background(), contextProbeKey{}, "request")

		if err := d.svc.DeleteAccount(ctx, d.userID, AppleProof{}); err != nil {
			t.Fatalf("DeleteAccount() error = %v", err)
		}

		got := d.store.deleteContexts[0]
		if got.hasDeadline || got.err != nil || got.probe != "request" {
			t.Errorf("store context = %+v, want the request's own: no deadline, alive, same values", got)
		}
	})
	t.Run("cancelled request", func(t *testing.T) {
		d := newAppleDeletion(t)
		d.store.appleSubjects = nil
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := d.svc.DeleteAccount(ctx, d.userID, AppleProof{}); err != nil {
			t.Fatalf("DeleteAccount() error = %v", err)
		}

		if got := d.store.deleteContexts[0]; !errors.Is(got.err, context.Canceled) {
			t.Errorf("store context error = %v, want context.Canceled: the request's context", got.err)
		}
	})
}

// A code of another Apple account is refused by the revoker and reaches the client as
// 403, with nothing deleted.
func TestService_DeleteAccount_aMismatchFromTheRevokerReachesTheHandlerAs403(t *testing.T) {
	d := newAppleDeletion(t)
	d.revoker.err = fmt.Errorf("%w: token exchange: the code was issued to another account", ErrAppleAccountMismatch)
	e := codedEcho(testAuthHandlers(d.svc))

	rec := deleteMe(t, e, bearerToken(t, d.userID), deleteBody(testProofIDToken, testProofCode, ""))

	assertCodedError(t, rec, http.StatusForbidden, "apple_account_mismatch", "this apple account does not belong to the account")
	d.assertUntouched(t, 1, 1)
}

// A trainer without business data is deleted like a client: Apple first, then the store.
func TestService_DeleteAccount_appleTrainerWithoutDataIsRevokedAndDeleted(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{"trainer", []string{RoleTrainer}},
		{"client and trainer", []string{RoleClient, RoleTrainer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.roles = tt.roles

			if err := d.run(); err != nil {
				t.Fatalf("DeleteAccount() error = %v", err)
			}

			if len(d.revoker.calls) != 1 || len(d.store.deleteCalls) != 1 {
				t.Fatalf("revoker calls = %d, store delete calls = %d, want 1 and 1", len(d.revoker.calls), len(d.store.deleteCalls))
			}
			if d.revoker.deletesBefore != 0 {
				t.Errorf("the store had seen %d deletes when Apple was called, want 0", d.revoker.deletesBefore)
			}
			if d.store.businessDataCalls != 1 {
				t.Errorf("trainer data checks = %d, want 1", d.store.businessDataCalls)
			}
		})
	}
}

// Apple is never asked to revoke for a trainer the store would refuse for business data.
func TestService_DeleteAccount_appleTrainerWithDataIsRefusedBeforeApple(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{"trainer", []string{RoleTrainer}},
		{"client and trainer", []string{RoleClient, RoleTrainer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAppleDeletion(t)
			d.store.roles = tt.roles
			d.store.businessData = true

			err := d.run()

			if !errors.Is(err, ErrAccountDeletionUnavailable) {
				t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
			}
			d.assertUntouched(t, 0, 0)
		})
	}
}

// The trainer data rule comes before the configuration check, like the role rule, and an
// account the roles refuse is not looked up for data at all.
func TestService_DeleteAccount_theTrainerDataRuleComesBeforeTheConfigurationCheck(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.roles = []string{RoleTrainer}
	d.store.businessData = true
	d.svc.appleRevoker = nil

	err := d.run()

	if !errors.Is(err, ErrAccountDeletionUnavailable) {
		t.Fatalf("error = %v, want ErrAccountDeletionUnavailable", err)
	}
	d.assertUntouched(t, 0, 0)

	d = newAppleDeletion(t)
	d.store.roles = []string{RoleAdmin}
	if err := d.run(); !errors.Is(err, ErrAccountDeletionUnavailable) {
		t.Fatalf("admin: error = %v, want ErrAccountDeletionUnavailable", err)
	}
	if d.store.businessDataCalls != 0 {
		t.Errorf("an admin was looked up for trainer data %d times, want 0", d.store.businessDataCalls)
	}
}

// An account without an Apple sign-in never reaches the pre-check: the store decides.
func TestService_DeleteAccount_accountWithoutAppleSkipsThePreCheck(t *testing.T) {
	d := newAppleDeletion(t)
	d.store.appleSubjects = nil
	d.store.roles = []string{RoleTrainer}
	d.store.businessData = true

	if err := d.run(); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if d.store.businessDataCalls != 0 {
		t.Errorf("trainer data checks = %d, want 0", d.store.businessDataCalls)
	}
	if len(d.store.deleteCalls) != 1 {
		t.Errorf("store delete calls = %d, want 1", len(d.store.deleteCalls))
	}
}
