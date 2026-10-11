package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
)

const (
	testAndroidPackage = "com.mentorix.mobile"
	appleCallbackPath  = "/auth/apple/callback"
	intentPrefix       = "intent://callback?"
	intentSuffix       = "#Intent;package=com.mentorix.mobile;scheme=signinwithapple;end"
	// appleCallbackBodyLimit is the route-level limit in bytes: Echo reads "16K"
	// as decimal kilobytes.
	appleCallbackBodyLimit = 16000
)

// appleCallbackRouter mounts the auth routes on a real Echo router so the
// route-level middleware runs.
func appleCallbackRouter(pkg string, middlewares ...echo.MiddlewareFunc) *echo.Echo {
	h := testAuthHandlers(&fakeAuthService{})
	WithAppleAndroidPackage(pkg)(h)
	e := echo.New()
	e.Use(middlewares...)
	h.Mount(e)
	return e
}

func postAppleCallback(e *echo.Echo, target, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set(echo.HeaderContentType, contentType)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func appleCallbackErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return out.Message
}

func TestAppleCallback_redirectsToApp(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		wantQuery   string
	}{
		{
			name:        "successful authorization",
			contentType: echo.MIMEApplicationForm,
			body:        "code=c1&id_token=hdr.payload.sig&state=st-1&user=%7B%22name%22%3A%22A%22%7D",
			wantQuery:   "code=c1&id_token=hdr.payload.sig&state=st-1&user=%7B%22name%22%3A%22A%22%7D",
		},
		{
			name:        "fields come out sorted by name",
			contentType: echo.MIMEApplicationForm,
			body:        "user=u&state=s&id_token=t&code=c",
			wantQuery:   "code=c&id_token=t&state=s&user=u",
		},
		{
			name:        "cancelled sign-in carries error and state",
			contentType: echo.MIMEApplicationForm,
			body:        "error=user_cancelled_authorize&state=st-2",
			wantQuery:   "error=user_cancelled_authorize&state=st-2",
		},
		{
			name:        "charset parameter on the content type",
			contentType: "application/x-www-form-urlencoded; charset=UTF-8",
			body:        "code=c1&state=st-3",
			wantQuery:   "code=c1&state=st-3",
		},
		{
			name:        "content type is case-insensitive",
			contentType: "Application/X-WWW-Form-Urlencoded",
			body:        "state=st-4",
			wantQuery:   "state=st-4",
		},
		{
			name:        "unknown fields are passed through",
			contentType: echo.MIMEApplicationForm,
			body:        "state=st-5&extra=1",
			wantQuery:   "extra=1&state=st-5",
		},
		{
			name:        "repeated field keeps every value",
			contentType: echo.MIMEApplicationForm,
			body:        "state=a&state=b",
			wantQuery:   "state=a&state=b",
		},
		{
			name:        "field with an empty value is kept",
			contentType: echo.MIMEApplicationForm,
			body:        "state=&code=c",
			wantQuery:   "code=c&state=",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), appleCallbackPath, tc.contentType, tc.body)

			assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
			if got, want := rec.Header().Get(echo.HeaderLocation), intentPrefix+tc.wantQuery+intentSuffix; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestAppleCallback_escapesValues(t *testing.T) {
	user := `{"name":{"firstName":"Анна","lastName":"K;#"},"email":"a@b.c"}`
	form := url.Values{
		"code":     {"a#b;c&d=e f"},
		"id_token": {"x;package=evil.app;end#Intent"},
		"state":    {"сост ояние"},
		"user":     {user},
		"error":    {"a%20b+c"},
	}

	rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), appleCallbackPath, echo.MIMEApplicationForm, form.Encode())

	assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
	loc := rec.Header().Get(echo.HeaderLocation)
	if n := strings.Count(loc, "#"); n != 1 {
		t.Fatalf("Location has %d '#', want 1: %q", n, loc)
	}
	if !strings.HasPrefix(loc, intentPrefix) {
		t.Fatalf("Location = %q, want prefix %q", loc, intentPrefix)
	}
	if !strings.HasSuffix(loc, intentSuffix) {
		t.Fatalf("Location = %q, want suffix %q", loc, intentSuffix)
	}
	query := strings.TrimSuffix(strings.TrimPrefix(loc, intentPrefix), intentSuffix)
	for _, raw := range []string{"#", ";", " ", "Анна"} {
		if strings.Contains(query, raw) {
			t.Errorf("query part contains raw %q: %q", raw, query)
		}
	}
	for _, escaped := range []string{"%23", "%3B", "%26", "%3D"} {
		if !strings.Contains(query, escaped) {
			t.Errorf("query part lacks %q: %q", escaped, query)
		}
	}
	got, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse query part %q: %v", query, err)
	}
	if len(got) != len(form) {
		t.Errorf("got %d fields, want %d: %v", len(got), len(form), got)
	}
	for name, want := range form {
		if len(got[name]) != 1 || got[name][0] != want[0] {
			t.Errorf("field %q = %q, want %q", name, got[name], want)
		}
	}
}

