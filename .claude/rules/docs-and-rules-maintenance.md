# Docs and rules maintenance

Human mirror: [docs/maintenance.md](../../docs/maintenance.md).

## Where content lives

| Kind | Location |
| ---- | -------- |
| Patterns (how) | `.claude/rules/*.md` |
| Always-on context + rule routing | `CLAUDE.md` |
| Implemented (what) | `docs/status.md` |
| Feature facts | `docs/features/<name>.md` |
| API contract | `api/openapi.yaml` |

**Do not** duplicate OpenAPI or naming rules in docs.

## New feature

1. Add `docs/features/<name>.md`; link in `docs/README.md`.
2. When **done** — one row in `docs/status.md`.
3. New naming/process pattern → update the matching rule (one line or new narrow file).

## New agent rule

| Topic | File |
| ----- | ---- |
| DB | `database-naming.md` |
| Redis | `redis-naming.md` |
| Go | `go-code.md` |
| HTTP/API | `api-endpoints.md` |
| QA/CI, git, always-on | `CLAUDE.md` |
| Repeatable workflow / on-demand output | new `.claude/skills/<name>/SKILL.md` |
| Other | new `.claude/rules/<topic>.md` |

Rules load **on demand**, not automatically: a new file is invisible until
`CLAUDE.md` routes to it. Adding `.claude/rules/<topic>.md` means adding its row
to the routing table in `CLAUDE.md` in the same commit.

Put a rule in `CLAUDE.md` itself only when it applies to every task regardless of
which files are touched — it costs context on each request.

## status.md

- Rows only for code that exists (+ OpenAPI for API).
- No future plans until work starts; then add `## В работе` or `## Запланировано`.
- One line per item + link; no specs (use feature docs).

## Migrations

After new `db/migrations/*.up.sql`, update version in `docs/architecture.md` and `docs/status.md`. Run `./scripts/docs-check.sh`.

## Size limits

Rule ~80–120 lines; skill ~120; feature doc ~150; `CLAUDE.md` ~120. Split or trim
when exceeded. Enforced by `./scripts/docs-check.sh`.

## Forbidden

- One mega-file (old AGENTS.md pattern).
- Copy-paste between docs and rules.
- Stale rows in status or docs contradicting code/OpenAPI/schema.

## On every commit

1. Sync docs/rules/status with the diff — see `CLAUDE.md` § Docs sync.
2. Run `./scripts/docs-check.sh` (or `make check-ci`).
