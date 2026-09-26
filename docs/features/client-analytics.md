# Client analytics page

**Status:** implemented  
**Code:** `internal/analytics/client_link.go`, `internal/analytics/` (`ClientSelfAnalytics`), `internal/telegrambot/` («📊 Статистика» button)

## Purpose

A client opens their analytics for the active trainer from the Telegram bot in a browser. The bot provides a signed link; the frontend forwards the query string to `GET /client/analytics` and renders the response. Clients have no web login; the link itself is a 30-minute pass.

## Security

- Link format: `<CLIENT_ANALYTICS_PAGE_URL>?client_user_id&trainer_id&exp&sig`. The `sig` is HMAC-SHA256 over `JWT_SECRET` with domain `client_analytics:` (separate from avatar links). `client_user_id` and `trainer_id` cannot be forged or swapped.
- TTL is 30 minutes; individual links cannot be revoked. While the link is valid, it can be shared — accepted by design because the page is read-only.
- The `trainer_clients` relationship and `blocked` status are checked on every request, not when the link is issued.
- The bot hides the link in an inline URL button in its message.

## API

Contract: `api/openapi.yaml` defines `GET /client/analytics`. No JWT required. Returns `400` if parameters are missing or unparseable, `401` if signature or expiry fails, `404` if client is not linked to the trainer or is blocked.

Response includes `client` (id, name), `trainer` (`trainers.id`, name), `current_assignment` and `activity` (same schemas as trainer analytics; see [trainer-analytics.md](trainer-analytics.md)), `recent_completions` (last 30 workouts with trainer replies), and `expires_at` (link expiry time).

## Bot

The «Статистика» button appears only when `CLIENT_ANALYTICS_PAGE_URL` is set. The trainer used is the active one (same rules as «Программа»); if no active trainer is set, the bot says «Выберите тренера в разделе «Тренеры»». Full menu details are in [telegram-bot.md](telegram-bot.md).

## Env

| Variable | Required | Purpose |
| ---------- | ----------- | ---------- |
| `CLIENT_ANALYTICS_PAGE_URL` | no | Absolute `http(s)` URL to frontend page without query string; empty disables the button |

The page origin must be in `CORS_ALLOW_ORIGINS`.

## See also

- [trainer-analytics.md](trainer-analytics.md) — same calculations from trainer perspective
- [telegram-bot.md](telegram-bot.md) — bot menu
