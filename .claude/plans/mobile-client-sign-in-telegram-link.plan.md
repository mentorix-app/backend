# Plan: Mobile client sign-in, Telegram link

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 3. Telegram link
**Complexity**: Medium

## Summary

A client links Telegram from the app: the app opens the bot, the bot answers with a one-time code, the client types the code into the app. The backend then attaches that Telegram to the app account, or, when the Telegram already has an account, moves the Google or Apple sign-in to it and removes the empty app account. The answer carries a fresh session for the account that remains. No migration.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Direction of the code | The bot issues the code, the client enters it in the app | The secret travels from the Telegram owner to the app. The opposite direction (the app issues a link, one tap in the bot confirms) lets an attacker send their own link to a victim: one tap would attach the attacker's sign-in to the victim's account |
| Where the code lives | Redis, `mentorix:telegram:link_code:{code}` holding the Telegram user id, 10 minutes, deleted on first use | Works for a Telegram user the system has never seen; no table, no migration. Same shape as the bot's pending workout |
| Code format | 8 characters from an alphabet without look-alikes | Short enough to type; about 10^12 combinations behind the existing per-IP limit |
| Which account remains | The Telegram one. Google and Apple identities move to it; its email is filled from the app account when it has none | History and trainer links already live there |
| "Empty" app account | No trainer link and no role other than `client` | Workouts and assignments cannot exist without a trainer link; the database also refuses to delete a user who has them |
| Both hold data | `409`, nothing changes, the code is spent | Owner's decision: resolve by hand |
| App account already has another Telegram | `409` | One Telegram per account |
| Session after linking | The answer always returns new tokens for the remaining account | The removed account's sessions disappear with it; the app just replaces its tokens |
| Profile | `GET /auth/me` gains `telegram_linked` | The app decides whether to ask "already training through Telegram?" |

Left out on purpose: unlinking, moving a Telegram from one account to another, merging two accounts with data, locks against a client who accepts an invite and links Telegram in the same instant (the window is milliseconds and needs the same person on both sides), the unused `user_link_codes` table.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Redis store | `internal/workoutcompletion/pending.go` | Key prefix constant, TTL, nil-safe store |
| Bot start payload | `internal/telegrambot/bot.go:157` `handleStart`, `parseInviteToken` | Branch on the `/start` argument |
| Bot dependency | `telegrambot.WithAvatarCheckStore` | Small interface injected with an option |
| Transaction | `internal/auth/store.go` `createClientWithIdentity` | One transaction through `s.q.WithTx(tx)` |
| Handler | `internal/auth/handlers.go` `Google`, `Me` | Limiter, bind, sentinel errors mapped to statuses, body answer |
| Wiring | `internal/app/routes.go` `Result` | Shared objects handed to `cmd/api` for the bot |

## Files to Change

| File | Action | Why |
|---|---|---|
| `internal/auth/telegram_link.go`, test | CREATE | Redis code store: issue, take (read and delete) |
| `db/queries/auth.sql`, `internal/db/sqlc/` | UPDATE | queries: move identities, fill empty email, delete user, count trainer links, find a user's Telegram |
| `internal/auth/store.go` | UPDATE | `LinkTelegram` in one transaction |
| `internal/auth/service.go`, `handlers.go`, `apitypes.go`, `const.go` | UPDATE | `POST /auth/telegram/link`, `telegram_linked` in the profile, errors |
| `internal/telegrambot/bot.go` | UPDATE | `/start link` answers with the code |
| `internal/app/routes.go`, `cmd/api/main.go` | UPDATE | pass the code store to the bot |
| `internal/db/storetest/` | CREATE | integration tests for the three outcomes and the refusal |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | new path, body, profile field |
| `docs/features/auth.md`, `docs/features/telegram-bot.md`, `docs/status.md`, `.claude/rules/redis-naming.md` | UPDATE | behaviour, new key |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | milestone statuses |

## Tasks

### Task 1: Code store
- **Action**: issue a random code for a Telegram user id; take it once. Redis missing: issue and take report "unavailable".
- **Validate**: `go test ./internal/auth/ -run LinkCode`

### Task 2: Link in the database
- **Action**: `LinkTelegram(appUserID, telegramUserID)` returns the remaining user id. Outcomes: attached; already linked to the same Telegram (no change); moved to the Telegram account and the app account removed; refused (data on both sides, or another Telegram already linked).
- **Validate**: integration tests, including: identities moved, sessions of the removed account gone, email filled only when empty, nothing changed on refusal.

### Task 3: Endpoint and profile
- **Action**: `POST /auth/telegram/link` with a Bearer token and `{code}`: `200` with tokens in the body, `400` empty, unknown or expired code, `401`, `409`, `429`, `503` when codes are unavailable. `telegram_linked` in `GET /auth/me`.
- **Validate**: `go test ./internal/auth/... ./internal/apicheck/...`

### Task 4: Bot
- **Action**: `/start link` sends the code with a "do not share it" line, in Russian like the other bot texts. Invite handling and every other command stay as they are.
- **Validate**: `go test ./internal/telegrambot/...`

### Task 5: Docs, PRD, Flutter note

## Validation

```bash
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Plus a local run. Sign-in needs a Google token, which cannot be made locally, so the local run uses rows prepared in the local database for the app account and covers: all outcomes over HTTP with a code placed in local Redis, and the profile field. The bot part is covered by its tests; a live check on stage needs the owner to press Start in the stage bot (it only produces a code and changes nothing).

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| A wrong account is removed | Low | Removal only when the app account has no trainer link and only the `client` role; the database refuses to delete users with workouts or assignments; integration tests |
| The bot stops handling invites | Low | The new branch is taken only for the exact argument `link`; existing bot tests stay |
| Someone talks a client into reading out the code | Low | The bot message says not to share it; the code lives 10 minutes and works once |
| Stage has real Telegram clients | Certain | No schema change; the only writes happen when a client completes a link themselves |

## Acceptance

- [ ] A new Telegram is attached to the app account and the bot then recognises it
- [ ] An existing Telegram client ends up in the old account with Google or Apple sign-in, and the empty app account is gone
- [ ] Data on both sides: refused, nothing changed
- [ ] Invites, the cabinet and the earlier sign-in endpoints behave as before
- [ ] `make check-ci` passes; docs and OpenAPI match the code
