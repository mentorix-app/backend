package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRateLimited = errors.New("too many requests")

type RateLimiter struct {
	rdb         *redis.Client
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
		loginMax:    loginMax,
		loginWin:    loginWin,
		registerMax: registerMax,
		registerWin: registerWin,
	}
}

func (l *RateLimiter) AllowLogin(ctx context.Context, ip string) error {
	if l == nil || l.loginMax <= 0 {
		return nil
	}
	return l.allow(ctx, "login:ip", ip, l.loginMax, l.loginWin)
}

func (l *RateLimiter) AllowRegister(ctx context.Context, ip string) error {
	if l == nil || l.registerMax <= 0 {
		return nil
	}
	return l.allow(ctx, "register:ip", ip, l.registerMax, l.registerWin)
}

func (l *RateLimiter) allow(ctx context.Context, prefix, key string, max int, window time.Duration) error {
	if key == "" {
		key = "unknown"
	}
	rkey := "mentorix:rl:" + prefix + ":" + key
	n, err := l.rdb.Incr(ctx, rkey).Result()
	if err != nil {
		return fmt.Errorf("rate limit incr: %w", err)
	}
	if n == 1 {
		if err := l.rdb.Expire(ctx, rkey, window).Err(); err != nil {
			return fmt.Errorf("rate limit expire: %w", err)
		}
	}
	if int(n) > max {
		return ErrRateLimited
	}
	return nil
}
