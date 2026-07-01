# Окружения

## Фазы

1. **Локально:** Docker Postgres/Redis, `.env`, `APP_ENV=development`.
2. **Render dev:** ветка `develop`, отдельные Postgres/Redis.
3. **Prod:** ветка `main`, новые секреты — после стабильного dev.

Локальный `.env` — только локальные URL; не prod-секреты.

## Переменные (основные)

| Переменная | Назначение |
| ---------- | ---------- |
| `APP_ENV` | `development` \| `production` |
| `PORT` | HTTP, default `8080` |
| `DATABASE_URL` | PostgreSQL |
| `REDIS_URL` | Redis; пусто — rate limit off |
| `JWT_SECRET` | HS256, ≥32 символов |
| `CORS_ALLOW_ORIGINS` | Origins SPA |
| `ACCESS_TTL_MINUTES` / `REFRESH_TTL_DAYS` | TTL токенов |
| `REFRESH_COOKIE_*` | HttpOnly cookie — см. `.env.example` |
| `AUTH_*_RATE_*` | Rate limit auth по IP |
| `TRUSTED_PROXY_CIDRS` | `private` на Render |

Полный список: [`.env.example`](../.env.example).

## Render dev

После деплоя: `migrate up` по External `DATABASE_URL`. Обязательно `TRUSTED_PROXY_CIDRS=private`. Кросс-доменный фронт: `REFRESH_COOKIE_SAMESITE=none`, `REFRESH_COOKIE_SECURE=true`, `CORS_ALLOW_ORIGINS`.

## Seed

```bash
make seed                    # локально, идемпотентно
SEED_CONFIRM=yes make seed   # удалённая БД
```

Учётка dev: `dev-trainer@test.com` / `Password123` (только dev). ~30 упражнений, ~20 программ.

Фронт: access в JSON, refresh в HttpOnly cookie; `credentials: 'include'`; при 401 — `POST /auth/refresh`.
