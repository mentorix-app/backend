//go:build integration

package storetest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
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

	list, err := svc.ListClients(ctx, trainerUserID, trainerclient.DefaultListParams())
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

	listB, err := svc.ListClients(ctx, trainerB, trainerclient.DefaultListParams())
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

func TestTrainerInvite_adminListsAllClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-list-trainer-a@test.com", pwHash, "Trainer A")
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-list-trainer-b@test.com", pwHash, "Trainer B")
	if err != nil {
		t.Fatalf("register trainer B: %v", err)
	}
	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-list-admin@test.com", pwHash, "Admin")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if err := authStore.GrantRole(ctx, adminID, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM mentorix.trainers WHERE user_id = $1`, adminID)
	if err != nil {
		t.Fatalf("delete admin trainer row: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	acceptClient := func(trainerUserID uuid.UUID, telegramID, name string) {
		t.Helper()
		invite, err := svc.CreateInvite(ctx, trainerUserID)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
		if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
			Token:          token,
			TelegramUserID: telegramID,
			DisplayName:    name,
		}); err != nil {
			t.Fatalf("AcceptInvite %s: %v", name, err)
		}
	}

	acceptClient(trainerA, "880001", "Client One")
	acceptClient(trainerA, "880002", "Client Two")
	acceptClient(trainerB, "880002", "Client Two")
	acceptClient(trainerB, "880003", "Client Three")

	listA, err := svc.ListClients(ctx, trainerA, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients trainer A: %v", err)
	}
	if len(listA.Items) != 2 {
		t.Fatalf("trainer A clients = %d, want 2", len(listA.Items))
	}

	listB, err := svc.ListClients(ctx, trainerB, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients trainer B: %v", err)
	}
	if len(listB.Items) != 2 {
		t.Fatalf("trainer B clients = %d, want 2", len(listB.Items))
	}

	listAdmin, err := svc.ListClients(ctx, adminID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients admin: %v", err)
	}
	if listAdmin.Pagination.Total != 3 {
		t.Fatalf("admin total = %d, want 3", listAdmin.Pagination.Total)
	}
	if len(listAdmin.Items) != 3 {
		t.Fatalf("admin clients = %d, want 3", len(listAdmin.Items))
	}
}

func TestTrainerInvite_adminPrefersOwnTrainerLink(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-prefer-trainer-a@test.com", pwHash, "Trainer A")
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-prefer-trainer-b@test.com", pwHash, "Trainer B")
	if err != nil {
		t.Fatalf("register trainer B: %v", err)
	}
	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-prefer-admin@test.com", pwHash, "Admin Trainer")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if err := authStore.GrantRole(ctx, adminID, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	acceptClient := func(trainerUserID uuid.UUID, telegramID, name string) uuid.UUID {
		t.Helper()
		invite, err := svc.CreateInvite(ctx, trainerUserID)
		if err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
		token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
		result, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
			Token:          token,
			TelegramUserID: telegramID,
			DisplayName:    name,
		})
		if err != nil {
			t.Fatalf("AcceptInvite %s: %v", name, err)
		}
		return result.UserID
	}

	acceptClient(trainerA, "881001", "Shared Client")
	acceptClient(adminID, "881001", "Shared Client")
	sharedClientID := acceptClient(trainerB, "881001", "Shared Client")
	acceptClient(trainerA, "881002", "Other Client")

	var adminLinkedAt, trainerBLinkedAt time.Time
	err = pool.QueryRow(ctx, `
SELECT tc.created_at
FROM mentorix.trainer_clients tc
INNER JOIN mentorix.trainers t ON t.id = tc.trainer_id
WHERE t.user_id = $1 AND tc.client_user_id = $2`, adminID, sharedClientID).Scan(&adminLinkedAt)
	if err != nil {
		t.Fatalf("admin link: %v", err)
	}
	err = pool.QueryRow(ctx, `
SELECT tc.created_at
FROM mentorix.trainer_clients tc
INNER JOIN mentorix.trainers t ON t.id = tc.trainer_id
WHERE t.user_id = $1 AND tc.client_user_id = $2`, trainerB, sharedClientID).Scan(&trainerBLinkedAt)
	if err != nil {
		t.Fatalf("trainer B link: %v", err)
	}
	if !trainerBLinkedAt.After(adminLinkedAt) {
		t.Fatal("expected trainer B link to be later than admin link")
	}

	allList, err := svc.ListClients(ctx, adminID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients admin: %v", err)
	}
	if allList.Pagination.Total != 2 {
		t.Fatalf("admin total = %d, want 2", allList.Pagination.Total)
	}
	var shared *trainerclient.Client
	for i := range allList.Items {
		if allList.Items[i].ClientUserID == sharedClientID {
			item := allList.Items[i]
			shared = &item
			break
		}
	}
	if shared == nil {
		t.Fatal("shared client missing from admin list")
	}
	if !shared.LinkedAt.Equal(adminLinkedAt.UTC()) {
		t.Fatalf("admin list linked_at = %v, want admin link %v", shared.LinkedAt, adminLinkedAt.UTC())
	}
	if shared.LinkedAt.Equal(trainerBLinkedAt.UTC()) {
		t.Fatal("expected admin-prefer link, got trainer B latest link")
	}
	if shared.TrainerUserID != adminID {
		t.Fatalf("trainer_user_id = %v, want admin %v", shared.TrainerUserID, adminID)
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

	_, err = svc.ListClients(ctx, userID, trainerclient.DefaultListParams())
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

	list, err := svc.ListClients(ctx, trainerUserID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].DisplayName != "Client" {
		t.Fatalf("clients = %+v, want display_name Client", list.Items)
	}
}

