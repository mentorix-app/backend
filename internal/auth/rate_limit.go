package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRateLimited = errors.New("too many requests")

// RateLimitError is returned when a limit is exceeded. RetryAfter is the time
// until the window resets. It matches ErrRateLimited under errors.Is.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string { return ErrRateLimited.Error() }

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// newRateLimitError falls back to the whole window when Redis reports no TTL.
func newRateLimitError(pttl int64, window time.Duration) *RateLimitError {
	if pttl <= 0 {
		return &RateLimitError{RetryAfter: window}
	}
	return &RateLimitError{RetryAfter: time.Duration(pttl) * time.Millisecond}
}

const (
	rateLimitKeyPrefix = "mentorix:rl:"
	rateLimitLogin     = "login:ip"
	rateLimitRefresh   = "refresh:ip"
	rateLimitUnknownIP = "unknown"
)

// incrWithTTL increments the counter and sets the window TTL on the first hit
// or when the key has none, in one atomic step. It returns the counter and the
// remaining TTL in milliseconds.
var incrWithTTL = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {n, redis.call('PTTL', KEYS[1])}
`)

type RateLimiter struct {
	rdb      *redis.Client
	log      *slog.Logger
	loginMax int
	loginWin time.Duration
}

func NewRateLimiter(rdb *redis.Client, loginMax int, loginWin time.Duration) *RateLimiter {
	if rdb == nil {
		return nil
	}
	return &RateLimiter{
		rdb:      rdb,
		log:      slog.Default(),
		loginMax: loginMax,
		loginWin: loginWin,
	}
}

// WithLogger sets the logger used for Redis failures; nil keeps the current one.
func (l *RateLimiter) WithLogger(log *slog.Logger) *RateLimiter {
	if l != nil && log != nil {
		l.log = log
	}
	return l
}

func (l *RateLimiter) AllowLogin(ctx context.Context, ip string) error {
	if l == nil || l.loginMax <= 0 {
		return nil
	}
	return l.allow(ctx, rateLimitLogin, ip, l.loginMax, l.loginWin)
}

func (l *RateLimiter) AllowRefresh(ctx context.Context, ip string) error {
	if l == nil || l.loginMax <= 0 {
		return nil
	}
	return l.allow(ctx, rateLimitRefresh, ip, l.loginMax, l.loginWin)
}

func (l *RateLimiter) allow(ctx context.Context, prefix, key string, max int, window time.Duration) error {
	if key == "" {
		key = rateLimitUnknownIP
	}
	rkey := rateLimitKeyPrefix + prefix + ":" + key
	reply, err := incrWithTTL.Run(ctx, l.rdb, []string{rkey}, window.Milliseconds()).Int64Slice()
	if err == nil && len(reply) != 2 {
		err = fmt.Errorf("unexpected script reply of %d values", len(reply))
	}
	if err != nil {
		// Fail open: a Redis outage must not take sign-in down.
		l.log.Warn("rate limiter unavailable, allowing request", "limit", prefix, "error", err)
		return nil
	}
	if reply[0] > int64(max) {
		return newRateLimitError(reply[1], window)
	}
	return nil
}
