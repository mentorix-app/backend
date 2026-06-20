package auth

import (
	"bytes"
	"testing"
)

func TestHashRefreshToken_stable(t *testing.T) {
	plain := "test-refresh-token-value"
	h1 := hashRefreshToken(plain)
	h2 := hashRefreshToken(plain)
	if !bytes.Equal(h1, h2) {
		t.Fatal("hashRefreshToken should be deterministic")
	}
	if len(h1) != 32 {
		t.Errorf("expected SHA-256 length 32, got %d", len(h1))
	}
}

func TestNewRefreshToken_format(t *testing.T) {
	plain, hash, err := newRefreshToken()
	if err != nil {
		t.Fatalf("newRefreshToken: %v", err)
	}
	if plain == "" {
		t.Fatal("expected non-empty plain token")
	}
	if len(hash) != 32 {
		t.Errorf("expected hash length 32, got %d", len(hash))
	}
	if !bytes.Equal(hash, hashRefreshToken(plain)) {
		t.Error("hash should match hashRefreshToken(plain)")
	}

	plain2, _, err := newRefreshToken()
	if err != nil {
		t.Fatalf("second newRefreshToken: %v", err)
	}
	if plain == plain2 {
		t.Error("expected unique refresh tokens")
	}
}
