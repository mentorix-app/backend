//go:build integration

package storetest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func TestTrainerInvite_acceptAndListClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "invite-trainer@test.com", pwHash, "Trainer Anna")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if !strings.Contains(invite.InviteURL, "t.me/mentorix_bot?start=inv_") {
		t.Fatalf("invite_url = %q", invite.InviteURL)
	}

	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")

	result, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "42424242",
		DisplayName:    "Ivan Client",
	})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if result.AlreadyLinked {
		t.Fatal("expected first accept not already linked")
	}
	if result.TrainerDisplayName != "Trainer Anna" {
		t.Fatalf("trainer_display_name = %q", result.TrainerDisplayName)
	}

	list, err := svc.ListClients(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("clients = %d, want 1", len(list.Items))
	}
	if list.Items[0].ClientUserID != result.UserID {
		t.Fatalf("client_user_id mismatch")
	}
	if list.Items[0].DisplayName != "Ivan Client" {
		t.Fatalf("display_name = %q", list.Items[0].DisplayName)
	}

	repeat, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "42424242",
		DisplayName:    "Ivan Client",
	})
	if err != nil {
		t.Fatalf("AcceptInvite repeat: %v", err)
	}
	if !repeat.AlreadyLinked {
		t.Fatal("expected already linked on repeat")
	}

	q := sqlc.New(pool)
	roles, err := q.ListUserRoles(ctx, pgconv.ToPGUUID(result.UserID))
	if err != nil {
		t.Fatalf("ListUserRoles: %v", err)
	}
	if !contains(roles, auth.RoleClient) {
		t.Fatalf("roles = %v, want client", roles)
	}
}

func TestTrainerInvite_notFound(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	_, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          "missing-token",
		TelegramUserID: "12345",
	})
	if !errors.Is(err, trainerclient.ErrInviteNotFound) {
		t.Fatalf("error = %v, want ErrInviteNotFound", err)
	}
}

func TestTrainerInvite_existingTelegramSecondTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-a@test.com", pwHash, "Trainer A")
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-b@test.com", pwHash, "Trainer B")
	if err != nil {
		t.Fatalf("register trainer B: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	inviteA, err := svc.CreateInvite(ctx, trainerA)
	if err != nil {
		t.Fatalf("invite A: %v", err)
	}
	tokenA := strings.TrimPrefix(strings.Split(inviteA.InviteURL, "start=")[1], "inv_")

	if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          tokenA,
		TelegramUserID: "777001",
		DisplayName:    "Shared Client",
	}); err != nil {
		t.Fatalf("accept A: %v", err)
	}

	inviteB, err := svc.CreateInvite(ctx, trainerB)
	if err != nil {
		t.Fatalf("invite B: %v", err)
	}
	tokenB := strings.TrimPrefix(strings.Split(inviteB.InviteURL, "start=")[1], "inv_")

	result, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          tokenB,
		TelegramUserID: "777001",
		DisplayName:    "Shared Client",
	})
	if err != nil {
		t.Fatalf("accept B: %v", err)
	}
	if result.AlreadyLinked {
		t.Fatal("expected new trainer link, not already linked to B")
	}

	listB, err := svc.ListClients(ctx, trainerB)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(listB.Items) != 1 {
		t.Fatalf("trainer B clients = %d, want 1", len(listB.Items))
	}
}

