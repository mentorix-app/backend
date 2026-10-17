# Workout completions

**Status:** implemented (bot, database, app)  
**Code:** `internal/workoutcompletion/`, `internal/telegrambot/workout.go`, `internal/trainerclient/client_app.go`

## Purpose

Clients mark workout days in Telegram or in the app (result as text goes to the log). History is not deleted during sync, clearing, or reassignment. Checkmarks are tracked by `(completion_cycle_id, day_key)` pair.

## Database

- `program_week_days.day_key` and `program_version_week_days.day_key` (copied on freeze)
- `program_assignments.completion_cycle_id` (new on assignment or `program_id` change; stays the same on sync; repeating the same `program_id` in `PUT` returns `already_assigned`)
- `client_workout_completions` is the log; `source` is `telegram` or `app`

Migrations: `000020_workout_completions`, `000033_workout_completion_source_app` (widens the source check).

## Telegram

On the day screen, «Тренировка выполнена» prompts for a result and offers «Отмена»; on submit, the result is inserted and shown as ✅.  
In day selection, completed days show «✅ День N»; a week with all workouts done shows «✅ Неделя N».  
A pending state is stored in Redis at `mentorix:telegram:workout_pending:{telegram_user_id}` with a 30-minute TTL; any bot button press clears it.

## App

The app marks a day with `POST /client/trainers/{trainer_id}/completions` and reads the checkmarks as `completed_days`. The checks and answers are in [client-app.md](client-app.md); the row is the same, with `source = 'app'`. The app writes through `CompleteForAssignment`, one conditional insert that needs the assignment to still have the cycle the client saw; the bot keeps `Complete`.

## See also

- [telegram-bot.md](telegram-bot.md)
- [programs.md](programs.md)
- [trainer-clients.md](trainer-clients.md)
