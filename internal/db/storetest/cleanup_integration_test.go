//go:build integration

package storetest

import (
	"context"
	"testing"

	"mentorix-backend/internal/cleanup"
)

func TestCleanup_Run_purgesStaleRows(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	result, err := cleanup.Run(ctx, pool)
	if err != nil {
		t.Fatalf("cleanup.Run: %v", err)
	}
	if result.TrainerInvites < 0 || result.RefreshSessions < 0 {
		t.Fatalf("unexpected negative counts: %+v", result)
	}
}
