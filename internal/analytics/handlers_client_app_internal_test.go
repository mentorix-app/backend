package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
)

const clientAppTestSecret = "test-jwt-secret-at-least-32-chars-long"

type fakeSelfReader struct {
	result ClientSelfAnalytics
	err    error
	calls  int
	client uuid.UUID
	coach  uuid.UUID
}

func (f *fakeSelfReader) ClientSelfAnalytics(_ context.Context, clientUserID, trainerID uuid.UUID) (ClientSelfAnalytics, error) {
	f.calls++
	f.client, f.coach = clientUserID, trainerID
	return f.result, f.err
}

func clientAppBearer(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(clientAppTestSecret))
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + s
}

func selfAnalyticsEcho(f *fakeSelfReader) *echo.Echo {
	h := NewHandlers(nil, nil, clientAppTestSecret)
	h.selfReader = f
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	h.Mount(e)
	return e
}

func getSelfAnalytics(e *echo.Echo, target, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if authz != "" {
		req.Header.Set(echo.HeaderAuthorization, authz)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestClientTrainerAnalytics_returnsTheSelfPageBody(t *testing.T) {
	userID, trainerID := uuid.New(), uuid.New()
	f := &fakeSelfReader{result: ClientSelfAnalytics{
		Client:            ClientSelfInfo{ClientUserID: userID, DisplayName: "Ira"},
		Trainer:           ClientSelfTrainer{TrainerID: trainerID, DisplayName: "Anna"},
		RecentCompletions: []CompletionItem{},
	}}

	rec := getSelfAnalytics(selfAnalyticsEcho(f), "/client/trainers/"+trainerID.String()+"/analytics", clientAppBearer(t, userID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.calls != 1 || f.client != userID || f.coach != trainerID {
		t.Fatalf("service called %d times for %v / %v, want once for %v / %v", f.calls, f.client, f.coach, userID, trainerID)
	}
	// Same keys as the signed-link page: the schema is shared.
	var keys []string
	for k := range decodeBody(t, rec) {
		keys = append(keys, k)
	}
	for _, want := range []string{"client", "trainer", "current_assignment", "activity", "recent_completions", "expires_at"} {
		found := false
		for _, k := range keys {
			found = found || k == want
		}
		if !found {
			t.Errorf("body lacks %q; keys %v", want, keys)
		}
	}
	if len(keys) != 6 {
		t.Errorf("body keys = %v, want exactly the six of the shared schema", keys)
	}
}

// The token route has no signed link to expire, so the key stays and is null. The page
// keeps answering the link's expiry.
func TestClientTrainerAnalytics_expiresAtIsNull(t *testing.T) {
	f := &fakeSelfReader{}

	rec := getSelfAnalytics(selfAnalyticsEcho(f), "/client/trainers/"+uuid.NewString()+"/analytics", clientAppBearer(t, uuid.New()))

	body := decodeBody(t, rec)
	if got, present := body["expires_at"]; !present || got != nil {
		t.Fatalf("expires_at = %v (present %v), want the key with null", got, present)
	}
}

func TestClientSelfPage_expiresAtIsTheLinkExpiry(t *testing.T) {
	clientID, trainerID := uuid.New(), uuid.New()
	f := &fakeSelfReader{}
	h := NewHandlers(nil, nil, clientAppTestSecret)
	h.selfReader = f
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	h.Mount(e)
	link := BuildClientAnalyticsLink("https://app.example.com/stats", clientAppTestSecret, clientID, trainerID, time.Now())
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	rec := getSelfAnalytics(e, "/client/analytics?"+u.RawQuery, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got, want := decodeBody(t, rec)["expires_at"], time.Unix(exp, 0).UTC().Format(time.RFC3339); got != want {
		t.Fatalf("expires_at = %v, want %v", got, want)
	}
	if f.client != clientID || f.coach != trainerID {
		t.Fatalf("service got %v / %v", f.client, f.coach)
	}
}

func TestClientTrainerAnalytics_requiresToken(t *testing.T) {
	f := &fakeSelfReader{}
	e := selfAnalyticsEcho(f)
	target := "/client/trainers/" + uuid.NewString() + "/analytics"
	for name, authz := range map[string]string{"missing": "", "malformed": "Bearer nope"} {
		if rec := getSelfAnalytics(e, target, authz); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
	if f.calls != 0 {
		t.Fatalf("service called %d times without a valid token", f.calls)
	}
}

func TestClientTrainerAnalytics_badTrainerID(t *testing.T) {
	f := &fakeSelfReader{}

	rec := getSelfAnalytics(selfAnalyticsEcho(f), "/client/trainers/nope/analytics", clientAppBearer(t, uuid.New()))

	if rec.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status = %d with %d service calls, want 400 and none; body %s", rec.Code, f.calls, rec.Body)
	}
}

func TestClientTrainerAnalytics_onlyThePathNamesTheTrainerAndTheTokenTheClient(t *testing.T) {
	userID, pathTrainer, other := uuid.New(), uuid.New(), uuid.New()
	f := &fakeSelfReader{}
	target := fmt.Sprintf("/client/trainers/%s/analytics?trainer_id=%s&client_user_id=%s", pathTrainer, other, other)

	rec := getSelfAnalytics(selfAnalyticsEcho(f), target, clientAppBearer(t, userID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.client != userID || f.coach != pathTrainer {
		t.Fatalf("service got client %v trainer %v, want %v and %v", f.client, f.coach, userID, pathTrainer)
	}
}

func TestClientTrainerAnalytics_notLinkedOrBlockedAnswersLikeThePage(t *testing.T) {
	f := &fakeSelfReader{err: ErrClientNotFound}

	rec := getSelfAnalytics(selfAnalyticsEcho(f), "/client/trainers/"+uuid.NewString()+"/analytics", clientAppBearer(t, uuid.New()))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body)
	}
	want := map[string]any{"code": "client_not_found", "message": "client not found"}
	if got := decodeBody(t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}

func TestClientTrainerAnalytics_unexpectedErrorHidesTheCauseFromTheBody(t *testing.T) {
	f := &fakeSelfReader{err: errors.New("dial tcp 10.0.0.5:5432: refused")}

	rec := getSelfAnalytics(selfAnalyticsEcho(f), "/client/trainers/"+uuid.NewString()+"/analytics", clientAppBearer(t, uuid.New()))

	want := map[string]any{"code": "internal_error", "message": "internal"}
	if got := decodeBody(t, rec); rec.Code != http.StatusInternalServerError || !reflect.DeepEqual(got, want) {
		t.Fatalf("status %d body %v, want 500 %v", rec.Code, got, want)
	}
}

// The log line of the request prints the internal error, so a 500 must keep its cause.
func TestClientTrainerAnalytics_unexpectedErrorKeepsTheCauseForTheLog(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.5:5432: refused")
	h := NewHandlers(nil, nil, clientAppTestSecret)
	h.selfReader = &fakeSelfReader{err: fmt.Errorf("read stats: %w", cause)}
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set(auth.ContextUserIDKey, uuid.New())
	c.SetParamNames("trainer_id")
	c.SetParamValues(uuid.NewString())

	err := h.GetClientTrainerAnalytics(c)

	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("err = %v, want a 500 HTTP error", err)
	}
	if !errors.Is(he.Internal, cause) {
		t.Fatalf("internal = %v, want the cause kept for the log", he.Internal)
	}
	if he.Message != httpx.MsgInternal {
		t.Fatalf("message = %v, want the fixed text", he.Message)
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON object: %v; body = %s", err, rec.Body)
	}
	return got
}
