# Mentorix Backend

Go REST API for Mentorix (trainers, clients, exercises). Echo + PostgreSQL + Redis.

## Prerequisites

- Go 1.25+
- Docker Desktop (Postgres + Redis locally)

Install dev CLI tools once (migrate, sqlc, golangci-lint, air):

```bash
make install-tools
```

## Local setup

```bash
make install-tools   # once: migrate, sqlc, lint, air
make setup           # docker compose, .env, migrations
make dev             # API with hot reload (or: make run)
```

`make setup` creates `.env` from `.env.example` if missing. Edit `JWT_SECRET` if needed (min 32 chars).

List all commands: `make help`

Check:

- `GET http://localhost:8080/health` → `{"status":"ok"}`
- `GET http://localhost:8080/health/ready` → database/redis `ok`

## Postman

Import from `postman/`:

- `mentorix-backend.postman_collection.json`
- `mentorix-local.postman_environment.json` — local API (`password123` for `trainer@test.com` is in the file)
- `mentorix-render-dev.postman_environment.json` — Render dev URL; **`user_password` is empty in git**

For **Render dev**, after import set in Postman → Environments → **Mentorix Render Dev**:

- `user_email` — your account on that deployment (default in file: `dev-trainer@test.com`)
- `user_password` — your password (**not** stored in the repository; set once in Postman on your machine)

Do not re-export the Render environment into git with a filled password.

Select environment, then: Login → Me → List exercises.

Validate collection (routes, OpenAPI paths, JSON schemas vs Go types, Postman bodies):

```bash
make validate
```

This runs `go test ./internal/apicheck/...` — Go routes come from Echo `Mount()`, not a hand-maintained list.

API contract: [`api/openapi.yaml`](api/openapi.yaml) (OpenAPI 3).

## Migrations

```bash
make migrate
make migrate-down          # default N=1; e.g. make migrate-down N=2
make migrate-version
make migrate-check         # verify DB matches latest migration in repo
```

After changing Compose Postgres credentials, run `docker compose down -v` and update `DATABASE_URL` in `.env`.

## sqlc

SQL queries live in `db/queries/`. Generated Go code: `internal/db/sqlc/`.

After changing migrations or queries:

```bash
make schema-sync    # refresh db/schema.sql from migrations
make generate       # runs sqlc (config: sqlc.yaml in repo root)
```

## Store integration tests

Requires Docker (Testcontainers). Not run in default `go test ./...`.

```bash
make test-integration
```

## Local QA (all checks)

`make check` runs, in order: gofmt, go vet, `go test ./...`, build, sqlc/go-generate drift, golangci-lint, contract validation (`postman/validate.sh` + `internal/apicheck`), migrate-check, store integration tests, and API smoke on `http://localhost:8080`.

**Full suite** (Docker + `.env` + API running):

```bash
make check
```

**CI parity** (no migrate-check, Testcontainers, or smoke):

```bash
make check-ci
```

**Partial** (combine as needed):

```bash
make check CHECK_FLAGS="--no-smoke"              # Docker OK, API not running
make check CHECK_FLAGS="--no-integration"      # skip Testcontainers
make check CHECK_FLAGS="--no-migrate-check"    # skip DB version check
```

See `scripts/check.sh --help` for all flags.

## Render dev

Deploy from `develop` branch. Run migrations against External `DATABASE_URL` from your machine.

Set the same env keys as in `.env.example` (cloud block), plus:

```env
TRUSTED_PROXY_CIDRS=private
```

Use this behind Render/nginx so rate limiting uses the real client IP (not spoofable `X-Forwarded-For`).

For a frontend on another domain, also set:

```env
REFRESH_COOKIE_SAMESITE=none
REFRESH_COOKIE_SECURE=true
CORS_ALLOW_ORIGINS=https://your-frontend.example.com
```

## Project layout

```text
cmd/api/              HTTP entrypoint
internal/auth/        Authentication (+ apitypes.go — shared REST DTOs)
internal/exercise/    Exercises API
internal/program/     Training programs API
internal/apicheck/      Contract tests (Go ↔ OpenAPI ↔ Postman)
internal/config/      Env configuration
internal/health/      Health probes
internal/db/
  sqlc/               sqlc generated code
  pgconv/             pgx ↔ domain type helpers
  storetest/          store integration tests (tag: integration)
db/migrations/        SQL migrations
db/queries/           sqlc query definitions
db/schema.sql         schema snapshot for sqlc (generated)
postman/              API collection + validate.sh
api/                  OpenAPI 3 specification
Makefile              make help — dev commands (wraps scripts/)
.air.toml             Air hot reload config (make dev)
scripts/              migrate, migrate-check, schema-sync, check
sqlc.yaml             sqlc config
```
