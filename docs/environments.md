# Окружения

1. **Local** — Postgres + Redis, `make run` / `make dev`, `.env` из [`.env.example`](../.env.example).
2. **Stage** — `develop` → CI → Deploy Hook → `mentorix-api-stage`.
3. **Prod** — `main` → CI → approval (`production`) → Deploy Hook → `mentorix-api`.

Секреты не коммитить. Blueprint: [`render.yaml`](../render.yaml).

## Имена

| | Stage | Prod |
| --- | --- | --- |
| Web | `mentorix-api-stage` (`https://mentorix-api-stage.onrender.com`) | `mentorix-api` (`https://mentorix-api.onrender.com`) |
| Postgres / Redis | `mentorix-db-stage` / `mentorix-redis-stage` | `mentorix-db` / `mentorix-redis` |

Plans: web **starter**, Postgres **basic-256mb** (PG 16), Redis **starter**, region **frankfurt**.

## Release flow

```text
feature/* → PR → develop → CI → Deploy stage → smoke
                 └─ PR → main → CI → approval → Deploy prod → health
```

- Required check: job **`check`** (`make check-ci`).
- Migrate: [`scripts/render-migrate.sh`](../scripts/render-migrate.sh) в `preDeployCommand` (fail = fail deploy).
- Local: pre-commit = `make check-ci`; before push = `make check`.

## GitHub (один раз)

**Branch protection** (`develop`, `main`): require PR; required status **`check`**; up to date.

| Environment | Reviewers | Secrets |
| --- | --- | --- |
| `staging` | нет | `RENDER_DEPLOY_HOOK_STAGE`; optional `SMOKE_EMAIL` / `SMOKE_PASSWORD` |
| `production` | required | `RENDER_DEPLOY_HOOK_PROD` |

Deploy Hook: Render → service → Settings → Deploy Hook.

## Render Blueprint

1. Push [`render.yaml`](../render.yaml) → Dashboard Blueprint sync/apply.
2. Fill `sync: false` secrets on both APIs; wire Deploy Hooks into GitHub Environments.
3. Smoke: `GET …/health` (stage URL above).

### Legacy cutover → stage

```bash
pg_dump --no-owner --format=custom -f mentorix_stage.dump "$OLD_DATABASE_URL"
pg_restore --no-owner --clean --if-exists -d "$NEW_DATABASE_URL" mentorix_stage.dump
```

Redis не переносить. Затем фронт/CORS → stage URL; bot webhook; удалить legacy `mentorix-backend` / `mentorix-dev-*`.

### Runbook

| Проблема | Действие |
| --- | --- |
| migrate dirty | Render logs → fix / осторожный `migrate force` → redeploy |
| bad stage | fix-forward на `develop` или Rollback в Dashboard |
| bad prod | Rollback + hotfix → `main` |
| smoke без auth | задать `SMOKE_*` в Environment `staging` |

## Env

| Источник | Назначение |
| --- | --- |
| `.env.example` / `.env` | local (`.env` не в git) |
| `render.yaml` | `DATABASE_URL`/`REDIS_URL` linked; cookie/proxy; `BOT_WEBHOOK_URL` |
| Dashboard `sync: false` | `JWT_SECRET`, `BOT_*`, `TELEGRAM_BOT_USERNAME`, `CORS_ALLOW_ORIGINS`, `TRAINER_INVITE_TTL_DAYS` |

Integration: `TEST_DATABASE_URL` → `mentorix_test`. Front: access JSON, refresh HttpOnly cookie, `credentials: 'include'`.
