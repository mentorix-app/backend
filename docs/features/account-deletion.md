# Account deletion

**Status:** implemented  
**Code:** `internal/auth/account_deletion.go`, `DeleteMe` in `internal/auth/handlers.go`, wiring in `internal/app/routes.go`

## Purpose

A client deletes their own account from the mobile app, as the App Store requires, and every row that belongs to them goes with it. Nothing is kept: no soft delete, no grace period, no anonymous copy.

## Database

No table of its own and no migration. One transaction deletes the rows of a single client, in this order:

1. `program_assignments`, then
2. `client_workout_completions`, which cascade `client_workout_completion_comments` (the trainer's replies). The order is the one a trainer's removal of an assignment takes (the foreign key action clears the completions' link), so the two transactions never lock the rows the other way round.
3. `trainer_invites` the client consumed. A consumed invite cannot lose its `consumed_by` (`trainer_invites_consumed_check` needs both columns set or both null), so the foreign key's `SET NULL` would be refused. Consumed invites are purged after 7 days anyway.
4. `users`, which cascade `auth_identities`, `user_roles`, `auth_refresh_sessions`, `user_link_codes`, `trainer_clients` and `program_block_clients`.

The trainer's programs, versions and exercises, and every other client, are not touched. Deleting an account does not run the program version cleanup, see [garbage-cleanup.md](garbage-cleanup.md).

## API

Contract: `api/openapi.yaml` (`DELETE /auth/me`, `deleteMe`).

Key details:

- Bearer JWT, no request body (none is read), no cookie set or cleared, no rate limit. Success is `204` with no body.
- **Who.** Only an account whose roles are exactly `client`. A `trainer` or `admin` role, alone or next to `client`, and an account with no role row are refused. The transaction locks the user row first (`FOR UPDATE`) and decides every refusal under that lock, so a role or an Apple sign-in added a moment earlier is seen. A trainer who assigns a program to the client at the same moment either commits first, and the assignment is deleted with the rest, or fails because the user is gone.
- **Refusal.** `409 account_deletion_unavailable` (`this account cannot be deleted in the app`), also when the database refuses the delete because other rows still reference the user, for example content the account authored. Nothing is changed.
- **Answers.** `204` deleted; `401` no token or a bad one (`missing_authorization`, `invalid_token`); `404 user_not_found` the account is already gone, so a repeated call answers `404`; `409 account_deletion_unavailable`; `503 provider_not_configured` (`apple account deletion is not configured`); any other failure is `500` (`account deletion failed`) with the cause in the log only.
- **Apple.** An account with an `apple` sign-in answers `503` and nothing is deleted, because the Apple token revocation that Apple asks for is not configured. The check runs after the role rule, under the same lock. Accounts without an Apple sign-in are deleted as described.
- **Invites and blocks.** The invites the client used are deleted, so tapping the link of one answers `invite_not_found` instead of `invite_consumed`. A trainer's block of the client goes with the account: a person who deletes the account and later joins with a fresh invite of that trainer is a new client and is not blocked.
- **Sessions.** Refresh sessions go with the user. An access token issued earlier works until it expires (`ACCESS_TTL_MINUTES`, 15 by default) and finds nothing: `GET /auth/me` answers `404 user_not_found`. The app clears its own tokens.
- **Telegram.** The Telegram sign-in goes with the account. After the commit the bot's `active_trainer` key for that Telegram is removed (key in [redis-naming.md](../../.claude/rules/redis-naming.md)). The removal runs on its own context with a 5 second limit, so a client that drops the connection after the commit does not cancel it. A failure there is logged as a warning and the request still succeeds. `workout_pending` (30 minutes) and `avatar_checked` (24 hours) expire by themselves. For the bot the person is unknown again, and accepting an invite later creates a new empty account.
- **Coming back.** Signing in again with the same Google or Apple account creates a new empty account.
- **Log.** One info line `account deleted` with `user_id`. A cleanup failure logs a warning with the user id and the error. Nothing personal reaches the log.
- The trainer is not told that a client left. Their client list, progress and history lose the client.

## See also

- [auth.md](auth.md): sign-in, `GET /auth/me`, error codes
- [workout-completions.md](workout-completions.md): the completions that are deleted
- [trainer-invites.md](trainer-invites.md): the invites a client consumes
- [garbage-cleanup.md](garbage-cleanup.md): the Redis keys the bot keeps
