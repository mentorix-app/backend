# Workout completions

**Status:** implemented (phase 1 — bot + database)  
**Code:** `internal/workoutcompletion/`, `internal/telegrambot/workout.go`

## Purpose

Clients mark workout days in Telegram (result as text goes to the log). History is not deleted during sync, clearing, or reassignment. Checkmarks are tracked by `(completion_cycle_id, day_key)` pair.

## Database

- `program_week_days.day_key` and `program_version_week_days.day_key` (copied on freeze)
- `program_assignments.completion_cycle_id` (new on assignment or `program_id` change; stays the same on sync; repeating the same `program_id` in `PUT` returns `already_assigned`)
- `client_workout_completions` is the log

Migration: `000020_workout_completions`.

## Telegram

On the day screen, «Тренировка выполнена» prompts for a result and offers «Отмена»; on submit, the result is inserted and shown as ✅.  
In day selection, completed days show «✅ День N»; a week with all workouts done shows «✅ Неделя N».  
A pending state is stored in Redis at `mentorix:telegram:workout_pending:{telegram_user_id}` with a 30-minute TTL; any bot button press clears it.

## See also

- [telegram-bot.md](telegram-bot.md)
- [programs.md](programs.md)
- [trainer-clients.md](trainer-clients.md)
