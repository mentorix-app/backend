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

## Local setup

```bash
docker compose up -d
cp .env.example .env
# Edit JWT_SECRET if needed (min 32 chars)

./scripts/migrate.sh up   # macOS/Linux
# or: .\scripts\migrate.ps1 up

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

Validate collection:

```bash
./postman/validate.sh
```

API contract: [`api/openapi.yaml`](api/openapi.yaml) (OpenAPI 3).

## Migrations

```bash
./scripts/migrate.sh up
./scripts/migrate.sh down 1
./scripts/migrate.sh version
./scripts/migrate-check.sh   # verify DB matches latest migration in repo
```

After changing Compose Postgres credentials, run `docker compose down -v` and update `DATABASE_URL` in `.env`.

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
cmd/api/           HTTP entrypoint
internal/auth/     Authentication
internal/exercise/ Exercises API
internal/config/   Env configuration
internal/health/   Health probes
db/migrations/     SQL migrations
postman/           API collection + validation
api/               OpenAPI 3 specification
scripts/           migrate and migrate-check wrappers
```
