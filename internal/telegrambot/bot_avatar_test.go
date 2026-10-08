package telegrambot

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/trainerclient"
)

func avatarTestBot(t *testing.T) (*Bot, *fakeTrainerClient, *fakeTelegramAPI, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	clients := &fakeTrainerClient{}
	api := &fakeTelegramAPI{}
	bot := New(api, clients, WithAvatarCheckStore(trainerclient.NewAvatarCheckStore(rdb)))
	return bot, clients, api, mr
}

func textFrom(userID int64) *tgbotapi.Message {
	return &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: userID}, From: &tgbotapi.User{ID: userID}, Text: btnHelp}
}

func TestBot_avatarRefreshThrottledToOncePerDay(t *testing.T) {
	bot, clients, _, mr := avatarTestBot(t)
	ctx := context.Background()

	bot.handleMessage(ctx, textFrom(42))
	bot.handleMessage(ctx, textFrom(42))
	if want := []string{"42"}; !reflect.DeepEqual(clients.avatarRefreshes, want) {
		t.Fatalf("refreshes within the window = %v, want %v", clients.avatarRefreshes, want)
	}

	mr.FastForward(trainerclient.AvatarCheckTTL + time.Second)
	bot.handleMessage(ctx, textFrom(42))
	if want := []string{"42", "42"}; !reflect.DeepEqual(clients.avatarRefreshes, want) {
		t.Fatalf("refreshes after expiry = %v, want %v", clients.avatarRefreshes, want)
	}
}

func TestBot_avatarRefreshIndependentPerUser(t *testing.T) {
	bot, clients, _, _ := avatarTestBot(t)
	ctx := context.Background()

	bot.handleMessage(ctx, textFrom(1))
	bot.handleMessage(ctx, textFrom(2))
	bot.handleMessage(ctx, textFrom(1))
	if want := []string{"1", "2"}; !reflect.DeepEqual(clients.avatarRefreshes, want) {
		t.Fatalf("refreshes = %v, want %v", clients.avatarRefreshes, want)
	}
}

func TestBot_avatarRefreshSkippedWhenRedisDown(t *testing.T) {
	bot, clients, api, mr := avatarTestBot(t)
	mr.Close()

	bot.handleMessage(context.Background(), textFrom(42))
	if len(clients.avatarRefreshes) != 0 {
		t.Fatalf("refreshes = %v, want none", clients.avatarRefreshes)
	}
	if len(api.sent) != 1 {
		t.Fatalf("update not handled: sent = %d", len(api.sent))
	}
}

func TestBot_avatarRefreshWithoutStoreAlwaysRuns(t *testing.T) {
	clients := &fakeTrainerClient{}
	bot := New(&fakeTelegramAPI{}, clients)

	bot.handleMessage(context.Background(), textFrom(42))
	bot.handleMessage(context.Background(), textFrom(42))
	if len(clients.avatarRefreshes) != 2 {
		t.Fatalf("refreshes = %v, want 2", clients.avatarRefreshes)
	}
}
