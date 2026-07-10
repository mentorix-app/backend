package trainerclient_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/trainerclient"
)

func TestMemoryActiveTrainerStore_setGet(t *testing.T) {
	store := trainerclient.NewMemoryActiveTrainerStore()
	id := uuid.New()
	if err := store.Set(context.Background(), "42", id); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(context.Background(), "42")
	if err != nil || !ok || got != id {
		t.Fatalf("Get = %v ok=%v err=%v", got, ok, err)
	}
}

func TestRedisActiveTrainerStore_setGet(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := trainerclient.NewRedisActiveTrainerStore(rdb)
	id := uuid.New()
	if err := store.Set(context.Background(), "99", id); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(context.Background(), "99")
	if err != nil || !ok || got != id {
		t.Fatalf("Get = %v ok=%v err=%v", got, ok, err)
	}
}

func TestRedisActiveTrainerStore_getMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := trainerclient.NewRedisActiveTrainerStore(rdb)
	_, ok, err := store.Get(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestRedisActiveTrainerStore_nilClient(t *testing.T) {
	var store *trainerclient.RedisActiveTrainerStore
	_, ok, err := store.Get(context.Background(), "1")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if err := store.Set(context.Background(), "1", uuid.New()); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

func TestNewActiveTrainerStore_prefersRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := trainerclient.NewActiveTrainerStore(rdb)
	id := uuid.New()
	if err := store.Set(context.Background(), "1", id); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(context.Background(), "1")
	if err != nil || !ok || got != id {
		t.Fatalf("Get = %v ok=%v err=%v", got, ok, err)
	}
}
