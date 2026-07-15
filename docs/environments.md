# Окружения

## Фазы

1. **Local:** нативный Postgres + Redis, API (`make run` / `make dev`). `.env` из `.env.example`.
2. **Render stage:** ветка `develop`, инфра через Blueprint [`render.yaml`](../render.yaml) — `mentorix-api-stage` / `mentorix-db-stage` / `mentorix-redis-stage`.

Секреты не коммитить.

## Имена stage (и prod на будущее)

| | Stage (сейчас) | Prod (позже) |
| --- | --- | --- |
| Web | `mentorix-api-stage` → `https://mentorix-api-stage.onrender.com` | `mentorix-api` |
| Postgres | `mentorix-db-stage` | `mentorix-db` |
| Redis | `mentorix-redis-stage` | `mentorix-redis` |

Plans stage: web **starter**, Postgres **basic-256mb** (PG 16), Redis **starter** (Free недоступен параллельно со старым free Key Value). Region: **frankfurt**.

## Env

| Файл | В git | Назначение |
| ---- | ----- | ---------- |
| [`.env.example`](../.env.example) | да | Local + комментарии stage |
| `.env` | **нет** | Локальная рабочая копия |
| [`render.yaml`](../render.yaml) | да | Blueprint: `DATABASE_URL`/`REDIS_URL` из linked services; секреты `sync: false` |

**Local:** `make setup` создаёт `.env`, если файла нет.

**Render stage:** после Blueprint в Dashboard заполнить `JWT_SECRET`, `CORS_ALLOW_ORIGINS`, `BOT_*`, `TELEGRAM_BOT_USERNAME`, … Зеркало значений можно держать в личном `.env` (не в git).

**Integration tests:** `TEST_DATABASE_URL` → БД `mentorix_test` (см. `.env.example`).

## Render stage — новый стенд (Blueprint)

1. Закоммить / запушь `render.yaml` в `develop`.
2. Dashboard → **New → Blueprint** → этот репо → файл `render.yaml` → apply (создаст **новые** сервисы; старые `mentorix-backend` / `mentorix-dev-*` не трогает).
3. Заполнить `sync: false` секреты на `mentorix-api-stage`.
4. Дождаться deploy → `GET https://mentorix-api-stage.onrender.com/health`.

### Cutover данных (со старого Postgres)

Пока старый API (`mentorix-backend.onrender.com`) обслуживает клиентов. Параллельно:

```bash
# External Database URL старого mentorix-dev-db
pg_dump --no-owner --format=custom -f mentorix_stage.dump "$OLD_DATABASE_URL"

# External Database URL нового mentorix-db-stage
pg_restore --no-owner --clean --if-exists -d "$NEW_DATABASE_URL" mentorix_stage.dump
```

Redis не переносить. Dump-файл не коммитить; удалить после успешного restore.

### Переключение трафика

1. Фронт: CORS / API base URL → `https://mentorix-api-stage.onrender.com`
2. `BOT_WEBHOOK_URL` уже в Blueprint; убедиться, что секрет/токен совпадают → redeploy / лог `telegram webhook registered`
3. Postman: environment **Mentorix Render Stage**
4. E2E: login → invite → Telegram
5. Удалить legacy: `mentorix-backend`, `mentorix-dev-db`, `mentorix-dev-redis`

Пока оба набора живы — платишь за оба (Starter + Basic-256mb ×2 примерно).

## Переменные по критичности (stage)

| Переменная | Как задаётся |
| ---------- | ------------ |
| `DATABASE_URL` / `REDIS_URL` | Blueprint `fromDatabase` / `fromService` |
| `JWT_SECRET`, `BOT_TOKEN`, `BOT_WEBHOOK_SECRET`, `TELEGRAM_BOT_USERNAME`, `CORS_ALLOW_ORIGINS` | Dashboard (`sync: false`) |
| Cookie / proxy | в `render.yaml` (`none` / `true` / `private`) |
| `BOT_WEBHOOK_URL` | `https://mentorix-api-stage.onrender.com/telegram/webhook` |

Фронт: access в JSON, refresh в HttpOnly cookie; `credentials: 'include'`; при 401 — `POST /auth/refresh`.
