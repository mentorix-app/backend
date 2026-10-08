package trainerclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/trainerclient"
)

func TestRedisAvatarCheckStore_claim(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := trainerclient.NewAvatarCheckStore(rdb)
	ctx := context.Background()

	if ok, err := store.Claim(ctx, "7"); err != nil || !ok {
		t.Fatalf("first Claim = %v, %v", ok, err)
	}
	if ok, err := store.Claim(ctx, "7"); err != nil || ok {
		t.Fatalf("second Claim = %v, %v", ok, err)
	}
	if !mr.Exists("mentorix:telegram:avatar_checked:7") {
		t.Fatal("key missing")
	}
	if ttl := mr.TTL("mentorix:telegram:avatar_checked:7"); ttl != trainerclient.AvatarCheckTTL {
		t.Fatalf("ttl = %v", ttl)
	}
	mr.FastForward(trainerclient.AvatarCheckTTL + time.Second)
	if ok, err := store.Claim(ctx, "7"); err != nil || !ok {
		t.Fatalf("Claim after expiry = %v, %v", ok, err)
	}
}

func TestRedisAvatarCheckStore_redisDown(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := trainerclient.NewAvatarCheckStore(rdb)
	mr.Close()
	if ok, err := store.Claim(context.Background(), "7"); err == nil || ok {
		t.Fatalf("Claim = %v, %v; want error", ok, err)
	}
}

func TestMemoryAvatarCheckStore_claim(t *testing.T) {
	store := trainerclient.NewAvatarCheckStore(nil)
	ctx := context.Background()
	if ok, err := store.Claim(ctx, "1"); err != nil || !ok {
		t.Fatalf("first Claim = %v, %v", ok, err)
	}
	if ok, err := store.Claim(ctx, "1"); err != nil || ok {
		t.Fatalf("second Claim = %v, %v", ok, err)
	}
	if ok, err := store.Claim(ctx, "2"); err != nil || !ok {
		t.Fatalf("other user Claim = %v, %v", ok, err)
	}
}
