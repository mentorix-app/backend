# Mentorix Backend

Go REST API for Mentorix (trainers, clients, exercises). Echo + PostgreSQL + Redis.

## Prerequisites

- Go 1.23+
- Docker Desktop (Postgres + Redis locally)
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI

Install migrate (once):

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.2
```

Install sqlc (once):

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0
```

Optional — golangci-lint (used by `./scripts/check.sh`; otherwise installed via `go run`):

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2
```

## Local setup

```bash
docker compose up -d
cp .env.example .env
# Edit JWT_SECRET if needed (min 32 chars)

./scripts/migrate.sh up

go run ./cmd/api
```

Check:

- `GET http://localhost:8080/health` → `{"status":"ok"}`
- `GET http://localhost:8080/health/ready` → database/redis `ok`

## Postman

Import from `postman/`:

- `mentorix-backend.postman_collection.json`
- `mentorix-local.postman_environment.json` (or Render dev env)

Select environment, then: Login → Me → List exercises.

Validate collection (routes, OpenAPI paths, JSON schemas vs Go types, Postman bodies):

```bash
./postman/validate.sh
```

This runs `go test ./internal/apicheck/...` — Go routes come from Echo `Mount()`, not a hand-maintained list.

API contract: [`api/openapi.yaml`](api/openapi.yaml) (OpenAPI 3).

## Migrations

```bash
./scripts/migrate.sh up
./scripts/migrate.sh down 1
./scripts/migrate.sh version
./scripts/migrate-check.sh   # verify DB matches latest migration in repo
```

After changing Compose Postgres credentials, run `docker compose down -v` and update `DATABASE_URL` in `.env`.

## sqlc

SQL queries live in `db/queries/`. Generated Go code: `internal/db/sqlc/`.

After changing migrations or queries:

```bash
./scripts/schema-sync.sh          # refresh db/schema.sql from migrations
go generate ./internal/db/...     # runs sqlc (config: sqlc.yaml in repo root)
```

## Store integration tests

Requires Docker (Testcontainers). Not run in default `go test ./...`.

```bash
go test -tags integration -timeout 5m ./internal/db/storetest/...
```

## Local QA (all checks)

`./scripts/check.sh` runs, in order: gofmt, go vet, `go test ./...`, build, sqlc/go-generate drift, golangci-lint, contract validation (`postman/validate.sh` + `internal/apicheck`), migrate-check, store integration tests, and API smoke on `http://localhost:8080`.

**Full suite** (Docker + `.env` + API running):

```bash
./scripts/check.sh
```

**CI parity** (no migrate-check, Testcontainers, or smoke):

```bash
./scripts/check.sh --ci
```

**Partial** (combine as needed):

```bash
./scripts/check.sh --no-smoke              # Docker OK, API not running
./scripts/check.sh --no-integration        # skip Testcontainers
./scripts/check.sh --no-migrate-check      # skip DB version check
```

See `./scripts/check.sh --help` for all flags.

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
scripts/              migrate, migrate-check, schema-sync, check
sqlc.yaml             sqlc config
```
