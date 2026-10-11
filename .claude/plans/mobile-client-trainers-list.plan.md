# Plan: A client's trainers for the app

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 6. The app knows whether the client has a trainer (new, from the Flutter developer's question)
**Complexity**: Small

## Summary

After sign-in the app cannot tell whether the client already has a trainer, so on a new device it shows the invite screen again. This plan adds `GET /client/trainers`: the caller's trainers with an active link. An empty list means "ask for an invite". No migration.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Shape | A separate `GET /client/trainers`, not a field in `/auth/me` | `/auth/me` is called by the web cabinet on every load and would pay for a join it does not need. The client screens will need this list anyway |
| Access | Bearer token, no role check | Any signed-in user may ask who their trainers are. A user with no links gets an empty list |
| Answer | `{ "items": [ { "trainer_id", "trainer_display_name", "linked_at" } ] }`, newest link first | Same names as the answer of `POST /client/invites/accept`. `trainer_id` is `trainers.id`, the id that endpoint returns |
| Which links | `active` only | A blocked client has no working trainer. The bot lists trainers the same way (`db/queries/telegram_client.sql:1`) |
| Paging | None | A client has a handful of trainers |
| Program data | Not included | It belongs to the client screens plan. Items are objects, so fields can be added later without breaking the app |
| Unknown user | An access token of a user that no longer exists gets an empty list | One query, no extra lookup. The app learns that the account is gone from `GET /auth/me`, which answers `404` |

Out of scope: the active trainer choice the bot keeps in Redis, program and workout data, avatars, paging, any change to `/auth/me`.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Query | `db/queries/telegram_client.sql:1` | Trainers of a client through `trainer_clients`, `trainers`, `users`, `status = 'active'` |
| Store | `internal/trainerclient/bot_store.go:31` | `ListTelegramTrainers`: rows to a slice that is never nil |
| Handler | `internal/trainerclient/handlers.go:62` | `ClientAcceptInvite`: user id from the token, per-route JWT middleware |
| Mount | `internal/trainerclient/handlers.go:57` | `/client/...` routes get per-route middleware, never a group |
| Contract | `api/openapi.yaml` `clientAcceptInvite` | Tags, `bearerAuth`, shared `Unauthorized` |
| Tests | `internal/db/storetest/` | Integration tests against `mentorix_test`; handler tests with a fake service |

## Files to Change

| File | Action | Why |
|---|---|---|
| `db/queries/trainer_invite.sql` | UPDATE | Query: active trainers of one client user |
| `internal/db/sqlc/` | REGENERATE | `sqlc generate` |
| `internal/trainerclient/model.go` | UPDATE | `ClientTrainer`, `ClientTrainerList` |
| `internal/trainerclient/store.go`, `service.go`, `handlers.go` | UPDATE | Store method, service method, handler and route |
| `internal/trainerclient/*_test.go`, `internal/db/storetest/*` | UPDATE | Handler and integration tests |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | Path, two schemas and their Go bindings |
| `docs/features/trainer-invites.md`, `docs/status.md` | UPDATE | Behaviour next to the invite accept paragraph, status row |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | Milestone 6 row |

## Tasks

### Task 1: Query and store

- **Action**: a query that returns `trainer_id`, the trainer's display name and the link's `created_at` for one `client_user_id`, active links only, newest first. Store method that maps it to `[]ClientTrainer`.
- **Mirror**: `ListClientTrainersByTelegramUser` and `ListTelegramTrainers`.
- **Validate**: integration tests written first: no links gives an empty slice; two trainers come newest first; a blocked link is not listed; another client's links are not visible; a user that does not exist gives an empty slice.

### Task 2: Handler and route

- **Action**: `GET /client/trainers` with per-route JWT middleware; answer `ClientTrainerList` with `items` always an array.
- **Mirror**: `ClientAcceptInvite`.
- **Validate**: handler tests first: `200` with items, `200` with `[]` (not `null`), `401` without a token, `500` when the store fails. An unknown `/client/...` path still answers `404` and `GET /client/analytics` stays public.

### Task 3: Contract and docs

- **Action**: OpenAPI path `clientListTrainers` and schemas `ClientTrainer`, `ClientTrainerListResponse`; bindings in `internal/apicheck/schema.go`; a short paragraph in `docs/features/trainer-invites.md`, next to the one about accepting an invite in the app; the status row.
- **Validate**: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`.

### Task 4: Proof before Gate 2

- **Action**: full `make check-ci` without agent variables; a local HTTP run: a prepared app account with no trainer gets `[]`, accepts a local trainer's invite, then sees that trainer; a second account does not see it.

### Task 5: After the merge

- **Action**: confirm on stage that the endpoint answers `401` without a token (no app session is available to me there), check the cabinet still reads its data, and send the Flutter developer the format.

## Validation

```bash
go test ./internal/trainerclient/... ./internal/apicheck/...
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The endpoint shows someone else's trainers | Low | The client id comes only from the access token; an integration test covers a second client |
| The app reads an empty list from a deleted account as "no trainer" | Low | Documented: `GET /auth/me` answers `404` for that token, and the app replaces both tokens after a Telegram link |
| The query is slow | Low | `trainer_clients_client_user_id_idx` exists; the database reviewer checks the plan |

## Acceptance

- [ ] All tasks complete
- [ ] Validation passes
- [ ] Patterns mirrored, not reinvented
- [ ] `/auth/me`, the bot and the web cabinet unchanged