// The bodies below are written by hand, as a forger would send them. A handler
// that forwarded the raw body would pass the round-trip test above, which only
// feeds bodies produced by url.Values.Encode.
func TestAppleCallback_forgedRawBody(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantQuery string
	}{
		{
			name:      "raw hash in a value",
			body:      "code=a#b&state=s",
			wantQuery: "code=a%23b&state=s",
		},
		{
			name:      "hostile key with a raw hash",
			body:      "x#Intent%3Bpackage%3Devil.app%3Bend=1&state=s",
			wantQuery: "state=s&x%23Intent%3Bpackage%3Devil.app%3Bend=1",
		},
		{
			name:      "hostile key in lowercase percent escapes",
			body:      "x%23Intent%3bpackage%3devil.app%3bend=1&state=s",
			wantQuery: "state=s&x%23Intent%3Bpackage%3Devil.app%3Bend=1",
		},
		{
			name:      "percent-encoded CR LF in a value",
			body:      "state=a%0D%0ASet-Cookie:+x",
			wantQuery: "state=a%0D%0ASet-Cookie%3A+x",
		},
		{
			name:      "literal CR LF in a value",
			body:      "state=a\r\nSet-Cookie:+x",
			wantQuery: "state=a%0D%0ASet-Cookie%3A+x",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), appleCallbackPath, echo.MIMEApplicationForm, tc.body)

			assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
			loc := rec.Header().Get(echo.HeaderLocation)
			if n := strings.Count(loc, "#"); n != 1 {
				t.Errorf("Location has %d '#', want 1: %q", n, loc)
			}
			if strings.ContainsAny(loc, "\r\n") {
				t.Errorf("Location contains a line break: %q", loc)
			}
			if !strings.HasSuffix(loc, intentSuffix) {
				t.Errorf("Location = %q, want suffix %q", loc, intentSuffix)
			}
			if want := intentPrefix + tc.wantQuery + intentSuffix; loc != want {
				t.Errorf("Location = %q, want %q", loc, want)
			}
		})
	}
}

func TestAppleCallback_ignoresQueryString(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    string
		body      string
		wantQuery string
	}{
		{
			name:      "field only in the URL query",
			target:    appleCallbackPath + "?injected=1&code=from-query",
			body:      "state=from-body",
			wantQuery: "state=from-body",
		},
		{
			name:      "same field in the URL query and in the body",
			target:    appleCallbackPath + "?state=from-query",
			body:      "state=from-body",
			wantQuery: "state=from-body",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), tc.target, echo.MIMEApplicationForm, tc.body)

			assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
			if got, want := rec.Header().Get(echo.HeaderLocation), intentPrefix+tc.wantQuery+intentSuffix; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
		})
	}
}

