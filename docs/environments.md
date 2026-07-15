# Окружения

## Фазы

1. **Local:** нативный Postgres + Redis, API (`make run` / `make dev`). `.env` из `.env.example`.
2. **Render stage:** ветка `develop` — ручное E2E (веб + Telegram). Env в Render Dashboard (подсказки в том же `.env.example`).

Секреты не коммитить.

## Env — один файл

Одинаковый набор переменных, canon: `internal/config/config.go`.

| Файл | В git | Назначение |
| ---- | ----- | ---------- |
| [`.env.example`](../.env.example) | да | Шаблон: активные значения = local; комментарии = Render stage → Dashboard |
| `.env` | **нет** | Рабочая копия → `cp .env.example .env` |

**Local:** `make setup` создаёт `.env`, если файла нет. Postgres/Redis ставятся отдельно (`brew install postgresql@16 redis` или аналог).

**Render stage:** stage-строки из `.env.example` / своего `.env` копируешь в Render Dashboard (и наоборот). Dashboard — источник правды для задеплоенного сервиса.

**Integration tests:** отдельная БД `mentorix_test`, переменная `TEST_DATABASE_URL` (см. `.env.example`).

## Переменные по критичности

### Критичные (API)

| Переменная | Local | Render stage |
| ---------- | ----- | ------------ |
| `DATABASE_URL` | `localhost`, `sslmode=disable` | External Postgres, `sslmode=require` |
| `JWT_SECRET` | ≥32 символов | ≥32, свой для stage |

### Критичные для E2E (Telegram + фронт) на stage

| Переменная | Примечание |
| ---------- | ---------- |
| `REDIS_URL` | External Redis |
| `CORS_ALLOW_ORIGINS` | URL фронта |
| `TRUSTED_PROXY_CIDRS` | `private` |
| `TELEGRAM_BOT_USERNAME` | без `@` |
| `BOT_TOKEN` | @BotFather |
| `BOT_WEBHOOK_URL` | `https://<api-host>/telegram/webhook` |
| `BOT_WEBHOOK_SECRET` | `openssl rand -hex 32` |

Cross-site фронт на stage: `REFRESH_COOKIE_SAMESITE=none`, `REFRESH_COOKIE_SECURE=true`.

### Опциональные в коде (в шаблоне заданы явно)

`APP_ENV` (`development`), `PORT`, TTL токенов, cookie, rate limit, `TRAINER_INVITE_TTL_DAYS`.

## Render stage — порядок

1. `migrate up` на stage DB
2. Заполнить Dashboard по stage-комментариям в `.env.example`
3. Деплой ветки `develop` → лог `telegram webhook registered`
4. E2E: login → invite → Telegram → assign

Фронт: access в JSON, refresh в HttpOnly cookie; `credentials: 'include'`; при 401 — `POST /auth/refresh`.
