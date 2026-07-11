# Mentorix Backend

Go REST API (Echo + PostgreSQL + Redis).

## Quick start

**Prerequisites:** Go 1.25+, Docker Desktop.

```bash
make install-tools   # once
make setup           # docker, .env, migrations
make dev             # API with hot reload
```

`make help` — all commands.

**Check:** `GET http://localhost:8080/health` → `{"status":"ok"}`

## Postman

Import `postman/mentorix-backend.postman_collection.json` + `mentorix-local.postman_environment.json`.

Validate contract: `make validate`

Render dev: [docs/environments.md](docs/environments.md).

## QA

- **Before commit:** sync docs/rules — see [.cursor/rules/qa-before-push.mdc](.cursor/rules/qa-before-push.mdc) §A.
- **Before push:** `make check` (full) or `make check-ci` (CI parity).

Details: [docs/README.md#qa](docs/README.md#qa).

## Documentation

**[docs/README.md](docs/README.md)** — product, architecture, features, status.

API contract: [api/openapi.yaml](api/openapi.yaml).
