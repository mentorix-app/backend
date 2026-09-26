# Documentation and rules maintenance

Mirror: [`.claude/rules/docs-and-rules-maintenance.md`](../.claude/rules/docs-and-rules-maintenance.md).

## Where things live

| Kind | Location |
| --- | --- |
| Patterns (how) | `.claude/rules/*.md` (loaded on demand) |
| Always-on context + rule routing | `CLAUDE.md` |
| Repeatable workflow | `.claude/skills/<name>/SKILL.md` |
| Implemented features | `docs/status.md` |
| Feature facts | `docs/features/<name>.md` |
| API contract | `api/openapi.yaml` |

## New feature

1. Add `docs/features/<name>.md` + link in [README.md](README.md).
2. When done, add a row to [status.md](status.md).
3. New pattern goes in the right rule file, not in docs.
4. A new rule file is invisible until it gets a row in `CLAUDE.md`.

## Limits

- Rule file: ~80–120 lines; skill: ~120; feature doc: ~150; `CLAUDE.md`: ~120.
- Do not copy OpenAPI and naming rules into docs.

## Migrations

When adding `db/migrations/*.up.sql`, update the version number in [architecture.md](architecture.md) and [status.md](status.md) (same number). Verified by `./scripts/migrate-check.sh` (database) and `./scripts/docs-check.sh` (docs).

## Before commit

1. Check diff against docs/rules/status per [CLAUDE.md](../CLAUDE.md) § Docs sync.
2. Manually: `./scripts/docs-check.sh` or `make check-ci`. Pre-commit (`make check-quick`) does not run docs-check — that runs in CI.

## Before push

`make check` (full run: also migrate-check + smoke). GitHub CI equals `make check-ci`.

Release / Render: [environments.md](environments.md).

## status.md

- Only implemented facts with code (+ OpenAPI for API).
- Sections "In progress" / "Planned" only when real work begins.
