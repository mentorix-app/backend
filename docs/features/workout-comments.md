# Workout comments

**Status:** implemented  
**Code:** `internal/workoutcomment/`, read in `internal/analytics/`, push in `internal/telegramnotify/`

## Purpose

Trainers reply to a client's workout result (chat in web). Currently limited to **one reply per result**; attempting another returns `409`. Future expansion to mini-chat will remove the UNIQUE constraint and the service check; the API contract stays the same.

## Database

- `client_workout_completion_comments` stores trainer replies with `client_workout_completion_id` (FK CASCADE), `trainer_id`, and `comment_text` (UNIQUE per completion).

Migration: `000025_workout_completion_comments`.

## API

- `POST /trainer/clients/{client_user_id}/completions/{completion_id}/comments` (`createTrainerCompletionComment`) takes `{"text"}` (1–2000 characters) and returns `201` with `{id, text, created_at}`. Returns `404` if the completion doesn't belong to that trainer-client pair; `409` if a reply already exists.
- Replies appear in `items[].comments` of the `GET /trainer/clients/{client_user_id}/completions` log (oldest first, fetched with one request per page).

## Telegram

After saving, a best-effort push goes to the client with the trainer's name, program name (`program_name_ru`), week and day number, a quote of `result_text` (up to 500 characters), and the reply text. If the client has no Telegram link, the push is silently skipped.

## See also

- [workout-completions.md](workout-completions.md)
- [trainer-analytics.md](trainer-analytics.md)
