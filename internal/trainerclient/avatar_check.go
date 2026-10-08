package trainerclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const avatarCheckKeyPrefix = "mentorix:telegram:avatar_checked:"

// AvatarCheckTTL is how long a Telegram user is left alone after an avatar check.
const AvatarCheckTTL = 24 * time.Hour

// AvatarCheckStore limits avatar refreshes to one per user per AvatarCheckTTL.
type AvatarCheckStore interface {
	// Claim reports whether the caller may refresh the avatar now: true only
	// for the first call within the window.
	Claim(ctx context.Context, telegramUserID string) (bool, error)
}

type redisAvatarCheckStore struct {
	rdb *redis.Client
}

func (s *redisAvatarCheckStore) Claim(ctx context.Context, telegramUserID string) (bool, error) {
	ok, err := s.rdb.SetNX(ctx, avatarCheckKeyPrefix+telegramUserID, 1, AvatarCheckTTL).Result()
	if err != nil {
		return false, fmt.Errorf("redis claim avatar check: %w", err)
	}
	return ok, nil
}

type memoryAvatarCheckStore struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func (s *memoryAvatarCheckStore) Claim(_ context.Context, telegramUserID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if until, ok := s.until[telegramUserID]; ok && now.Before(until) {
		return false, nil
	}
	s.until[telegramUserID] = now.Add(AvatarCheckTTL)
	return true, nil
}

// NewAvatarCheckStore returns the Redis-backed store, or an in-memory one when rdb is nil.
func NewAvatarCheckStore(rdb *redis.Client) AvatarCheckStore {
	if rdb != nil {
		return &redisAvatarCheckStore{rdb: rdb}
	}
	return &memoryAvatarCheckStore{until: make(map[string]time.Time)}
}
