# Mentorix Backend

**Monolith** Go REST (Echo) + PostgreSQL (`mentorix`) + Redis. One `user_id` for all
login channels. No microservices/gRPC without explicit request.

Prefer idiomatic Go: bounded parallelism, worker pools for independent I/O,
`context` cancellation. Details: [.claude/rules/go-code.md](.claude/rules/go-code.md).

## Before non-trivial work

1. [docs/status.md](docs/status.md) — what exists
2. [docs/features/](docs/features/)`<feature>.md` — the feature you touch
3. [.claude/rules/docs-and-rules-maintenance.md](.claude/rules/docs-and-rules-maintenance.md) — how to update docs/rules

## Rules by area

Rules are **not** auto-loaded. Read the file below before writing in that area.

| Touching | Read |
| -------- | ---- |
| `**/*.go` | [go-code.md](.claude/rules/go-code.md) |
| `db/**` — migrations, queries, schema | [database-naming.md](.claude/rules/database-naming.md) |
| Redis keys in `internal/**` | [redis-naming.md](.claude/rules/redis-naming.md) |
| `api/openapi.yaml`, handlers, `internal/apicheck`, `postman/**` | [api-endpoints.md](.claude/rules/api-endpoints.md) |
| docs, rules, `docs/status.md` | [docs-and-rules-maintenance.md](.claude/rules/docs-and-rules-maintenance.md) |

Changelog for frontend / Telegram: `/changelog` skill.

**Precedence:** project rules + docs > code/OpenAPI on conflict.

## Docs sync — before every commit (required)

No commit with code/API/DB changes without aligned docs/rules.

| If diff touches… | Update / verify |
| ---------------- | --------------- |
| `internal/<feature>/`, handlers, service | `docs/features/<feature>.md` |
| `api/openapi.yaml`, Postman, apicheck | same feature doc; remove stale endpoints from docs |
| `db/migrations/`, `db/queries/` | feature doc tables; new capability → `docs/status.md` |
| New naming/process pattern | matching `.claude/rules/*.md` |
| Docs/rules only | no conflict with code, OpenAPI, `db/schema.sql` |

Docs gate: `./scripts/docs-check.sh` (included in `make check-ci`).

**Actuality:** docs describe **now** only; delete stale text in the same commit.
Canon: code + OpenAPI + schema.

**status.md:** each row = live feature; new done capability → add row if missing.

Docs-only commits: still check links, duplicates, contradictions.

**Pre-commit hook:** after `make install-hooks` (included in `make setup`), each
`git commit` runs `make check-ci`. Override:
`PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit …`.

## Technical QA — before push

Full run: `make check`. CI / pre-commit: `make check-ci`.

| Step | In `check-ci` | Full `check` only |
| ---- | --- | --- |
| gofmt, vet, unit, build | yes | yes |
| sqlc drift | yes | yes |
| golangci-lint | yes | yes |
| contract (`SKIP_SMOKE=1`) | yes | via validate; smoke separate |
| docs-check | yes | yes |
| integration + coverage 85% | yes | yes |
| migrate-check | no | yes |
| live smoke (`scripts/smoke.sh`) | no | yes (API up) |

After migrations: `make schema-sync`. See [scripts/check.sh](scripts/check.sh).

Deploy / Render: [docs/environments.md](docs/environments.md).

## Git

Branches: `feature/`, `bugfix/`, `hotfix/`, `chore/`, `docs/`, `refactor/`, `test/`
+ kebab-case. Commits: Conventional Commits (`feat`, `fix`, …). `feat` only for
user-facing behavior.

Compare 2–3 library options before adding deps; record significant choices in
[docs/architecture.md](docs/architecture.md).
