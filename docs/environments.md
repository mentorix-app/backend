# Окружения

1. **Local** — Postgres + Redis, `make run` / `make dev`; env-файлы — см. [Env](#env).
2. **Stage** — `develop` → CI → Deploy Hook → `mentorix-api-stage`.
3. **Prod** — позже (`main` + отдельные сервисы); пока **не** в [`render.yaml`](../render.yaml).

Секреты не коммитить.

## Имена (stage)

| | Stage |
| --- | --- |
| Web | `mentorix-api-stage` → `https://mentorix-api-stage.onrender.com` |
| Postgres / Redis | `mentorix-db-stage` / `mentorix-redis-stage` |
| Frontend | `https://dashboard-u7fz.onrender.com` — origin в `CORS_ALLOW_ORIGINS`, база для `CLIENT_ANALYTICS_PAGE_URL` |

Plans: web **starter**, Postgres **basic-256mb** (PG 16), Redis **starter**, region **frankfurt**.

## Release flow (сейчас)

```text
feature/* → PR → develop → CI → Deploy stage → smoke
```

- Required check: job **`check`** (`make check-ci`).
- Migrate: [`scripts/render-migrate.sh`](../scripts/render-migrate.sh) в `preDeployCommand`.
- Local: pre-commit = `make check-ci`; before push = `make check`.
- Workflow [`deploy-prod.yml`](../.github/workflows/deploy-prod.yml) — только руками (`workflow_dispatch`), пока нет prod на Render.

## GitHub (один раз)

**Branch protection** (`develop`): require PR; required status **`check`**.

| Environment | Secrets |
| --- | --- |
| `staging` | `RENDER_DEPLOY_HOOK_STAGE`; optional `SMOKE_EMAIL` / `SMOKE_PASSWORD` |

Deploy Hook: Render → `mentorix-api-stage` → Settings → Deploy Hook.

## Render Blueprint

1. Push [`render.yaml`](../render.yaml) (только stage) → Dashboard Blueprint sync.
2. Fill `sync: false` secrets on stage API; wire Deploy Hook into Environment `staging`.
3. Smoke: `GET https://mentorix-api-stage.onrender.com/health`.

Если в Dashboard уже появились `mentorix-api` / `mentorix-db` / `mentorix-redis` — **удали вручную** (Sync Blueprint сам их не сотрёт).

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
| smoke без auth | задать `SMOKE_*` в Environment `staging` |

## Env

Local: один файл `.env`, приоритет `shell > .env` (загрузка в `internal/config.LoadDotenv`,
скрипты — `scripts/lib/env.sh`). Уже выставленные в shell переменные никогда не перекрываются.

| Файл | В git | Содержание |
| --- | --- | --- |
| `.env.example` | да | шаблон `.env`; `make setup` копирует его, если `.env` ещё нет |
| `.env` | нет | всё локальное: порты, localhost Postgres/Redis, CORS, TTL, `JWT_SECRET`, `BOT_*` |
| `.env.stage` | нет | только внешние `DATABASE_URL` (`sslmode=require`) и `REDIS_URL` (`rediss://`) стейджа для `make psql-stage` / `make redis-stage`; API его не читает |

Stage: файлы не используются, всё из Render.

| Источник | Назначение |
| --- | --- |
| `render.yaml` | stage links + cookie/proxy + `BOT_WEBHOOK_URL`; `ipAllowList` БД и Key Value открыт (`0.0.0.0/0`) для локальных инструментов |
| Dashboard `sync: false` | `JWT_SECRET`, `BOT_*`, `TELEGRAM_BOT_USERNAME`, `CORS_ALLOW_ORIGINS`, `TRAINER_INVITE_TTL_DAYS`, `CLIENT_ANALYTICS_PAGE_URL` |

Добавленная в `render.yaml` переменная `sync: false` в Dashboard сама не появляется — её заводят руками (Environment → Add).

Datastore CLI: `make psql` / `make redis` (локально), `make psql-stage` / `make redis-stage` (Render; `ARGS="-c 'select 1'"`).
Значения для `.env.stage`: Dashboard → инстанс → External URL, или `render pg get` / `render kv get … --include-sensitive-connection-info`.

Frontend локально → stage API: base URL `https://mentorix-api-stage.onrender.com`, `credentials: 'include'`; origin `http://localhost:3000` должен быть в `CORS_ALLOW_ORIGINS` стейджа.

Integration: `TEST_DATABASE_URL` → `mentorix_test` (в `.env`). Front: access JSON, refresh HttpOnly cookie, `credentials: 'include'`.
