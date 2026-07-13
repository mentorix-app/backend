package workoutcompletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	workoutPendingKeyPrefix = "mentorix:telegram:workout_pending:"
	WorkoutPendingTTL       = 30 * time.Minute
)

type Pending struct {
	ClientUserID        uuid.UUID `json:"client_user_id"`
	TrainerID           uuid.UUID `json:"trainer_id"`
	ProgramID           uuid.UUID `json:"program_id"`
	ProgramVersionID    uuid.UUID `json:"program_version_id"`
	ProgramAssignmentID uuid.UUID `json:"program_assignment_id"`
	CompletionCycleID   uuid.UUID `json:"completion_cycle_id"`
	DayKey              uuid.UUID `json:"day_key"`
	WeekNumber          int       `json:"week_number"`
	DayNumber           int       `json:"day_number"`
	ProgramName         string    `json:"program_name"`
	ProgramNameRu       string    `json:"program_name_ru"`
}

type PendingStore interface {
	Get(ctx context.Context, telegramUserID string) (*Pending, bool, error)
	Set(ctx context.Context, telegramUserID string, p Pending) error
	Delete(ctx context.Context, telegramUserID string) error
}

type RedisPendingStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisPendingStore(rdb *redis.Client) *RedisPendingStore {
	return &RedisPendingStore{rdb: rdb, ttl: WorkoutPendingTTL}
}

func (s *RedisPendingStore) Get(ctx context.Context, telegramUserID string) (*Pending, bool, error) {
	if s == nil || s.rdb == nil {
		return nil, false, nil
	}
	val, err := s.rdb.Get(ctx, workoutPendingKeyPrefix+telegramUserID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get workout pending: %w", err)
	}
	var p Pending
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		return nil, false, fmt.Errorf("parse workout pending: %w", err)
	}
	return &p, true, nil
}

func (s *RedisPendingStore) Set(ctx context.Context, telegramUserID string, p Pending) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal workout pending: %w", err)
	}
	if err := s.rdb.Set(ctx, workoutPendingKeyPrefix+telegramUserID, b, s.ttl).Err(); err != nil {
		return fmt.Errorf("redis set workout pending: %w", err)
	}
	return nil
}

func (s *RedisPendingStore) Delete(ctx context.Context, telegramUserID string) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	if err := s.rdb.Del(ctx, workoutPendingKeyPrefix+telegramUserID).Err(); err != nil {
		return fmt.Errorf("redis delete workout pending: %w", err)
	}
	return nil
}

type memoryPendingStore struct {
	mu    sync.Mutex
	items map[string]pendingEntry
}

type pendingEntry struct {
	p         Pending
	expiresAt time.Time
}

func NewPendingStore(rdb *redis.Client) PendingStore {
	if rdb != nil {
		return NewRedisPendingStore(rdb)
	}
	return NewMemoryPendingStore()
}

func NewMemoryPendingStore() PendingStore {
	return &memoryPendingStore{items: make(map[string]pendingEntry)}
}

func (s *memoryPendingStore) Get(_ context.Context, telegramUserID string) (*Pending, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[telegramUserID]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(e.expiresAt) {
		delete(s.items, telegramUserID)
		return nil, false, nil
	}
	p := e.p
	return &p, true, nil
}

func (s *memoryPendingStore) Set(_ context.Context, telegramUserID string, p Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[telegramUserID] = pendingEntry{p: p, expiresAt: time.Now().Add(WorkoutPendingTTL)}
	return nil
}

func (s *memoryPendingStore) Delete(_ context.Context, telegramUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, telegramUserID)
	return nil
}
