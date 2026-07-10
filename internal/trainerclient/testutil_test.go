package trainerclient_test

import (
	"mentorix-backend/internal/trainerclient"
)

func testHandlers(programs trainerclient.ClientProgramService) *trainerclient.Handlers {
	svc := trainerclient.NewService(nil, programs, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           0,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)
	return trainerclient.NewHandlers(svc, nil, "test-jwt-secret-at-least-32-chars-long")
}
