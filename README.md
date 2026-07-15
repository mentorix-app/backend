# Mentorix Backend

Go REST API (Echo + PostgreSQL + Redis).

## Quick start

**Prerequisites:** Go 1.25+, Postgres 16, Redis (e.g. `brew install postgresql@16 redis`).

```bash
make install-tools   # once
make setup           # .env, migrations, pre-commit hook
make dev             # API with hot reload
```

`make help` — all commands.

**Check:** `GET http://localhost:8080/health` → `{"status":"ok"}`

## Postman

Import `postman/mentorix-backend.postman_collection.json` + `mentorix-local.postman_environment.json`.

Validate contract: `make validate`

Local + Render stage (Blueprint): [docs/environments.md](docs/environments.md).

## QA

- **Pre-commit:** `make install-hooks` (once; included in `make setup`) — hook runs `make check` on every commit. Needs `.env`, Postgres (`TEST_DATABASE_URL` for integration), API on `:8080` for smoke; or `PRE_COMMIT_CHECK_FLAGS=--no-smoke`.
- **Before commit:** sync docs/rules — see [.cursor/rules/qa-before-push.mdc](.cursor/rules/qa-before-push.mdc) §A; `make docs-check`.
- **Before push:** `make check` (full) or `make check-ci` (CI parity).

Details: [docs/README.md#qa](docs/README.md#qa).

## Documentation

**[docs/README.md](docs/README.md)** — product, architecture, features, status.

API contract: [api/openapi.yaml](api/openapi.yaml).
