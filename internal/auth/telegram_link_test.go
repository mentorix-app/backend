package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newLinkCodeTestStore(t *testing.T) (*LinkCodeStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewLinkCodeStore(rdb), mr
}

func TestLinkCodeStore_issueFormat(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)

	code, err := store.Issue(context.Background(), "42")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if len(code) != linkCodeLen {
		t.Fatalf("code = %q, want %d characters", code, linkCodeLen)
	}
	for _, r := range code {
		if !strings.ContainsRune(linkCodeAlphabet, r) {
			t.Fatalf("code %q has %q outside the alphabet", code, r)
		}
	}
	if got, err := mr.Get(linkCodeKeyPrefix + code); err != nil || got != "42" {
		t.Fatalf("stored value = %q, err = %v, want 42", got, err)
	}
	if ttl := mr.TTL(linkCodeKeyPrefix + code); ttl != LinkCodeTTL {
		t.Fatalf("ttl = %v, want %v", ttl, LinkCodeTTL)
	}
}

func TestLinkCodeAlphabet_hasNoLookAlikes(t *testing.T) {
	for _, r := range "01OIL" {
		if strings.ContainsRune(linkCodeAlphabet, r) {
			t.Errorf("alphabet contains look-alike %q", r)
		}
	}
}

func TestLinkCodeStore_takeIsOneShot(t *testing.T) {
	store, _ := newLinkCodeTestStore(t)
	ctx := context.Background()
	code, err := store.Issue(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Take(ctx, code)
	if err != nil || !ok || got != "42" {
		t.Fatalf("first Take() = %q, %v, %v, want 42, true, nil", got, ok, err)
	}
	if _, ok, err := store.Take(ctx, code); err != nil || ok {
		t.Fatalf("second Take() ok = %v, err = %v, want false, nil", ok, err)
	}
}

func TestLinkCodeStore_takeNormalisesInput(t *testing.T) {
	store, _ := newLinkCodeTestStore(t)
	ctx := context.Background()
	code, err := store.Issue(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Take(ctx, "  "+strings.ToLower(code)+" \n")
	if err != nil || !ok || got != "7" {
		t.Fatalf("Take() = %q, %v, %v, want 7, true, nil", got, ok, err)
	}
}

func TestLinkCodeStore_takeUnknownOrMalformed(t *testing.T) {
	store, _ := newLinkCodeTestStore(t)
	for _, in := range []string{"", "   ", "ABCDEFGH", "short", strings.Repeat("A", 100), "mentorix:x"} {
		if _, ok, err := store.Take(context.Background(), in); err != nil || ok {
			t.Errorf("Take(%q) ok = %v, err = %v, want false, nil", in, ok, err)
		}
	}
}

func TestLinkCodeStore_expires(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)
	ctx := context.Background()
	code, err := store.Issue(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	mr.FastForward(LinkCodeTTL + time.Second)
	if _, ok, err := store.Take(ctx, code); err != nil || ok {
		t.Fatalf("Take() after expiry ok = %v, err = %v, want false, nil", ok, err)
	}
}

func TestLinkCodeStore_issuedCodesDiffer(t *testing.T) {
	store, _ := newLinkCodeTestStore(t)
	seen := map[string]bool{}
	for range 50 {
		code, err := store.Issue(context.Background(), "42")
		if err != nil {
			t.Fatal(err)
		}
		if seen[code] {
			t.Fatalf("code %q issued twice", code)
		}
		seen[code] = true
	}
}

func TestLinkCodeStore_unavailable(t *testing.T) {
	ctx := context.Background()
	stores := map[string]*LinkCodeStore{
		"nil store":  nil,
		"nil client": NewLinkCodeStore(nil),
	}
	for name, store := range stores {
		if _, err := store.Issue(ctx, "42"); !errors.Is(err, ErrLinkCodesUnavailable) {
			t.Errorf("%s: Issue() error = %v, want ErrLinkCodesUnavailable", name, err)
		}
		if _, _, err := store.Take(ctx, "ABCDEFGH"); !errors.Is(err, ErrLinkCodesUnavailable) {
			t.Errorf("%s: Take() error = %v, want ErrLinkCodesUnavailable", name, err)
		}
	}
}

func TestLinkCodeStore_redisErrorsReportUnavailableAndKeepCause(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)
	mr.Close()
	_, err := store.Issue(context.Background(), "42")
	if !errors.Is(err, ErrLinkCodesUnavailable) || !strings.Contains(err.Error(), "redis") {
		t.Errorf("Issue() error = %v, want ErrLinkCodesUnavailable wrapping the redis cause", err)
	}
	_, _, err = store.Take(context.Background(), "ABCDEFGH")
	if !errors.Is(err, ErrLinkCodesUnavailable) || !strings.Contains(err.Error(), "redis") {
		t.Errorf("Take() error = %v, want ErrLinkCodesUnavailable wrapping the redis cause", err)
	}
}

func TestLinkCodeStore_issueRejectsEmptyTelegramUserID(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)
	if _, err := store.Issue(context.Background(), ""); err == nil {
		t.Fatal("Issue() with an empty id succeeded")
	}
	if keys := mr.Keys(); len(keys) != 0 {
		t.Errorf("stored keys = %v, want none", keys)
	}
}

func TestLinkCodeStore_takeTreatsEmptyValueAsNotFound(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)
	if err := mr.Set(linkCodeKeyPrefix+"ABCD2345", ""); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Take(context.Background(), "ABCD2345"); err != nil || ok || got != "" {
		t.Fatalf("Take() = %q, %v, %v, want empty, false, nil", got, ok, err)
	}
}