func TestTrainerInvite_listClientsSearchAndSort(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "search-sort-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	names := []string{"Charlie", "Alice", "Bob"}
	for i, name := range names {
		invite, err := svc.CreateInvite(ctx, trainerUserID)
		if err != nil {
			t.Fatalf("CreateInvite %d: %v", i, err)
		}
		token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
		if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
			Token:          token,
			TelegramUserID: fmt.Sprintf("90000%d", i),
			DisplayName:    name,
		}); err != nil {
			t.Fatalf("AcceptInvite %s: %v", name, err)
		}
	}

	searchParams, err := trainerclient.ParseListParams("", "", "", "", "ob")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}
	search, err := svc.ListClients(ctx, trainerUserID, searchParams)
	if err != nil {
		t.Fatalf("ListClients search: %v", err)
	}
	if len(search.Items) != 1 || search.Items[0].DisplayName != "Bob" {
		t.Fatalf("search items = %+v, want Bob only", search.Items)
	}
	if search.Pagination.Total != 1 {
		t.Fatalf("search total = %d, want 1", search.Pagination.Total)
	}

	sortParams, err := trainerclient.ParseListParams("", "", "name", "asc", "")
	if err != nil {
		t.Fatalf("ParseListParams sort: %v", err)
	}
	sorted, err := svc.ListClients(ctx, trainerUserID, sortParams)
	if err != nil {
		t.Fatalf("ListClients sort: %v", err)
	}
	if len(sorted.Items) != 3 {
		t.Fatalf("sorted count = %d, want 3", len(sorted.Items))
	}
	if sorted.Items[0].DisplayName != "Alice" || sorted.Items[1].DisplayName != "Bob" || sorted.Items[2].DisplayName != "Charlie" {
		t.Fatalf("sorted order = %v %v %v", sorted.Items[0].DisplayName, sorted.Items[1].DisplayName, sorted.Items[2].DisplayName)
	}
	if sorted.Pagination.Total != 3 || sorted.Pagination.TotalPages != 1 {
		t.Fatalf("pagination = %+v", sorted.Pagination)
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

func TestTrainerClient_listIncludesAvatarURL(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "avatar-list-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	const jwtSecret = "test-jwt-secret-at-least-32-chars-long"
	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil,
		trainerclient.WithAvatarSupport(jwtSecret, "123:test-token", nil),
	)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
	result, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "909101",
		DisplayName:    "Avatar Client",
	})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	_, err = pool.Exec(ctx, `UPDATE mentorix.users SET avatar_file_path = $1 WHERE id = $2`, "photos/test.jpg", result.UserID)
	if err != nil {
		t.Fatalf("set avatar path: %v", err)
	}

	list, err := svc.ListClients(ctx, trainerUserID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("clients = %d, want 1", len(list.Items))
	}
	if list.Items[0].AvatarURL == "" {
		t.Fatal("expected signed avatar_url")
	}
	if !strings.Contains(list.Items[0].AvatarURL, "/avatar?exp=") {
		t.Fatalf("avatar_url = %q", list.Items[0].AvatarURL)
	}

	h := trainerclient.NewHandlers(svc, pool, jwtSecret)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, list.Items[0].AvatarURL, nil)
	rec := httptest.NewRecorder()
	echoCtx := e.NewContext(req, rec)
	echoCtx.SetParamNames("client_user_id")
	echoCtx.SetParamValues(result.UserID.String())

	err = h.GetClientAvatar(echoCtx)
	if err == nil {
		t.Fatal("expected error without real Telegram file")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || (he.Code != http.StatusBadGateway && he.Code != http.StatusNotFound) {
		t.Fatalf("GetClientAvatar error = %v, want 404 or 502", err)
	}
}

