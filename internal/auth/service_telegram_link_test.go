package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeLinkCodes struct {
	telegramUserID string
	ok             bool
	err            error
	taken          []string
}

func (f *fakeLinkCodes) Take(_ context.Context, code string) (string, bool, error) {
	f.taken = append(f.taken, code)
	return f.telegramUserID, f.ok, f.err
}

func testLinkService(store authStore, codes linkCodeTaker) *Service {
	svc := testAuthService(store)
	svc.linkCodes = codes
	return svc
}

func TestService_LinkTelegram_success(t *testing.T) {
	appUser, remaining := uuid.New(), uuid.New()
	store := &fakeAuthStore{linkRemaining: remaining, profile: UserProfile{Email: "old@example.com"}}
	codes := &fakeLinkCodes{telegramUserID: "tg-100", ok: true}
	svc := testLinkService(store, codes)

	issued, err := svc.LinkTelegram(context.Background(), appUser, " abcd2345 ")
	if err != nil {
		t.Fatalf("LinkTelegram() error = %v", err)
	}
	if issued.UserID != remaining {
		t.Errorf("session user = %v, want the remaining account %v", issued.UserID, remaining)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" {
		t.Fatal("expected access and refresh tokens")
	}
	if issued.Email != "old@example.com" {
		t.Errorf("email = %q", issued.Email)
	}
	want := []linkTelegramCall{{appUser, "tg-100"}}
	if len(store.linkCalls) != 1 || store.linkCalls[0] != want[0] {
		t.Errorf("store calls = %+v, want %+v", store.linkCalls, want)
	}
	if len(codes.taken) != 1 || codes.taken[0] != " abcd2345 " {
		t.Errorf("codes taken = %q", codes.taken)
	}
}

func TestService_LinkTelegram_codeProblems(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		codes linkCodeTaker
		want  error
	}{
		{"empty code", "", &fakeLinkCodes{}, ErrInvalidLinkCode},
		{"blank code", "   ", &fakeLinkCodes{}, ErrInvalidLinkCode},
		{"unknown code", "ABCD2345", &fakeLinkCodes{ok: false}, ErrInvalidLinkCode},
		{"store unavailable", "ABCD2345", &fakeLinkCodes{err: ErrLinkCodesUnavailable}, ErrLinkCodesUnavailable},
		{"no code store configured", "ABCD2345", nil, ErrLinkCodesUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := testLinkService(store, tt.codes)
			issued, err := svc.LinkTelegram(context.Background(), uuid.New(), tt.code)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if issued.AccessToken != "" || issued.RefreshToken != "" {
				t.Error("no tokens on error")
			}
			if len(store.linkCalls) != 0 {
				t.Error("store must not be called")
			}
		})
	}
}

func TestService_LinkTelegram_emptyCodeDoesNotTouchRedis(t *testing.T) {
	codes := &fakeLinkCodes{}
	svc := testLinkService(&fakeAuthStore{}, codes)
	if _, err := svc.LinkTelegram(context.Background(), uuid.New(), ""); !errors.Is(err, ErrInvalidLinkCode) {
		t.Fatalf("error = %v", err)
	}
	if len(codes.taken) != 0 {
		t.Errorf("codes taken = %q, want none", codes.taken)
	}
}

func TestService_LinkTelegram_storeErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		store *fakeAuthStore
		want  error
	}{
		{"conflict", &fakeAuthStore{linkErr: ErrTelegramLinkConflict}, ErrTelegramLinkConflict},
		{"already linked", &fakeAuthStore{linkErr: ErrTelegramAlreadyLinked}, ErrTelegramAlreadyLinked},
		{"app user gone", &fakeAuthStore{linkErr: pgx.ErrNoRows}, pgx.ErrNoRows},
		{"link", &fakeAuthStore{linkErr: boom}, boom},
		{"profile", &fakeAuthStore{profileErr: boom}, boom},
		{"session", &fakeAuthStore{insertSession: boom}, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := testLinkService(tt.store, &fakeLinkCodes{telegramUserID: "tg-1", ok: true})
			issued, err := svc.LinkTelegram(context.Background(), uuid.New(), "ABCD2345")
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if issued.AccessToken != "" || issued.RefreshToken != "" {
				t.Error("no tokens on error")
			}
		})
	}
}

func TestService_LinkTelegram_redisFailureIsUnavailable(t *testing.T) {
	store, mr := newLinkCodeTestStore(t)
	mr.Close()
	fake := &fakeAuthStore{}
	svc := testLinkService(fake, store)

	_, err := svc.LinkTelegram(context.Background(), uuid.New(), "ABCD2345")

	if !errors.Is(err, ErrLinkCodesUnavailable) {
		t.Fatalf("error = %v, want ErrLinkCodesUnavailable", err)
	}
	if len(fake.linkCalls) != 0 {
		t.Error("store must not be called")
	}
}
