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
| Auth | JWT + opaque refresh (cookie or JSON body); Google and Apple ID tokens checked with go-oidc |
| Deployment | Render Blueprint ([`render.yaml`](../render.yaml)): stage from `develop`; CI-gated Deploy Hook |
| Logs | `log/slog`, request ID |

Migrations: version **36**, checked by `./scripts/migrate-check.sh`.

## Layout

```text
cmd/api/           — REST + Telegram webhook + push
cmd/janitor/       — one-off cleanup of stale rows
cmd/refresh-fingerprints/ — pre-deploy: rewrites stored program version fingerprints
internal/app/      — mounts all OpenAPI routes; shared by cmd/api and the contract test
internal/<feature>/ — domain
internal/db/sqlc/  — generated
db/migrations/     — SQL
db/queries/        — sqlc
api/openapi.yaml   — REST contract
```

**Telegram SDK:** `github.com/go-telegram-bot-api/telegram-bot-api/v5` is the de facto Go standard; alternatives are `telebot` (higher level) or raw HTTP (too low level).

**ID token verification:** `github.com/coreos/go-oidc/v3` checks signature, issuer and expiry of provider ID tokens, and the code matches the audience against `GOOGLE_CLIENT_IDS`. Alternatives were `google.golang.org/api/idtoken` (Google only, large dependency tree) and `golang-jwt` with a hand-written key fetch (more own code). go-oidc also fits Apple sign-in.
