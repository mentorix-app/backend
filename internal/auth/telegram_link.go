package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	linkCodeKeyPrefix = "mentorix:telegram:link_code:"
	// LinkCodeTTL is how long a code stays valid after the bot issues it.
	LinkCodeTTL = 10 * time.Minute

	linkCodeLen = 8
	// linkCodeAlphabet leaves out 0, O, 1, I and L, which are easy to misread.
	linkCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// linkCodeAttempts bounds the retries when a generated code is already taken.
	linkCodeAttempts = 3
)

// LinkCodeStore keeps one-time codes the bot hands to a Telegram user and the
// app later redeems. It is safe for concurrent use. A nil store, or one built
// without a Redis client, reports ErrLinkCodesUnavailable.
type LinkCodeStore struct {
	rdb *redis.Client
}

func NewLinkCodeStore(rdb *redis.Client) *LinkCodeStore {
	return &LinkCodeStore{rdb: rdb}
}

// Issue stores a new code for telegramUserID and returns it.
func (s *LinkCodeStore) Issue(ctx context.Context, telegramUserID string) (string, error) {
	if s == nil || s.rdb == nil {
		return "", ErrLinkCodesUnavailable
	}
	if telegramUserID == "" {
		return "", errors.New("link code needs a telegram user id")
	}
	for range linkCodeAttempts {
		code, err := randomLinkCode()
		if err != nil {
			return "", err
		}
		ok, err := s.rdb.SetNX(ctx, linkCodeKeyPrefix+code, telegramUserID, LinkCodeTTL).Result()
		if err != nil {
			return "", fmt.Errorf("%w: redis set link code: %w", ErrLinkCodesUnavailable, err)
		}
		if ok {
			return code, nil
		}
	}
	return "", errors.New("link code collided on every attempt")
}

// Take returns the Telegram user id behind code and deletes the code in the
// same step, so a code works once. ok is false for an unknown or expired code.
func (s *LinkCodeStore) Take(ctx context.Context, code string) (telegramUserID string, ok bool, err error) {
	if s == nil || s.rdb == nil {
		return "", false, ErrLinkCodesUnavailable
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != linkCodeLen {
		return "", false, nil
	}
	val, err := s.rdb.GetDel(ctx, linkCodeKeyPrefix+code).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%w: redis take link code: %w", ErrLinkCodesUnavailable, err)
	}
	if val == "" {
		return "", false, nil
	}
	return val, true, nil
}

func randomLinkCode() (string, error) {
	alphabetLen := big.NewInt(int64(len(linkCodeAlphabet)))
	buf := make([]byte, linkCodeLen)
	for i := range buf {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", fmt.Errorf("random link code: %w", err)
		}
		buf[i] = linkCodeAlphabet[n.Int64()]
	}
	return string(buf), nil
}
