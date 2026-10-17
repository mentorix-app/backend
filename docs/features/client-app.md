# Client app: program, workout marks, stats

**Status:** implemented  
**Code:** `internal/trainerclient/` (`client_app.go`, `handlers_client_program.go`), `internal/analytics/handlers.go` (`GetClientTrainerAnalytics`), `internal/workoutcompletion/`

## Purpose

The mobile app does what the Telegram bot does for a client: read the program, mark a workout day done with a text result, read the stats with the trainer's replies. The result stays a text. Every answer is a JSON object, so structured results (sets, weight, repetitions) and per-day fields can be added later as new fields.

## Access

- Bearer token. The client is the token's user; no role is required. The trainer is named in the path, `/client/trainers/{trainer_id}/...`, with the ids from `GET /client/trainers`. Nothing is read from the query or the body to pick the trainer, and there is no active-trainer state in Redis for the app.
- The caller needs an active link with that trainer (`trainer_clients`), checked on every call as the bot does. The routes answer differently:

| Route | Not linked (also a deleted user's token) | Blocked by the trainer |
| ----- | ----- | ----- |
| program, completions | `403 client_not_linked` | `422 client_blocked` |
| analytics | `404 client_not_found` | `404 client_not_found` |

The analytics answer is the signed-link page's, kept the same on purpose.

## Program

`GET /client/trainers/{trainer_id}/program` returns `trainer_id`, `trainer_display_name`, `assignment`, `program` and `completed_days`.

- `assignment` has `assignment_id`, `program_id`, `program_version_id`, `completion_cycle_id` and `assigned_at`. `assignment` and `program` are `null` while the trainer has not assigned a program, and `completed_days` is then `[]`.
- `program` is built for the app, not the trainer's editor view: `program_id`, `name`, `name_ru`, `description`, `description_ru`, `category`, `difficulty`, `preview_image_url` and `weeks`. It has no owner ids, counters or draft state. Weeks, days and blocks are the trainer-side shapes, cut to the blocks this client may see ([program-block-visibility.md](program-block-visibility.md)). In a block, `client_user_ids` is `[]` for a shared block and holds only the caller for a restricted one, so other clients are never named.
- Every day carries `day_key`, the stable id of the day. `completed_days` is a list of `{day_key}` for the days marked done in the current completion cycle, ordered by key; it can name days a newer version no longer has.

## Marking a day

`POST /client/trainers/{trainer_id}/completions` with `program_assignment_id`, `program_version_id`, `completion_cycle_id`, `day_key` and `result_text` answers `201` with `id`, `day_key`, `week_number`, `day_number`, `completed_at` and `result_text`.

- The three ids are the ones of the program answer the app was showing. If any of them is no longer current, or the client has no assignment, the answer is `409 program_changed` and nothing is written; the app reloads the program. The cycle is needed because a reassignment keeps the assignment row and a return to an earlier program brings the same version back: only the cycle tells the first stay from the third.
- The write is conditional on those ids too. One statement inserts the row only while the assignment still has the cycle, version, client and trainer and the link is active; it takes no lock on the assignment row, because the trainer's reassignment and account deletion lock the trainer or user first and a lock here made them deadlock. A mark that runs at the same moment as a reassignment may land in the old cycle, as in the bot; a stale request after the reassignment committed is always refused (`409 program_changed`). The checks before the write exist for the clear answers: not linked, blocked, day not found.
- The day is looked up by `day_key` in the visible version. It must hold at least one exercise the caller may see, as in the bot; otherwise `404 day_not_found` (also for a key of another program, and for a day whose blocks are all hidden from this client). The stored snapshot holds only the blocks the client sees.
- A day marked once in a cycle cannot be marked again: `409 workout_already_completed`.
- `result_text` is required, trimmed and at most 1000 characters (counted as characters, not bytes). It may not contain a NUL byte or another C0 control character other than a line break, carriage return or tab: `400` with `result_text contains control characters`. Bad JSON and a missing or zero id are `400` too.
- The body is limited to 16000 bytes (`413 payload_too_large`), checked after the token.
- The row has `source = 'app'`; the bot writes `telegram`. The text rules and the snapshot are those of `workoutcompletion.Service` ([workout-completions.md](workout-completions.md)), which the bot uses; the control-character check is the app's alone.

## Stats

`GET /client/trainers/{trainer_id}/analytics` returns the body of the signed-link page ([client-analytics.md](client-analytics.md)) for the token's user and that trainer. `expires_at` belongs to the signed link: it is `null` here and the link's expiry on the page. An unexpected error keeps its cause for the request log and never puts it in the body.

## Not covered

Structured results, idempotency keys for offline sync, editing or deleting a mark, client comments, trainer notifications for app marks, rate limits.

## See also

- [trainer-invites.md](trainer-invites.md) for `GET /client/trainers`
- [telegram-bot.md](telegram-bot.md)
