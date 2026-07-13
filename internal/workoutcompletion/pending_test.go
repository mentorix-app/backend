package workoutcompletion

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestMemoryPendingStore_roundTripAndExpiry(t *testing.T) {
	store := NewMemoryPendingStore()
	ctx := context.Background()
	tgID := "42"
	pend := Pending{
		ClientUserID: uuid.New(),
		TrainerID:    uuid.New(),
		DayKey:       uuid.New(),
		WeekNumber:   1,
		DayNumber:    2,
		ProgramName:  "Force",
	}

	got, ok, err := store.Get(ctx, tgID)
	if err != nil || ok || got != nil {
		t.Fatalf("empty get: got=%v ok=%v err=%v", got, ok, err)
	}

	if err := store.Set(ctx, tgID, pend); err != nil {
		t.Fatal(err)
	}
	got, ok, err = store.Get(ctx, tgID)
	if err != nil || !ok || got == nil {
		t.Fatalf("after set: got=%v ok=%v err=%v", got, ok, err)
	}
	if got.WeekNumber != 1 || got.DayNumber != 2 || got.ProgramName != "Force" {
		t.Fatalf("pending = %+v", got)
	}

	if err := store.Delete(ctx, tgID); err != nil {
		t.Fatal(err)
	}
	got, ok, err = store.Get(ctx, tgID)
	if err != nil || ok || got != nil {
		t.Fatalf("after delete: got=%v ok=%v err=%v", got, ok, err)
	}
}

func TestMemoryPendingStore_expires(t *testing.T) {
	store := &memoryPendingStore{items: make(map[string]pendingEntry)}
	tgID := "99"
	store.items[tgID] = pendingEntry{
		p:         Pending{WeekNumber: 3},
		expiresAt: time.Now().Add(-time.Second),
	}
	got, ok, err := store.Get(context.Background(), tgID)
	if err != nil || ok || got != nil {
		t.Fatalf("expired: got=%v ok=%v err=%v", got, ok, err)
	}
	if _, still := store.items[tgID]; still {
		t.Fatal("expired entry should be removed")
	}
}

func TestNewPendingStore_nilUsesMemory(t *testing.T) {
	store := NewPendingStore(nil)
	if _, ok := store.(*memoryPendingStore); !ok {
		t.Fatalf("got %T", store)
	}
}

func TestRedisPendingStore_roundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := NewPendingStore(rdb)
	ctx := context.Background()
	tgID := "tg-1"
	pend := Pending{
		ClientUserID:      uuid.New(),
		CompletionCycleID: uuid.New(),
		DayKey:            uuid.New(),
		WeekNumber:        2,
		DayNumber:         5,
		ProgramNameRu:     "Сила",
	}
	if err := store.Set(ctx, tgID, pend); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, tgID)
	if err != nil || !ok || got == nil {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.ProgramNameRu != "Сила" || got.WeekNumber != 2 {
		t.Fatalf("got %+v", got)
	}
	if err := store.Delete(ctx, tgID); err != nil {
		t.Fatal(err)
	}
	_, ok, err = store.Get(ctx, tgID)
	if err != nil || ok {
		t.Fatalf("after delete ok=%v err=%v", ok, err)
	}
}

func TestRedisPendingStore_nilReceiver(t *testing.T) {
	var store *RedisPendingStore
	ctx := context.Background()
	if _, ok, err := store.Get(ctx, "x"); err != nil || ok {
		t.Fatalf("nil get: ok=%v err=%v", ok, err)
	}
	if err := store.Set(ctx, "x", Pending{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestRedisPendingStore_corruptJSON(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewRedisPendingStore(rdb)
	key := workoutPendingKeyPrefix + "bad"
	if err := rdb.Set(context.Background(), key, "{not-json", 0).Err(); err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.Get(context.Background(), "bad")
	if err == nil || ok {
		t.Fatalf("want parse error, ok=%v err=%v", ok, err)
	}
}