func TestAppleCallback_rejectsBadRequests(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      string
		contentType string
		body        string
		wantMessage string
	}{
		{
			name:        "empty form",
			target:      appleCallbackPath,
			contentType: echo.MIMEApplicationForm,
			body:        "",
			wantMessage: "form body is required",
		},
		{
			name:        "fields only in the URL query",
			target:      appleCallbackPath + "?state=s&code=c",
			contentType: echo.MIMEApplicationForm,
			body:        "",
			wantMessage: "form body is required",
		},
		{
			name:        "JSON body",
			target:      appleCallbackPath,
			contentType: echo.MIMEApplicationJSON,
			body:        `{"state":"s"}`,
			wantMessage: "form body is required",
		},
		{
			name:        "form fields under a JSON content type",
			target:      appleCallbackPath,
			contentType: echo.MIMEApplicationJSON,
			body:        "state=s",
			wantMessage: "form body is required",
		},
		{
			name:        "multipart body",
			target:      appleCallbackPath,
			contentType: "multipart/form-data; boundary=x",
			body:        "--x\r\nContent-Disposition: form-data; name=\"state\"\r\n\r\ns\r\n--x--\r\n",
			wantMessage: "form body is required",
		},
		{
			name:        "missing content type",
			target:      appleCallbackPath,
			contentType: "",
			body:        "state=s",
			wantMessage: "form body is required",
		},
		{
			name:        "unparsable content type",
			target:      appleCallbackPath,
			contentType: "application/x-www-form-urlencoded; charset",
			body:        "state=s",
			wantMessage: "invalid form body",
		},
		{
			name:        "broken percent escape in the body",
			target:      appleCallbackPath,
			contentType: echo.MIMEApplicationForm,
			body:        "state=%zz",
			wantMessage: "invalid form body",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), tc.target, tc.contentType, tc.body)

			assertHTTPStatus(t, rec, http.StatusBadRequest)
			if got := rec.Header().Get(echo.HeaderLocation); got != "" {
				t.Errorf("Location = %q, want none", got)
			}
			if got := appleCallbackErrorMessage(t, rec); got != tc.wantMessage {
				t.Errorf("message = %q, want %q", got, tc.wantMessage)
			}
			if strings.Contains(rec.Body.String(), "%zz") {
				t.Errorf("error body echoes the request: %s", rec.Body.String())
			}
		})
	}
}

func TestAppleCallback_notConfigured(t *testing.T) {
	rec := postAppleCallback(appleCallbackRouter(""), appleCallbackPath, echo.MIMEApplicationForm, "state=s")

	assertHTTPStatus(t, rec, http.StatusServiceUnavailable)
	if got := appleCallbackErrorMessage(t, rec); got != "apple sign-in return is not configured" {
		t.Errorf("message = %q", got)
	}
	if got := rec.Header().Get(echo.HeaderLocation); got != "" {
		t.Errorf("Location = %q, want none", got)
	}
}

func TestAppleCallback_notConfiguredWithJSONBody(t *testing.T) {
	rec := postAppleCallback(appleCallbackRouter(""), appleCallbackPath, echo.MIMEApplicationJSON, `{}`)

	assertHTTPStatus(t, rec, http.StatusServiceUnavailable)
}

// chunkedBody hides the length of the body so the request has no Content-Length.
type chunkedBody struct{ io.Reader }

