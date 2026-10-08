package telegram

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClient_hasTimeout(t *testing.T) {
	if HTTPClient.Timeout != 10*time.Second {
		t.Fatalf("timeout = %v, want 10s", HTTPClient.Timeout)
	}
}

func TestNewBotAPI_makesNoNetworkCall(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	api := NewBotAPI("123:abc")
	if api.Token != "123:abc" {
		t.Fatalf("token = %q", api.Token)
	}
	if api.Client != HTTPClient {
		t.Fatal("bot must use the shared client with a timeout")
	}
	if hits.Load() != 0 {
		t.Fatalf("unexpected requests: %d", hits.Load())
	}
}

func TestNewBotAPI_callsTelegramEndpoint(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true}}`))
	}))
	defer srv.Close()

	api := NewBotAPI("123:abc")
	api.SetAPIEndpoint(srv.URL + "/bot%s/%s")
	if _, err := api.GetMe(); err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if path != "/bot123:abc/getMe" {
		t.Fatalf("path = %q", path)
	}
}
