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
	if got := NewRateLimiter(nil, 10, time.Minute); got != nil {
		t.Fatal("expected nil limiter without redis")
	}
}

func TestRateLimiter_disabled(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 0, time.Minute)
	if err := l.AllowLogin(context.Background(), "1.2.3.4"); err != nil {
		t.Errorf("AllowLogin() = %v", err)
	}
}

func TestRateLimiter_allowAndBlock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 2, time.Minute)
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
}

func TestRateLimiter_refreshLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute)
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
	l := NewRateLimiter(rdb, 5, time.Minute)
	if err := l.AllowLogin(context.Background(), ""); err != nil {
		t.Errorf("AllowLogin empty ip: %v", err)
	}
}

func TestRateLimiter_failsOpenWhenRedisDown(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute)
	mr.Close()
	ctx := context.Background()

	if err := l.AllowLogin(ctx, "10.0.0.4"); err != nil {
		t.Errorf("AllowLogin with redis down = %v, want nil", err)
	}
	if err := l.AllowRefresh(ctx, "10.0.0.4"); err != nil {
		t.Errorf("AllowRefresh with redis down = %v, want nil", err)
	}
}

func TestRateLimiter_setsTTLOnFirstHit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 5, time.Minute)

	if err := l.AllowLogin(context.Background(), "10.0.0.5"); err != nil {
		t.Fatalf("AllowLogin: %v", err)
	}
	if got := mr.TTL("mentorix:rl:login:ip:10.0.0.5"); got != time.Minute {
		t.Errorf("TTL = %v, want %v", got, time.Minute)
	}
}

func TestRateLimiter_healsKeyWithoutTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 5, time.Minute)
	key := "mentorix:rl:login:ip:10.0.0.6"
	if err := mr.Set(key, "3"); err != nil {
		t.Fatal(err)
	}

	if err := l.AllowLogin(context.Background(), "10.0.0.6"); err != nil {
		t.Fatalf("AllowLogin: %v", err)
	}
	if got := mr.TTL(key); got != time.Minute {
		t.Errorf("TTL of healed key = %v, want %v", got, time.Minute)
	}
}

func TestRateLimiter_doesNotExtendWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 5, time.Minute)
	ctx := context.Background()
	key := "mentorix:rl:login:ip:10.0.0.7"

	_ = l.AllowLogin(ctx, "10.0.0.7")
	mr.FastForward(40 * time.Second)
	_ = l.AllowLogin(ctx, "10.0.0.7")
	if got := mr.TTL(key); got != 20*time.Second {
		t.Errorf("TTL after second hit = %v, want 20s", got)
	}
}

func TestRateLimiter_windowExpiryResets(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute)
	ctx := context.Background()

	_ = l.AllowLogin(ctx, "10.0.0.8")
	if err := l.AllowLogin(ctx, "10.0.0.8"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second AllowLogin = %v, want ErrRateLimited", err)
	}
	mr.FastForward(time.Minute + time.Second)
	if err := l.AllowLogin(ctx, "10.0.0.8"); err != nil {
		t.Errorf("AllowLogin after window = %v, want nil", err)
	}
}

func retryAfterOf(t *testing.T, err error) time.Duration {
	t.Helper()
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("error %v is not a *RateLimitError", err)
	}
	return rl.RetryAfter
}

func TestRateLimiter_retryAfterIsTimeToWindowReset(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute)
	ctx := context.Background()

	_ = l.AllowLogin(ctx, "10.0.1.1")
	err := l.AllowLogin(ctx, "10.0.1.1")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second AllowLogin = %v, want ErrRateLimited", err)
	}
	if got := retryAfterOf(t, err); got <= 59*time.Second || got > time.Minute {
		t.Errorf("RetryAfter = %v, want within (59s, 1m]", got)
	}

	mr.FastForward(30 * time.Second)
	err = l.AllowLogin(ctx, "10.0.1.1")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("AllowLogin after 30s = %v, want ErrRateLimited", err)
	}
	if got := retryAfterOf(t, err); got <= 29*time.Second || got > 30*time.Second {
		t.Errorf("RetryAfter after 30s = %v, want within (29s, 30s]", got)
	}
}

func TestRateLimiter_retryAfterForEveryAction(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, 2*time.Minute)
	ctx := context.Background()
	tests := []struct {
		name   string
		allow  func(context.Context, string) error
		window time.Duration
	}{
		{"login", l.AllowLogin, 2 * time.Minute},
		{"refresh", l.AllowRefresh, 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.allow(ctx, "10.0.1.2")
			got := retryAfterOf(t, tt.allow(ctx, "10.0.1.2"))
			if got <= tt.window-time.Second || got > tt.window {
				t.Errorf("RetryAfter = %v, want within (%v, %v]", got, tt.window-time.Second, tt.window)
			}
		})
	}
}

func TestNewRateLimitError_retryAfter(t *testing.T) {
	tests := []struct {
		name string
		pttl int64
		want time.Duration
	}{
		{"remaining ttl", 1500, 1500 * time.Millisecond},
		{"no ttl falls back to the window", -1, time.Minute},
		{"missing key falls back to the window", -2, time.Minute},
		{"zero falls back to the window", 0, time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newRateLimitError(tt.pttl, time.Minute).RetryAfter; got != tt.want {
				t.Errorf("RetryAfter = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRateLimitError_isErrRateLimited(t *testing.T) {
	err := error(&RateLimitError{RetryAfter: time.Second})
	if !errors.Is(err, ErrRateLimited) {
		t.Error("errors.Is(RateLimitError, ErrRateLimited) = false")
	}
	if err.Error() != ErrRateLimited.Error() {
		t.Errorf("Error() = %q, want %q", err.Error(), ErrRateLimited.Error())
	}
}

func TestRateLimiter_failsOpenOnScriptError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := NewRateLimiter(rdb, 1, time.Minute)
	// INCR fails on a non-integer value, so the script returns an error.
	if err := mr.Set("mentorix:rl:login:ip:10.0.1.3", "not-a-number"); err != nil {
		t.Fatal(err)
	}

	if err := l.AllowLogin(context.Background(), "10.0.1.3"); err != nil {
		t.Errorf("AllowLogin on a script error = %v, want nil", err)
	}
}
