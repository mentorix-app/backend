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
| Auth | JWT + opaque refresh (cookie or body), argon2id, Apple/Google ID tokens via go-oidc |
| Deployment | Render Blueprint ([`render.yaml`](../render.yaml)): stage from `develop`; CI-gated Deploy Hook |
| Logs | `log/slog`, request ID |

Migrations: version **27**, checked by `./scripts/migrate-check.sh`.

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

**ID token verification:** `github.com/coreos/go-oidc/v3` for Apple and Google sign-in. It fetches signing keys on first use and caches them, so the API starts when a provider is down, and one verifier type covers both providers. Alternatives: `MicahParks/keyfunc` with `golang-jwt` (fetches keys at startup, runs a background refresh goroutine, issuer and audience checks written by hand) and `google.golang.org/api/idtoken` (Google only).
