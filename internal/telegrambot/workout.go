package telegrambot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/workoutcompletion"
)

const (
	programDayDoneCallbackPrefix     = "program_day_done:"
	programDayDoneCancelCallbackData = "program_day_done_cancel"
	askResultText                    = "Напишите результат тренировки (как прошло, веса, самочувствие) и отправьте сообщением."
	alreadyCompletedCallbackText     = "Уже отмечено"
	resultEmptyText                  = "Результат не должен быть пустым. Напишите ещё раз или нажмите «Отмена»."
	resultTooLongText                = "Слишком длинный текст (макс. 1000 символов). Сократите и отправьте снова."
	btnCompleteWorkout               = "Тренировка выполнена"
	btnCompleteWorkoutDone           = "✅ Тренировка выполнена"
	btnCancelPending                 = "Отмена"
)

func programDayDoneCallbackData(weekNumber, dayNumber int) string {
	return fmt.Sprintf("%s%d:%d", programDayDoneCallbackPrefix, weekNumber, dayNumber)
}

func parseProgramDayDoneCallback(data string) (weekNum, dayNum int, ok bool) {
	if !strings.HasPrefix(data, programDayDoneCallbackPrefix) {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(data, programDayDoneCallbackPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	weekNum, err := strconv.Atoi(parts[0])
	if err != nil || weekNum <= 0 {
		return 0, 0, false
	}
	dayNum, err = strconv.Atoi(parts[1])
	if err != nil || dayNum <= 0 {
		return 0, 0, false
	}
	return weekNum, dayNum, true
}

func programDayKeyboard(weeks []program.Week, weekNumber, dayNumber int, completed bool) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 2)
	label := btnCompleteWorkout
	if completed {
		label = btnCompleteWorkoutDone
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(label, programDayDoneCallbackData(weekNumber, dayNumber)),
	))
	nav := programDayNavKeyboard(weeks, weekNumber, dayNumber)
	rows = append(rows, nav.InlineKeyboard...)
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func pendingCancelKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(btnCancelPending, programDayDoneCancelCallbackData),
	))
}

func (b *Bot) clearWorkoutPending(ctx context.Context, telegramUserID string) {
	if b.pending == nil || telegramUserID == "" {
		return
	}
	_ = b.pending.Delete(ctx, telegramUserID)
}

