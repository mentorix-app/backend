# Architecture

## Model

- **Go monolith:** single deployment with REST (Echo v4), PostgreSQL, and Redis.
- Auth built in Go and Postgres (no Supabase).

## Tech stack

| Area | Choice |
| ---- | ------ |
| HTTP | Echo v4 |
| Database | PostgreSQL, schema `mentorix`, pgx + sqlc, golang-migrate |
| Cache/rate limits | Redis |
| Auth | JWT + opaque refresh (cookie), argon2id |
| Deployment | Render Blueprint ([`render.yaml`](../render.yaml)): stage from `develop`; CI-gated Deploy Hook |
| Logs | `log/slog`, request ID |

Migrations: version **26**, checked by `./scripts/migrate-check.sh`.

## Layout

```text
cmd/api/           — REST + Telegram webhook + push
internal/<feature>/ — domain
internal/db/sqlc/  — generated
db/migrations/     — SQL
db/queries/        — sqlc
api/openapi.yaml   — REST contract
```

**Telegram SDK:** `github.com/go-telegram-bot-api/telegram-bot-api/v5` is the de facto Go standard; alternatives are `telebot` (higher level) or raw HTTP (too low level).