type integrationPhotos struct {
	path string
}

func (p integrationPhotos) ProfilePhotoFilePath(context.Context, string) (string, bool, error) {
	if p.path == "" {
		return "", false, nil
	}
	return p.path, true, nil
}

func TestTrainerClient_refreshTelegramAvatar(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "refresh-avatar-trainer@test.com", pwHash, "Trainer")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil,
		trainerclient.WithAvatarSupport("test-jwt-secret-at-least-32-chars-long", "token", integrationPhotos{path: "photos/refreshed.jpg"}),
	)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
	result, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "919202",
		DisplayName:    "Refresh Client",
	})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	if err := svc.RefreshTelegramAvatar(ctx, "919202"); err != nil {
		t.Fatalf("RefreshTelegramAvatar: %v", err)
	}

	var path string
	if err := pool.QueryRow(ctx, `SELECT avatar_file_path FROM mentorix.users WHERE id = $1`, result.UserID).Scan(&path); err != nil {
		t.Fatalf("query avatar: %v", err)
	}
	if path != "photos/refreshed.jpg" {
		t.Fatalf("avatar_file_path = %q", path)
	}
}

func TestTrainerClient_listIncludesProgramAssignmentNames(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-name-trainer@test.com", pwHash, "Trainer")
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
	accept, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "717273",
		DisplayName:    "Program Client",
	})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	draft, err := progSvc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create program: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, progSvc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Strength Plan"
	nameRu := "Силовой план"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := progSvc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		NameRu:     &nameRu,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update program: %v", err)
	}
	if _, err := progSvc.Publish(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	createdAssignment, err := progSvc.SetClientProgramAssignment(ctx, trainerUserID, accept.UserID, &draft.ID)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	list, err := svc.ListClients(ctx, trainerUserID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("clients = %d, want 1", len(list.Items))
	}
	assignment := list.Items[0].ProgramAssignment
	if assignment == nil {
		t.Fatal("expected program_assignment")
	}
	if assignment.ProgramName != name {
		t.Fatalf("program_name = %q, want %q", assignment.ProgramName, name)
	}
	if assignment.ProgramNameRu != nameRu {
		t.Fatalf("program_name_ru = %q, want %q", assignment.ProgramNameRu, nameRu)
	}
	if assignment.AssignmentID != createdAssignment.ID {
		t.Fatalf("assignment_id = %v, want %v", assignment.AssignmentID, createdAssignment.ID)
	}
	if assignment.IsBehindLatest == nil || *assignment.IsBehindLatest {
		t.Fatalf("is_behind_latest = %v, want false on latest version", assignment.IsBehindLatest)
	}

	updatedName := "Strength Plan v2"
	if _, err := progSvc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{Name: &updatedName}); err != nil {
		t.Fatalf("Update after publish: %v", err)
	}
	if _, err := progSvc.PublishUpdate(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("PublishUpdate: %v", err)
	}

	listAfterUpdate, err := svc.ListClients(ctx, trainerUserID, trainerclient.DefaultListParams())
	if err != nil {
		t.Fatalf("ListClients after publish-update: %v", err)
	}
	assignmentAfter := listAfterUpdate.Items[0].ProgramAssignment
	if assignmentAfter == nil {
		t.Fatal("expected program_assignment after publish-update")
	}
	if assignmentAfter.IsBehindLatest == nil || !*assignmentAfter.IsBehindLatest {
		t.Fatalf("is_behind_latest = %v, want true before sync", assignmentAfter.IsBehindLatest)
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