func TestTrainerInvite_createForbiddenForNonTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "later-client@test.com", pwHash, "User")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	_, err = pool.Exec(ctx, `DELETE FROM mentorix.trainers WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("delete trainer row: %v", err)
	}

	_, err = svc.CreateInvite(ctx, userID)
	if !errors.Is(err, program.ErrForbidden) {
		t.Fatalf("CreateInvite error = %v, want ErrForbidden", err)
	}
}

func TestTrainerInvite_notConfigured(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "noconfig-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		InviteTTL: time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	_, err = svc.CreateInvite(ctx, trainerUserID)
	if !errors.Is(err, trainerclient.ErrInviteNotConfigured) {
		t.Fatalf("error = %v, want ErrInviteNotConfigured", err)
	}
}

func TestTrainerInvite_listForbiddenForNonTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "list-nontrainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM mentorix.trainers WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("delete trainer row: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	_, err = svc.ListClients(ctx, userID)
	if !errors.Is(err, program.ErrForbidden) {
		t.Fatalf("ListClients error = %v, want ErrForbidden", err)
	}
}

func TestTrainerInvite_acceptWithEmptyDisplayName(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "empty-name-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")

	if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "606001",
		DisplayName:    "   ",
	}); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	list, err := svc.ListClients(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].DisplayName != "Client" {
		t.Fatalf("clients = %+v, want display_name Client", list.Items)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestTrainerInvite_expired(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "expired-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")

	_, err = pool.Exec(ctx, `UPDATE mentorix.trainer_invites SET expires_at = now() - interval '1 hour' WHERE token = $1`, token)
	if err != nil {
		t.Fatalf("expire invite: %v", err)
	}

	_, err = svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "999001",
		DisplayName:    "Late Client",
	})
	if !errors.Is(err, trainerclient.ErrInviteExpired) {
		t.Fatalf("AcceptInvite error = %v, want ErrInviteExpired", err)
	}
}

func TestTrainerInvite_consumedByOtherUser(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "consumed-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")

	if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "111001",
		DisplayName:    "First",
	}); err != nil {
		t.Fatalf("first accept: %v", err)
	}

	_, err = svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "111002",
		DisplayName:    "Second",
	})
	if !errors.Is(err, trainerclient.ErrInviteConsumed) {
		t.Fatalf("AcceptInvite error = %v, want ErrInviteConsumed", err)
	}
}

func TestTrainerInvite_blockedClient(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	q := sqlc.New(pool)

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "blocked-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "blocked-client@test.com", pwHash, "Client")
	if err != nil {
		t.Fatalf("register client user: %v", err)
	}

	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status, blocked_at)
		VALUES ($1, $2, 'blocked', now())
	`, trainerID, clientUserID)
	if err != nil {
		t.Fatalf("insert blocked link: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO mentorix.auth_identities (user_id, provider, subject)
		VALUES ($1, $2, $3)
	`, clientUserID, auth.ProviderTelegram, "555001")
	if err != nil {
		t.Fatalf("insert telegram identity: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")

	_, err = svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "555001",
		DisplayName:    "Blocked",
	})
	if !errors.Is(err, program.ErrClientBlocked) {
		t.Fatalf("AcceptInvite error = %v, want ErrClientBlocked", err)
	}
}

func TestTrainerInvite_handlersCreateListAccept(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "handler-trainer@test.com", pwHash, "Handler Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)
	h := trainerclient.NewHandlers(svc, pool, "test-jwt-secret-at-least-32-chars-long")
	e := echo.New()

	createReq := httptest.NewRequest(http.MethodPost, "/trainer/invites", nil)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.Set(auth.ContextUserIDKey, trainerUserID)
	if err := h.CreateInvite(createCtx); err != nil {
		t.Fatalf("CreateInvite handler: %v", err)
	}
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", createRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/trainer/clients", nil)
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)
	listCtx.Set(auth.ContextUserIDKey, trainerUserID)
	if err := h.ListClients(listCtx); err != nil {
		t.Fatalf("ListClients handler: %v", err)
	}
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite svc: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
	if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "808001",
		DisplayName:    "Handler Client",
	}); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
}

func TestTrainerInvite_storeEmptyToken(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := trainerclient.NewStore(pool)

	_, err := store.AcceptInvite(ctx, "  ", "12345", "Name")
	if !errors.Is(err, trainerclient.ErrInviteNotFound) {
		t.Fatalf("error = %v, want ErrInviteNotFound", err)
	}
}
