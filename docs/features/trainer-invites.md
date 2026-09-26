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

- `POST /trainer/invites` requires `TELEGRAM_BOT_USERNAME`; the link is `https://t.me/<bot>?start=inv_<token>`.
- Invites are not created when the plan's client limit is full (returns `409 quota_exceeded`). Accept rechecks the quota under a lock; if it fails, the invite is **not** consumed and the bot responds «нет свободных мест». A trainer cannot accept their own invite. See [subscriptions.md](subscriptions.md).
- Invite TTL is `TRAINER_INVITE_TTL_DAYS` (default 7 days). When creating a new invite, old expired or consumed invites (with a 7-day grace period) are purged in-line.
- Accept in the bot (`/start inv_<token>`) calls `trainerclient.Service.AcceptInvite` in-process, not via HTTP. The Telegram profile photo is saved if present.
- Accept creates a `user` and `auth_identity(telegram)` with the `client` role if the Telegram id is new; otherwise it links to the existing user.
- Accepting the same invite again with the same user is idempotent and returns ok with `already_linked: true` if the link already existed.
- If the invite is consumed by another user, accept returns `409`; if expired, `410`.
- `GET /trainer/clients` returns a paginated list with optional active `program_assignment`, supports search by name (`q`), and sorting by `name` or `linked_at`. Admins see all linked clients deduplicated by `client_user_id`, prioritizing their own relationship; see [trainer-clients.md](trainer-clients.md).
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
