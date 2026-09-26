# Mentorix Backend

Go REST API (Echo + PostgreSQL + Redis).

## Quick start

You need Go 1.25+, Postgres 16 and Redis (e.g. `brew install postgresql@16 redis`).

```bash
make install-tools   # once
make setup           # .env, migrations, pre-commit hook
make dev             # API with hot reload
```

`make help` lists all commands.

Check: `GET http://localhost:8080/health` should return `{"status":"ok"}`.

## API contract

Canon: `api/openapi.yaml`. Checked against Go routes and types by `go test ./internal/apicheck/...` (also inside `make check-ci`).

Local + Render stage (CI/CD): [docs/environments.md](docs/environments.md). Prod later.

## QA

- Pre-commit: `make install-hooks` (once; part of `make setup`) makes every commit run `make check-quick` (gofmt, vet, unit, lint). Skip once: `git commit --no-verify`.
- Before a commit, sync docs and rules per [CLAUDE.md](CLAUDE.md) § Docs sync (`docs-check` in check-ci / CI).
- Before a push, run `make check` (full: migrate-check + smoke).

Details: [docs/README.md#qa](docs/README.md#qa).

## Documentation

[docs/README.md](docs/README.md) covers product, architecture, features and status.

Agent context: [CLAUDE.md](CLAUDE.md) (always loaded) + [.claude/rules/](.claude/rules/) (read per area).

API contract: [api/openapi.yaml](api/openapi.yaml).
