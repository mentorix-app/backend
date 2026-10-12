package auth

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	httpx "mentorix-backend/internal/http"
)

// The whole Apple step end to end: a real appleRevokeClient talks to a fake Apple, behind
// the real Service method, DeleteMe, httpx.ErrorHandler and the request logger. Whatever
// Apple says, the answer and every log line carry none of the secrets of the exchange
// and none of Apple's free text; the log still names the step and Apple's error value.
func TestDeleteMe_appleFailuresStayOutOfTheAnswerAndTheLogs(t *testing.T) {
	const extra = "EXTRA-FREE-TEXT-FROM-APPLE"
	describe := `"error_description":"` + extra + ` for code ` + testProofCode + `"`
	otherAccount := func(t *testing.T, a *fakeApple) {
		a.setIDToken(appleIDToken(t, answerClaims(testOtherAppleSubject, testAppleClientID)))
	}
	tests := []struct {
		name        string
		fail        func(t *testing.T, a *fakeApple)
		wantStatus  int
		wantCode    string
		wantMessage string
		wantLog     []string
		wantRevokes int
	}{
		{"the exchange answers invalid_client", func(_ *testing.T, a *fakeApple) { a.token = answer(400, `{"error":"invalid_client",`+describe+`}`) },
			502, "apple_revocation_failed", "apple did not confirm the revocation", []string{"token exchange", "invalid_client"}, 0},
		{"the exchange answers invalid_grant", func(_ *testing.T, a *fakeApple) { a.token = answer(400, `{"error":"invalid_grant",`+describe+`}`) },
			400, "invalid_apple_authorization_code", "invalid or expired apple authorization code", []string{"token exchange", "invalid_grant"}, 0},
		{"the exchange answers with a page", func(_ *testing.T, a *fakeApple) { a.token = answer(500, "<html>"+extra+" "+testProofCode+"</html>") },
			502, "apple_revocation_failed", "apple did not confirm the revocation", []string{"token exchange", "500"}, 0},
		{"the revocation answers invalid_request", func(_ *testing.T, a *fakeApple) { a.revoke = answer(400, `{"error":"invalid_request",`+describe+`}`) },
			502, "apple_revocation_failed", "apple did not confirm the revocation", []string{"token revocation", "invalid_request"}, 1},
		{"the code belongs to another account", otherAccount,
			403, "apple_account_mismatch", "this apple account does not belong to the account", []string{"token exchange", "the code was issued to another apple account"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apple := newFakeApple(t)
			tt.fail(t, apple)
			d := newAppleDeletion(t)
			client, _ := newTestRevoker(t, apple)
			d.svc.appleRevoker = client
			var requestLog bytes.Buffer
			e := codedEcho(testAuthHandlers(d.svc), httpx.RequestLog(slog.New(slog.NewTextHandler(&requestLog, nil))))
			body := deleteBody(testProofIDToken, testProofCode, d.proof.RedirectURI)

			rec := deleteMe(t, e, bearerToken(t, d.userID), body)

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
			if requestLog.Len() == 0 {
				t.Fatal("nothing was logged, the check proves nothing")
			}
			for _, want := range tt.wantLog {
				if !strings.Contains(requestLog.String(), want) {
					t.Errorf("log %q does not name %q", requestLog.String(), want)
				}
			}
			secrets := append(apple.clientSecrets(), testProofCode, testProofIDToken, extra, testAppleRefreshToken, testAppleAccessToken, apple.currentIDToken(),
				testAppleSubject, testOtherAppleSubject)
			for _, secret := range secrets {
				for place, text := range map[string]string{"response body": rec.Body.String(), "request log": requestLog.String(), "service log": d.logs.String()} {
					if strings.Contains(text, secret) {
						t.Errorf("%s contains %q...: %s", place, secret[:min(len(secret), 12)], text)
					}
				}
			}
			if got := len(apple.callsTo("revoke")); got != tt.wantRevokes {
				t.Errorf("revocation calls = %d, want %d", got, tt.wantRevokes)
			}
			if len(d.store.deleteCalls) != 0 {
				t.Errorf("store delete calls = %d, want 0 after an Apple failure", len(d.store.deleteCalls))
			}
		})
	}
}

// The control for the cases above: with an Apple that confirms, the same stack deletes.
func TestDeleteMe_appleConfirmsAndTheAccountIsDeleted(t *testing.T) {
	apple := newFakeApple(t)
	d := newAppleDeletion(t)
	client, _ := newTestRevoker(t, apple)
	d.svc.appleRevoker = client
	e := codedEcho(testAuthHandlers(d.svc))

	rec := deleteMe(t, e, bearerToken(t, d.userID), deleteBody(testProofIDToken, testProofCode, d.proof.RedirectURI))

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(d.store.deleteCalls) != 1 || d.store.deleteCalls[0] != d.userID || d.store.deleteSubjects[0] != testAppleSubject {
		t.Fatalf("store delete calls = %v with subjects %q, want one for %v and %s", d.store.deleteCalls, d.store.deleteSubjects, d.userID, testAppleSubject)
	}
	revokes := apple.callsTo("revoke")
	if len(revokes) != 1 || revokes[0].form.Get("token") != testAppleRefreshToken {
		t.Errorf("revocation calls = %+v, want one that revokes the refresh token of the exchange", revokes)
	}
}

// The other mismatch: the token proves an Apple account that is not a sign-in of the
// account. The answer is the same 403, and the log tells it from a code that was issued
// to another Apple account, in fixed words that name no subject.
func TestDeleteMe_aTokenOfAnotherAppleAccountIsToldApartInTheLog(t *testing.T) {
	d := newAppleDeletion(t)
	d.verifier.claims.Subject = testOtherAppleSubject
	var requestLog bytes.Buffer
	e := codedEcho(testAuthHandlers(d.svc), httpx.RequestLog(slog.New(slog.NewTextHandler(&requestLog, nil))))

	rec := deleteMe(t, e, bearerToken(t, d.userID), deleteBody(testProofIDToken, testProofCode, d.proof.RedirectURI))

	assertCodedError(t, rec, http.StatusForbidden, "apple_account_mismatch", "this apple account does not belong to the account")
	if want := "the id_token belongs to an apple account that is not a sign-in of this account"; !strings.Contains(requestLog.String(), want) {
		t.Errorf("log %q does not say %q", requestLog.String(), want)
	}
	if other := "the code was issued to another apple account"; strings.Contains(requestLog.String(), other) {
		t.Errorf("log %q gives the reason of the other mismatch", requestLog.String())
	}
	for _, secret := range []string{testAppleSubject, testOtherAppleSubject, testProofCode, testProofIDToken} {
		for place, text := range map[string]string{"response body": rec.Body.String(), "request log": requestLog.String(), "service log": d.logs.String()} {
			if strings.Contains(text, secret) {
				t.Errorf("%s contains %q: %s", place, secret, text)
			}
		}
	}
	d.assertUntouched(t, 1, 0)
}
