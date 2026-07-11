package trainerclient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const activeTrainerKeyPrefix = "mentorix:telegram:active_trainer:"

var ErrTelegramUserNotFound = errors.New("telegram user not found")
var ErrActiveTrainerNotSet = errors.New("active trainer not set")
var ErrTrainerNotLinked = errors.New("trainer is not linked to client")

type ActiveTrainerStore interface {
	Get(ctx context.Context, telegramUserID string) (uuid.UUID, bool, error)
	Set(ctx context.Context, telegramUserID string, trainerID uuid.UUID) error
	Delete(ctx context.Context, telegramUserID string) error
}

type RedisActiveTrainerStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisActiveTrainerStore(rdb *redis.Client) *RedisActiveTrainerStore {
	return &RedisActiveTrainerStore{rdb: rdb, ttl: 0}
}

func (s *RedisActiveTrainerStore) Get(ctx context.Context, telegramUserID string) (uuid.UUID, bool, error) {
	if s == nil || s.rdb == nil {
		return uuid.Nil, false, nil
	}
	val, err := s.rdb.Get(ctx, activeTrainerKeyPrefix+telegramUserID).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("redis get active trainer: %w", err)
	}
	id, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("parse active trainer id: %w", err)
	}
	return id, true, nil
}

func (s *RedisActiveTrainerStore) Set(ctx context.Context, telegramUserID string, trainerID uuid.UUID) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	if err := s.rdb.Set(ctx, activeTrainerKeyPrefix+telegramUserID, trainerID.String(), s.ttl).Err(); err != nil {
		return fmt.Errorf("redis set active trainer: %w", err)
	}
	return nil
}

func (s *RedisActiveTrainerStore) Delete(ctx context.Context, telegramUserID string) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	if err := s.rdb.Del(ctx, activeTrainerKeyPrefix+telegramUserID).Err(); err != nil {
		return fmt.Errorf("redis delete active trainer: %w", err)
	}
	return nil
}

type memoryActiveTrainerStore struct {
	mu    sync.RWMutex
	items map[string]uuid.UUID
}

func NewActiveTrainerStore(rdb *redis.Client) ActiveTrainerStore {
	if rdb != nil {
		return NewRedisActiveTrainerStore(rdb)
	}
	return NewMemoryActiveTrainerStore()
}

func NewMemoryActiveTrainerStore() ActiveTrainerStore {
	return &memoryActiveTrainerStore{items: make(map[string]uuid.UUID)}
}

func (s *memoryActiveTrainerStore) Get(_ context.Context, telegramUserID string) (uuid.UUID, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.items[telegramUserID]
	return id, ok, nil
}

func (s *memoryActiveTrainerStore) Set(_ context.Context, telegramUserID string, trainerID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[telegramUserID] = trainerID
	return nil
}

func (s *memoryActiveTrainerStore) Delete(_ context.Context, telegramUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, telegramUserID)
	return nil
}
