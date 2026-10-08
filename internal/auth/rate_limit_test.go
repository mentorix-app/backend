package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewRateLimiter_nilRedis(t *testing.T) {
	if got := NewRateLimiter(nil, 10, time.Minute, 10, time.Minute); got != nil {
		t.Fatal("expected nil limiter without redis")
	}
}

func TestRateLimiter_disabled(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 0, time.Minute, 0, time.Minute)
	if err := l.AllowLogin(context.Background(), "1.2.3.4"); err != nil {
		t.Errorf("AllowLogin() = %v", err)
	}
}

func TestRateLimiter_allowAndBlock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 2, time.Minute, 2, time.Minute)
	ctx := context.Background()
	ip := "10.0.0.1"

	for i := 0; i < 2; i++ {
		if err := l.AllowLogin(ctx, ip); err != nil {
			t.Fatalf("AllowLogin attempt %d: %v", i+1, err)
		}
	}
	if err := l.AllowLogin(ctx, ip); !errors.Is(err, ErrRateLimited) {
		t.Errorf("third AllowLogin = %v, want ErrRateLimited", err)
	}

	regIP := "10.0.0.2"
	for i := 0; i < 2; i++ {
		if err := l.AllowRegister(ctx, regIP); err != nil {
			t.Fatalf("AllowRegister attempt %d: %v", i+1, err)
		}
	}
	if err := l.AllowRegister(ctx, regIP); !errors.Is(err, ErrRateLimited) {
		t.Errorf("third AllowRegister = %v, want ErrRateLimited", err)
	}
}

func TestRateLimiter_refreshLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	ctx := context.Background()
	ip := "10.0.0.3"

	if err := l.AllowRefresh(ctx, ip); err != nil {
		t.Fatalf("first AllowRefresh: %v", err)
	}
	if err := l.AllowRefresh(ctx, ip); !errors.Is(err, ErrRateLimited) {
		t.Errorf("second AllowRefresh = %v, want ErrRateLimited", err)
	}
}

func TestRateLimiter_unknownIP(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 5, time.Minute, 5, time.Minute)
	if err := l.AllowLogin(context.Background(), ""); err != nil {
		t.Errorf("AllowLogin empty ip: %v", err)
	}
}