func (b *Bot) handleProgramDayDone(ctx context.Context, chatID int64, tgID string, weekNumber, dayNumber int, queryID string) {
	if b.workouts == nil || b.pending == nil {
		b.answerCallback(queryID, "")
		return
	}
	resp, err := b.clients.GetTelegramProgram(ctx, tgID, nil)
	if err != nil || !resp.HasProgram || resp.Program == nil || resp.Assignment == nil {
		b.answerCallback(queryID, "")
		b.sendText(chatID, formatProgramSummary(resp), mainMenuKeyboard())
		return
	}
	week, ok := findWeek(resp.Program.Weeks, weekNumber)
	if !ok {
		b.answerCallback(queryID, "")
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	day, ok := findDay(week.Days, dayNumber)
	if !ok || !dayHasExercises(*day) {
		b.answerCallback(queryID, "")
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}

	done, err := b.workouts.IsCompleted(ctx, resp.Assignment.CompletionCycleID, day.DayKey)
	if err != nil {
		b.answerCallback(queryID, "")
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	if done {
		b.answerCallback(queryID, alreadyCompletedCallbackText)
		return
	}

	clientUserID, err := b.clients.ClientUserIDByTelegram(ctx, tgID)
	if err != nil {
		b.answerCallback(queryID, "")
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}

	pend := workoutcompletion.Pending{
		ClientUserID:        clientUserID,
		TrainerID:           resp.TrainerID,
		ProgramID:           resp.Assignment.ProgramID,
		ProgramVersionID:    resp.Assignment.ProgramVersionID,
		ProgramAssignmentID: resp.Assignment.AssignmentID,
		CompletionCycleID:   resp.Assignment.CompletionCycleID,
		DayKey:              day.DayKey,
		WeekNumber:          weekNumber,
		DayNumber:           dayNumber,
		ProgramName:         resp.Program.Name,
		ProgramNameRu:       resp.Program.NameRu,
	}
	if err := b.pending.Set(ctx, tgID, pend); err != nil {
		b.answerCallback(queryID, "")
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	b.answerCallback(queryID, "")
	b.sendMarkdownWithInline(chatID, askResultText, mainMenuKeyboard(), pendingCancelKeyboard())
}

func (b *Bot) handlePendingCancel(ctx context.Context, chatID int64, tgID string) {
	var weekNumber, dayNumber int
	if b.pending != nil {
		if pend, ok, err := b.pending.Get(ctx, tgID); err == nil && ok && pend != nil {
			weekNumber, dayNumber = pend.WeekNumber, pend.DayNumber
		}
	}
	b.clearWorkoutPending(ctx, tgID)
	if weekNumber > 0 && dayNumber > 0 {
		b.handleProgramDay(ctx, chatID, tgID, weekNumber, dayNumber)
		return
	}
	b.sendText(chatID, "Отменено.", mainMenuKeyboard())
}

func (b *Bot) tryHandleWorkoutResult(ctx context.Context, msg *tgbotapi.Message) bool {
	if b.workouts == nil || b.pending == nil || msg == nil || msg.From == nil {
		return false
	}
	tgID := telegramUserID(msg.From)
	pend, ok, err := b.pending.Get(ctx, tgID)
	if err != nil || !ok || pend == nil {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		b.sendMarkdownWithInline(msg.Chat.ID, resultEmptyText, mainMenuKeyboard(), pendingCancelKeyboard())
		return true
	}

	resp, err := b.clients.GetTelegramProgram(ctx, tgID, nil)
	if err != nil || !resp.HasProgram || resp.Program == nil {
		b.clearWorkoutPending(ctx, tgID)
		b.sendText(msg.Chat.ID, formatProgramSummary(resp), mainMenuKeyboard())
		return true
	}
	week, ok := findWeek(resp.Program.Weeks, pend.WeekNumber)
	if !ok {
		b.clearWorkoutPending(ctx, tgID)
		b.sendText(msg.Chat.ID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return true
	}
	day, ok := findDay(week.Days, pend.DayNumber)
	if !ok || day.DayKey != pend.DayKey {
		b.clearWorkoutPending(ctx, tgID)
		b.sendText(msg.Chat.ID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return true
	}

	_, err = b.workouts.Complete(ctx, workoutcompletion.CompleteInput{
		ClientUserID:        pend.ClientUserID,
		TrainerID:           pend.TrainerID,
		ProgramID:           pend.ProgramID,
		ProgramVersionID:    pend.ProgramVersionID,
		ProgramAssignmentID: pend.ProgramAssignmentID,
		CompletionCycleID:   pend.CompletionCycleID,
		DayKey:              pend.DayKey,
		WeekNumber:          pend.WeekNumber,
		DayNumber:           pend.DayNumber,
		ProgramName:         pend.ProgramName,
		ProgramNameRu:       pend.ProgramNameRu,
		Day:                 *day,
		ResultText:          text,
		Source:              workoutcompletion.SourceTelegram,
	})
	if err != nil {
		if errors.Is(err, workoutcompletion.ErrAlreadyCompleted) {
			b.clearWorkoutPending(ctx, tgID)
			b.handleProgramDay(ctx, msg.Chat.ID, tgID, pend.WeekNumber, pend.DayNumber)
			return true
		}
		if errors.Is(err, workoutcompletion.ErrValidation) && strings.Contains(err.Error(), "too long") {
			b.sendMarkdownWithInline(msg.Chat.ID, resultTooLongText, mainMenuKeyboard(), pendingCancelKeyboard())
			return true
		}
		if errors.Is(err, workoutcompletion.ErrValidation) {
			b.sendMarkdownWithInline(msg.Chat.ID, resultEmptyText, mainMenuKeyboard(), pendingCancelKeyboard())
			return true
		}
		b.clearWorkoutPending(ctx, tgID)
		b.sendText(msg.Chat.ID, menuErrorText(err), mainMenuKeyboard())
		return true
	}
	b.clearWorkoutPending(ctx, tgID)
	b.handleProgramDay(ctx, msg.Chat.ID, tgID, pend.WeekNumber, pend.DayNumber)
	return true
}

func (b *Bot) answerCallback(queryID, text string) {
	if queryID == "" {
		return
	}
	cb := tgbotapi.NewCallback(queryID, text)
	cb.ShowAlert = text != ""
	_, _ = b.api.Request(cb)
}
