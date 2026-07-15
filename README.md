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

Local + Render stage/prod (CI/CD): [docs/environments.md](docs/environments.md).

## QA

- **Pre-commit:** `make install-hooks` (once; in `make setup`) — `make check-ci` on every commit. Override: `PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit …`.
- **Before commit:** sync docs/rules — [.cursor/rules/qa-before-push.mdc](.cursor/rules/qa-before-push.mdc) §A (`docs-check` in check-ci).
- **Before push:** `make check` (full: migrate-check + smoke).

Details: [docs/README.md#qa](docs/README.md#qa).

## Documentation

**[docs/README.md](docs/README.md)** — product, architecture, features, status.

API contract: [api/openapi.yaml](api/openapi.yaml).
