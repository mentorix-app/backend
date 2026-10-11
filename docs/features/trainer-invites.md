# Trainer invites

**Status:** implemented  
**Code:** `internal/trainerclient/`

## Purpose

Trainers invite clients via Telegram deep links; the link creates a `trainer_clients` relationship when accepted in the bot.

## Database

Uses `trainer_invites`, `trainer_clients`, `auth_identities` (with `provider = telegram`), and `user_roles` (with `client`). No new migrations needed; tables are from init and migration `000007`.

## API

Contract: `api/openapi.yaml` under the `trainer` tag.

Non-obvious rules:

- `POST /trainer/invites` requires `TELEGRAM_BOT_USERNAME` (without it: `503 invite_not_configured`); the link is `https://t.me/<bot>?start=inv_<token>`.
- Invites are not created when the plan's client limit is full (returns `409` with code `quota_exceeded`). Accept rechecks the quota under a lock; if it fails, the invite is **not** consumed and the bot responds «нет свободных мест». A trainer cannot accept their own invite. See [subscriptions.md](subscriptions.md).
- Invite TTL is `TRAINER_INVITE_TTL_DAYS` (default 7 days). When creating a new invite, old expired or consumed invites (with a 7-day grace period) are purged in-line.
- Accept in the bot (`/start inv_<token>`) calls `trainerclient.Service.AcceptInvite` in-process, not via HTTP. The Telegram profile photo is saved if present.
- Accept creates a `user` and `auth_identity(telegram)` with the `client` role if the Telegram id is new; otherwise it links to the existing user. Two concurrent first accepts for the same Telegram id both succeed and end up with one user.
- Accepting the same invite again with the same user is idempotent and returns ok with `already_linked: true` if the link already existed.
- If the invite is consumed by another user, accept returns `409 invite_consumed`; if expired, `410 invite_expired`.
- `GET /trainer/clients` returns a paginated list with optional active `program_assignment`, supports search by name (`q`), and sorting by `name` or `linked_at`. Admins see all linked clients deduplicated by `client_user_id`, prioritizing their own relationship; see [trainer-clients.md](trainer-clients.md).
- The app accepts the same invite with `POST /client/invites/accept`. `token` may be the bare token, `inv_<token>`, or the whole invite link. Any signed-in user except an admin may call it (an admin gets `403`); the `client` role is added when missing, and a trainer may accept another trainer's invite. The refusals are the bot's, each with its own `code`: unknown invite `404 invite_not_found`, account gone `404 user_not_found`, admin caller `403 admin_cannot_accept`, used by someone else `409 invite_consumed`, no free places `409 client_limit_reached`, expired `410 invite_expired`, own invite `422 self_invite`, blocked by the trainer `422 client_blocked`. The two `409` answers differ by `code`; a client branches on `code`, not on `message`. The app path does not set the bot's active trainer or sync the Telegram avatar.
- The app asks who its trainers are with `GET /client/trainers`. `trainer_id` is the id the accept call returns. Only links with `active` status are listed, newest first, so a blocked client is not shown the trainer. Any signed-in user may call it, and the user comes only from the access token. An empty list means the caller has no active trainer link (a client without a trainer, or one whose only trainer blocked them), and the app then shows the invite screen. A token of a user that no longer exists gets an empty list too, while `GET /auth/me` answers `404` for it.
- Program assignment only works after accept; see [trainer-clients.md](trainer-clients.md).

## Phases (Telegram client)

| Phase | Status |
| ---- | ------ |
| 1. Invites + clients list + accept | done |
| 2. Minimal Telegram bot | done |
| 3. Program in bot (read-only) | done |
| 4. Push on assignment / sync | done |

## See also

- [telegram-bot.md](telegram-bot.md) — client bot
- [trainer-clients.md](trainer-clients.md) — program assignment
- [product.md](../product.md) — client via Telegram
