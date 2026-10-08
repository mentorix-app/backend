package telegrambot

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"mentorix-backend/internal/trainerclient"
)

type fakeLinkIssuer struct {
	code  string
	err   error
	users []string
}

func (f *fakeLinkIssuer) Issue(_ context.Context, telegramUserID string) (string, error) {
	f.users = append(f.users, telegramUserID)
	return f.code, f.err
}

func privateLinkMessage() *tgbotapi.Message {
	msg := commandMessage("start", "link")
	msg.Chat.Type = "private"
	return msg
}

func TestBot_handleStart_link_sendsCode(t *testing.T) {
	api := &fakeTelegramAPI{}
	backend := &fakeTrainerClient{}
	issuer := &fakeLinkIssuer{code: "ABCD2345"}
	bot := New(api, backend, WithLinkCodes(issuer))

	bot.handleStart(context.Background(), privateLinkMessage())

	if len(issuer.users) != 1 || issuer.users[0] != "42" {
		t.Fatalf("issued for %v, want [42]", issuer.users)
	}
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(api.sent))
	}
	got := api.sent[0]
	if got.ChatID != 100 {
		t.Errorf("chat id = %d", got.ChatID)
	}
	if !strings.Contains(got.Text, "\n`ABCD2345`\n") {
		t.Errorf("code is not on its own line in monospace: %q", got.Text)
	}
	for _, want := range []string{"10 минут", "приложени", "Mentorix", "Telegram", "никому"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("text lacks %q: %q", want, got.Text)
		}
	}
	if len(backend.calls) != 0 {
		t.Errorf("invite accepted for /start link: %+v", backend.calls)
	}
}

func TestBot_handleStart_link_issueFailureAsksToRetry(t *testing.T) {
	api := &fakeTelegramAPI{}
	issuer := &fakeLinkIssuer{err: errors.New("redis down")}
	bot := New(api, &fakeTrainerClient{}, WithLinkCodes(issuer))

	bot.handleStart(context.Background(), privateLinkMessage())

	if len(api.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(api.sent))
	}
	if !strings.Contains(api.sent[0].Text, "позже") {
		t.Errorf("text = %q, want a try-later message", api.sent[0].Text)
	}
	if strings.Contains(api.sent[0].Text, "redis") {
		t.Errorf("text leaks the cause: %q", api.sent[0].Text)
	}
}

func TestBot_handleStart_link_withoutIssuerFallsThrough(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})

	bot.handleStart(context.Background(), commandMessage("start", "link"))

	if len(api.sent) != 1 || api.sent[0].Text != "Откройте ссылку-приглашение от тренера или выберите пункт меню." {
		t.Fatalf("sent = %+v, want the default start text", api.sent)
	}
}

func TestBot_handleStart_otherArgumentsIgnoreIssuer(t *testing.T) {
	for _, arg := range []string{"", "links", "LINK", "link_x", "inv_tok123"} {
		t.Run(arg, func(t *testing.T) {
			api := &fakeTelegramAPI{}
			backend := &fakeTrainerClient{result: trainerclient.AcceptInviteResult{TrainerDisplayName: "Anna"}}
			issuer := &fakeLinkIssuer{code: "ABCD2345"}
			bot := New(api, backend, WithLinkCodes(issuer))

			bot.handleStart(context.Background(), commandMessage("start", arg))

			if len(issuer.users) != 0 {
				t.Errorf("issued a code for argument %q", arg)
			}
			if wantInvite := arg == "inv_tok123"; (len(backend.calls) == 1) != wantInvite {
				t.Errorf("invite calls = %+v for argument %q", backend.calls, arg)
			}
		})
	}
}

func TestBot_handleStart_link_onlyInPrivateChatOfHuman(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*tgbotapi.Message)
	}{
		{"group chat", func(m *tgbotapi.Message) { m.Chat.Type = "group" }},
		{"supergroup chat", func(m *tgbotapi.Message) { m.Chat.Type = "supergroup" }},
		{"channel chat", func(m *tgbotapi.Message) { m.Chat.Type = "channel" }},
		{"nil chat", func(m *tgbotapi.Message) { m.Chat = nil }},
		{"nil sender", func(m *tgbotapi.Message) { m.From = nil }},
		{"bot sender", func(m *tgbotapi.Message) { m.From.IsBot = true }},
		{"zero user id", func(m *tgbotapi.Message) { m.From.ID = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeTelegramAPI{}
			issuer := &fakeLinkIssuer{code: "ABCD2345"}
			bot := New(api, &fakeTrainerClient{}, WithLinkCodes(issuer))
			msg := commandMessage("start", "link")
			msg.Chat.Type = "private"
			tt.mutate(msg)

			bot.handleStart(context.Background(), msg)

			if len(issuer.users) != 0 {
				t.Fatalf("issued a code for %v", issuer.users)
			}
			for _, sent := range api.sent {
				if strings.Contains(sent.Text, "ABCD2345") {
					t.Errorf("reply carries a code: %q", sent.Text)
				}
			}
			if msg.Chat != nil && len(api.sent) != 1 {
				t.Fatalf("sent = %d, want one short reply", len(api.sent))
			}
			if msg.Chat != nil && !strings.Contains(api.sent[0].Text, "личные сообщения") {
				t.Errorf("reply = %q, want a private chat hint", api.sent[0].Text)
			}
		})
	}
}
