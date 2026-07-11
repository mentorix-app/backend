# Окружения

## Фазы

1. **Локально:** Docker Postgres/Redis — `.env` из `.env.example`; автотесты (`make test`, `make test-integration`).
2. **Render dev:** ветка `develop` — ручное E2E (веб + Telegram).
3. **Render prod:** ветка `main` — после стабильного dev.

Секреты не коммитить. Prod-секреты не копировать из dev.

## Файлы env (единый паттерн)

Одинаковый набор переменных (23), один порядок, canon: `internal/config/config.go`.

| Файл | В git | Назначение |
| ---- | ----- | ---------- |
| [`.env.example`](../.env.example) | да | Шаблон локалки → `cp .env.example .env` |
| `.env` | **нет** | Локальная разработка |
| [`.env.dev.example`](../.env.dev.example) | да | Шаблон Render **dev** → `cp .env.dev.example .env.dev` |
| `.env.dev` | **нет** | Render dev (секреты, зеркало Dashboard) |
| [`.env.prod.example`](../.env.prod.example) | да | Шаблон Render **prod** → `cp .env.prod.example .env.prod` |
| `.env.prod` | **нет** | Render prod (секреты, зеркало Dashboard) |

**Локально:** `make setup` создаёт `.env` из `.env.example`, если файла нет.

**Render:** значения из `.env.dev` / `.env.prod` копируешь в Render Dashboard (и наоборот). Файлы только у себя, не в GitHub.

## Переменные по критичности

### Критичные (API)

| Переменная | Render dev / prod |
| ---------- | ----------------- |
| `DATABASE_URL` | External Postgres, `sslmode=require` |
| `JWT_SECRET` | ≥32 символов, **разный** на dev и prod |

### Критичные для E2E (Telegram + фронт)

| Переменная | Примечание |
| ---------- | ---------- |
| `REDIS_URL` | External Redis |
| `CORS_ALLOW_ORIGINS` | URL фронта |
| `TRUSTED_PROXY_CIDRS` | `private` |
| `TELEGRAM_BOT_USERNAME` | без `@`; dev/prod — разные боты |
| `BOT_TOKEN` | @BotFather |
| `BOT_WEBHOOK_URL` | `https://<api-host>/telegram/webhook` |
| `BOT_WEBHOOK_SECRET` | `openssl rand -hex 32` |

Cross-site фронт: `REFRESH_COOKIE_SAMESITE=none`, `REFRESH_COOKIE_SECURE=true`; prod — `REFRESH_COOKIE_DOMAIN`.

### Опциональные в коде (в шаблонах заданы явно)

`APP_ENV`, `PORT`, TTL токенов, cookie, rate limit, `TRAINER_INVITE_TTL_DAYS`.

## Render dev — порядок

1. `migrate up`
2. `cp .env.dev.example .env.dev` → заполнить → скопировать в Render Dashboard
3. Деплой → лог `telegram webhook registered`
4. E2E: login → invite → Telegram → assign

Фронт: access в JSON, refresh в HttpOnly cookie; `credentials: 'include'`; при 401 — `POST /auth/refresh`.
