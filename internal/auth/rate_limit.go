package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRateLimited = errors.New("too many requests")

const (
	rateLimitKeyPrefix = "mentorix:rl:"
	rateLimitLogin     = "login:ip"
	rateLimitRegister  = "register:ip"
	rateLimitRefresh   = "refresh:ip"
	rateLimitUnknownIP = "unknown"
)

// incrWithTTL increments the counter and sets the window TTL on the first hit
// or when the key has none, in one atomic step.
var incrWithTTL = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

type RateLimiter struct {
	rdb         *redis.Client
	log         *slog.Logger
	loginMax    int
	loginWin    time.Duration
	registerMax int
	registerWin time.Duration
}

func NewRateLimiter(rdb *redis.Client, loginMax int, loginWin time.Duration, registerMax int, registerWin time.Duration) *RateLimiter {
	if rdb == nil {
		return nil
	}
	return &RateLimiter{
		rdb:         rdb,
		log:         slog.Default(),
		loginMax:    loginMax,
		loginWin:    loginWin,
		registerMax: registerMax,
		registerWin: registerWin,
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

func (l *RateLimiter) AllowRegister(ctx context.Context, ip string) error {
	if l == nil || l.registerMax <= 0 {
		return nil
	}
	return l.allow(ctx, rateLimitRegister, ip, l.registerMax, l.registerWin)
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
	n, err := incrWithTTL.Run(ctx, l.rdb, []string{rkey}, window.Milliseconds()).Int()
	if err != nil {
		// Fail open: a Redis outage must not take sign-in down.
		l.log.Warn("rate limiter unavailable, allowing request", "limit", prefix, "error", err)
		return nil
	}
	if n > max {
		return ErrRateLimited
	}
	return nil
}
