# Plan: Program, workout mark and history for the client app

**Source**: the frontend developer's voice message of 2026-10-17 (the mobile developer needs to read the client's program and send feedback on a workout), the paused "client screens" item of `.claude/prds/mobile-client-sign-in.prd.md`
**Complexity**: Medium, one pull request with one small migration

## Summary

The bot already shows a client their program and lets them mark a workout day done with a text result. The app has no HTTP way to do either. This plan adds three endpoints over the services the bot uses: read the program from one trainer, mark a day done, read the stats with the trainer's replies. The result stays a text, as in the bot. The answers are objects, so structured results (sets, weight, repetitions) can be added later as new fields without breaking the app.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Which trainer | The app names it in the path: `/client/trainers/{trainer_id}/...`. The list comes from `GET /client/trainers` | The bot keeps an "active trainer" in Redis per Telegram user; the app does not need that state |
| Access | Bearer token; the caller must have an active link with that trainer | Same rule as the bot (`TrainerLinkedToClient`) |
| Program | `GET /client/trainers/{trainer_id}/program`: trainer, the current assignment or `null`, the program version with only the blocks the client may see, and the day keys already done in this assignment | Same view as the bot (`GetVersionDetailForClient`); the done keys let the app draw checkmarks without another call |
| Mark done | `POST /client/trainers/{trainer_id}/completions` with `program_assignment_id`, `program_version_id`, `day_key`, `result_text` | The ids say which assignment the app was looking at. If the trainer reassigned meanwhile, the server refuses instead of writing to the wrong cycle, as the bot does |
| Result | `result_text` required, at most 1000 characters, as in the bot | Structured results are a later plan; free text then becomes an optional note |
| Repeat | A day already done answers `409 workout_already_completed` | The database already refuses a second row for the same day of a cycle |
| Stats and replies | `GET /client/trainers/{trainer_id}/analytics`: the same body as the signed-link page `GET /client/analytics`, by token | The client reads the trainer's replies to their workouts; the service already exists (`ClientSelfAnalytics`) |
| Source | New value `app` for `client_workout_completions.source` | Today the CHECK allows only `telegram`; the trainer and the analytics should know where a mark came from |
| Telegram | Unchanged | The bot keeps working as before |

Out of scope: structured results, idempotency keys for offline sync, editing or deleting a mark, trainer notifications, push, sending comments from the client, the trainer side.

### Answers

| Endpoint | Success | Errors |
|---|---|---|
| `GET .../program` | `200` | `401`; `403 client_not_linked`; `422 client_blocked` |
| `POST .../completions` | `201` with `id`, `day_key`, `week_number`, `day_number`, `completed_at`, `result_text` | `400` empty or too long `result_text`, bad JSON or ids; `401`; `403 client_not_linked`; `404 day_not_found`; `409 program_changed` (the assignment or version is no longer current, or there is none); `409 workout_already_completed`; `422 client_blocked` |
| `GET .../analytics` | `200` (body of `GET /client/analytics`) | `401`; `404 client_not_found` (not linked or blocked, same as the page) |

The exact codes follow the codes that already exist for the same texts.

## Migration

`000033`: the CHECK on `client_workout_completions.source` becomes `source IN ('telegram', 'app')`. It only widens the set, so the running code stays valid before and after. Migrations run on Render before the new version starts (`scripts/render-migrate.sh`). Before the merge: a stage backup with a restore test, then your word, as with every stage database change.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Client route | `internal/trainerclient/handlers.go` `ClientListTrainers` | `/client/...` with per-route JWT middleware |
| Program view | `internal/trainerclient/bot_service.go:82` `GetTelegramProgram`, `clientProgramView` | link check, assignment, visible version detail |
| Mark done | `internal/telegrambot/workout.go:178-240` | match the assignment ids, find the day, `workoutcompletion.Service.Complete`, `ErrAlreadyCompleted` |
| Stats | `internal/analytics/handlers.go:61` `GetClientSelfAnalytics` | same service, other caller |
| Error codes | `internal/trainerclient/errors.go`, `.claude/rules/api-endpoints.md` "Errors" | one text, one code |
| Tests | `internal/trainerclient/handlers_client_trainers_internal_test.go`, `internal/db/storetest/workout_completion_integration_test.go` | handler tests with fakes, integration tests on `mentorix_test` |

## Files to Change

| File | Action | Why |
|---|---|---|
| `db/migrations/000033_*.sql`, `db/schema.sql` | CREATE, UPDATE | widen the source CHECK |
| `internal/workoutcompletion/` | UPDATE | accept `SourceApp` |
| `internal/trainerclient/` | UPDATE | program view and mark by user id, handlers, routes |
| `internal/analytics/` | UPDATE | the token route for stats |
| `internal/app/routes.go` | UPDATE | wiring |
| tests next to each | CREATE, UPDATE | |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | three operations, request and response schemas |
| `docs/features/workout-completions.md`, `trainer-invites.md` or a new `client-app.md`, `client-analytics.md`, `docs/status.md`, `docs/architecture.md` (migration version) | UPDATE | |

## Tasks

1. Migration and `SourceApp`. Validate: migrate up and down locally, integration test that an `app` row is stored.
2. Program view by user id: refactor nothing in the bot; add the user-id path next to it and reuse the shared steps. Validate: integration tests for no link, blocked, no assignment, hidden blocks, done day keys.
3. Mark done: same checks as the bot. Validate: tests for every answer in the table, including a reassignment between read and write.
4. Stats by token. Validate: same body as the signed-link page for the same data.
5. Contract and docs. Validate: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`.

## Validation

```bash
go test ./... -count=1
go test -tags integration ./internal/db/storetest/... -count=1
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Local HTTP run before Gate 2: a client with a trainer and a program reads it, marks a day, sees the checkmark and the stats, a second mark answers `409`, a reassignment in between answers `409 program_changed`.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The migration on stage | Low | Only widens a CHECK; backup with a restore test before the merge |
| The bot changes behaviour | Low | Bot code paths untouched; its tests stay green |
| The app later needs structured results | Certain | Answers are objects; new fields are additive |
| A client marks a day of a program that the trainer just changed | Low | The assignment and version ids in the request; `409 program_changed` |
