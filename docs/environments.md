# Environments

1. **Local** — Postgres + Redis, `make run` or `make dev`; see [Env](#env) for env files.
2. **Stage** — `develop` → CI → Deploy Hook → `mentorix-api-stage`.
3. **Prod** — later (`main` + separate services); not yet in [`render.yaml`](../render.yaml).

Do not commit secrets.

## Names (stage)

| Component | Stage |
| --------- | ----- |
| Web | `mentorix-api-stage` → `https://mentorix-api-stage.onrender.com` |
| Postgres / Redis | `mentorix-db-stage` / `mentorix-redis-stage` |
| Frontend | `https://dashboard-u7fz.onrender.com` — origin in `CORS_ALLOW_ORIGINS`, base for `CLIENT_ANALYTICS_PAGE_URL` |

Plans: web **starter**, Postgres **basic-256mb** (PG 16), Redis **starter**, region **frankfurt**.

## Release flow (current)

```text
feature/* → PR → develop → CI → Deploy stage → smoke
```

- Required check: `check` job (`make check-ci`).
- Migrations: [`scripts/render-migrate.sh`](../scripts/render-migrate.sh) in `preDeployCommand`.
- Local: pre-commit runs `make check-quick`; before push run `make check`.
- Workflow [`deploy-prod.yml`](../.github/workflows/deploy-prod.yml) — manual only (`workflow_dispatch`), no prod on Render yet.

## GitHub setup (one-time)

**Branch protection** (`develop`): require PR; required status `check`.

| Environment | Secrets |
| --- | --- |
| `staging` | `RENDER_DEPLOY_HOOK_STAGE`; optional `SMOKE_EMAIL` / `SMOKE_PASSWORD` |

Deploy Hook: Render → `mentorix-api-stage` → Settings → Deploy Hook.

## Render Blueprint

1. Push [`render.yaml`](../render.yaml) (stage only) → Dashboard Blueprint sync.
2. Set `sync: false` secrets on stage API; connect Deploy Hook to Environment `staging`.
3. Smoke test: `GET https://mentorix-api-stage.onrender.com/health`.

If `mentorix-api` / `mentorix-db` / `mentorix-redis` already appear in Dashboard, delete them manually (Blueprint sync will not remove them).

### Legacy cutover to stage

```bash
pg_dump --no-owner --format=custom -f mentorix_stage.dump "$OLD_DATABASE_URL"
pg_restore --no-owner --clean --if-exists -d "$NEW_DATABASE_URL" mentorix_stage.dump
```

Do not migrate Redis. Then update frontend/CORS to stage URL, bot webhook, and delete legacy `mentorix-backend` / `mentorix-dev-*`.

### Runbook

| Problem | Action |
| --- | --- |
| migrate dirty | Check Render logs → fix / careful `migrate force` → redeploy |
| bad stage | fix-forward to `develop` or Rollback in Dashboard |
| smoke without auth | set `SMOKE_*` in Environment `staging` |

## Env

Local: single `.env` file, priority `shell > .env` (loaded in `internal/config.LoadDotenv`, scripts via `scripts/lib/env.sh`). Variables already set in shell are never overridden.

| File | In git | Content |
| --- | --- | --- |
| `.env.example` | yes | template for `.env`; `make setup` copies it if `.env` does not exist |
| `.env` | no | all local: ports, localhost Postgres/Redis, CORS, TTL, `JWT_SECRET`, `BOT_*` |
| `.env.stage` | no | only external `DATABASE_URL` (`sslmode=require`) and `REDIS_URL` (`rediss://`) for stage, for `make psql-stage` / `make redis-stage`; API does not read it |

Stage: files not used, all from Render.

| Source | Destination |
| --- | --- |
| `render.yaml` | stage links + cookie/proxy + `BOT_WEBHOOK_URL`; `ipAllowList` for DB and Key Value open to `0.0.0.0/0` for local tools |
| Dashboard `sync: false` | `JWT_SECRET`, `BOT_*`, `TELEGRAM_BOT_USERNAME`, `CORS_ALLOW_ORIGINS`, `TRAINER_INVITE_TTL_DAYS`, `CLIENT_ANALYTICS_PAGE_URL` |

Variables added to `render.yaml` with `sync: false` do not appear in Dashboard automatically — set them manually (Environment → Add).

Datastore CLI: `make psql` / `make redis` (local), `make psql-stage` / `make redis-stage` (Render; `ARGS="-c 'select 1'"`).
For `.env.stage` values: Dashboard → instance → External URL, or `render pg get` / `render kv get … --include-sensitive-connection-info`.

Frontend local to stage API: base URL `https://mentorix-api-stage.onrender.com`, `credentials: 'include'`; origin `http://localhost:3000` must be in stage `CORS_ALLOW_ORIGINS`.

Integration: `TEST_DATABASE_URL` → `mentorix_test` (in `.env`). Frontend: access JSON, refresh in HttpOnly cookie, `credentials: 'include'`.
