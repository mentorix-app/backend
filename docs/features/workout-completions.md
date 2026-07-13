# Workout completions

**Статус:** реализовано (фаза 1 — бот + БД)  
**Код:** `internal/workoutcompletion/`, `internal/telegrambot/workout.go`

## Назначение

Клиент отмечает день программы в Telegram (результат текстом → журнал).  
История не удаляется при sync/clear/reassign. Галочки по `(completion_cycle_id, day_key)`.

## БД

- `program_week_days.day_key`, `program_version_week_days.day_key` (копия при freeze)
- `program_assignments.completion_cycle_id` (новый при assign / смене `program_id`; не при sync; PUT той же программы → skip `already_assigned`)
- `client_workout_completions` — журнал

Миграция: `000020_workout_completions`.

## Telegram

На экране дня: «Тренировка выполнена» → запрос результата + «Отмена» → INSERT → ✅.  
Pending Redis `mentorix:telegram:workout_pending:{telegram_user_id}` TTL 30м; сброс при любой кнопке бота.

## См. также

- [telegram-bot.md](telegram-bot.md)
- [programs.md](programs.md)
- [trainer-clients.md](trainer-clients.md)