func TestAppleCallback_bodyLimit(t *testing.T) {
	const prefix = "state="
	atLimit := prefix + strings.Repeat("a", appleCallbackBodyLimit-len(prefix))
	overLimit := atLimit + "a"

	t.Run("body of exactly the limit is accepted", func(t *testing.T) {
		rec := postAppleCallback(appleCallbackRouter(testAndroidPackage), appleCallbackPath, echo.MIMEApplicationForm, atLimit)

		assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
	})

	t.Run("over the limit with Content-Length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, appleCallbackPath, strings.NewReader(overLimit))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		if req.ContentLength != int64(len(overLimit)) {
			t.Fatalf("ContentLength = %d, want %d", req.ContentLength, len(overLimit))
		}
		rec := httptest.NewRecorder()
		appleCallbackRouter(testAndroidPackage).ServeHTTP(rec, req)

		assertHTTPStatus(t, rec, http.StatusRequestEntityTooLarge)
		if got := rec.Header().Get(echo.HeaderLocation); got != "" {
			t.Errorf("Location = %q, want none", got)
		}
	})

	t.Run("over the limit chunked without Content-Length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, appleCallbackPath, chunkedBody{strings.NewReader(overLimit)})
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.ContentLength = -1
		rec := httptest.NewRecorder()
		appleCallbackRouter(testAndroidPackage).ServeHTTP(rec, req)

		assertHTTPStatus(t, rec, http.StatusRequestEntityTooLarge)
		if got := rec.Header().Get(echo.HeaderLocation); got != "" {
			t.Errorf("Location = %q, want none", got)
		}
	})

	t.Run("chunked body within the limit is accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, appleCallbackPath, chunkedBody{strings.NewReader("state=s")})
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.ContentLength = -1
		rec := httptest.NewRecorder()
		appleCallbackRouter(testAndroidPackage).ServeHTTP(rec, req)

		assertHTTPStatus(t, rec, http.StatusTemporaryRedirect)
	})
}

func TestAppleCallback_getIsNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, appleCallbackPath+"?state=s", nil)
	rec := httptest.NewRecorder()
	appleCallbackRouter(testAndroidPackage).ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusMethodNotAllowed)
}

// The link carries the ID token and the authorization code, so no part of the
// form may reach the request log, on the success path or on an error path.
func TestAppleCallback_logsNothingFromTheForm(t *testing.T) {
	const prefix = "code=canary-big-code&state="
	oversized := prefix + strings.Repeat("a", appleCallbackBodyLimit)

	for _, tc := range []struct {
		name       string
		body       string
		chunked    bool
		wantStatus int
		// mustNotLog are values from the request, plus the fragment that
		// url.ParseQuery quotes in its error for a broken percent escape.
		mustNotLog []string
	}{
		{
			name:       "valid form",
			body:       "id_token=canary-id-token&code=canary-code&user=canary-user&state=canary-state",
			wantStatus: http.StatusTemporaryRedirect,
			mustNotLog: []string{"canary-id-token", "canary-code", "canary-user", "canary-state"},
		},
		{
			name:       "broken percent escape",
			body:       "id_token=canary-bad-token&state=%zqcanary-bad-state",
			wantStatus: http.StatusBadRequest,
			mustNotLog: []string{"canary-bad-token", "canary-bad-state", "%zq", "invalid URL escape"},
		},
		{
			name:       "oversized with Content-Length",
			body:       oversized,
			wantStatus: http.StatusRequestEntityTooLarge,
			mustNotLog: []string{"canary-big-code"},
		},
		{
			name:       "oversized chunked",
			body:       oversized,
			chunked:    true,
			wantStatus: http.StatusRequestEntityTooLarge,
			mustNotLog: []string{"canary-big-code"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			e := appleCallbackRouter(testAndroidPackage, httpx.RequestLog(slog.New(slog.NewJSONHandler(&logs, nil))))

			var body io.Reader = strings.NewReader(tc.body)
			if tc.chunked {
				body = chunkedBody{body}
			}
			req := httptest.NewRequest(http.MethodPost, appleCallbackPath, body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			if tc.chunked {
				req.ContentLength = -1
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assertHTTPStatus(t, rec, tc.wantStatus)
			// The request must have been logged at all, or the checks below prove nothing.
			if want := `"status":` + strconv.Itoa(tc.wantStatus); !strings.Contains(logs.String(), want) {
				t.Fatalf("log lacks %s: %q", want, logs.String())
			}
			for _, canary := range tc.mustNotLog {
				if strings.Contains(logs.String(), canary) {
					t.Errorf("log contains %q: %s", canary, logs.String())
				}
			}
		})
	}
}
