# Архитектура

## Модель

- **Монолит** Go: один деплой REST (Echo v4) + PostgreSQL + Redis.
- Свой auth в Go и Postgres (без Supabase).

## Стек

| Область | Выбор |
| ------- | ----- |
| HTTP | Echo v4 |
| БД | PostgreSQL, схема `mentorix`, pgx + sqlc, golang-migrate |
| Кэш/лимиты | Redis |
| Auth | JWT + opaque refresh (cookie), argon2id |
| Деплой | Render Web Service, ветка `develop` (dev) |
| Логи | `log/slog`, request ID |

Миграции: версия **16**, проверка `./scripts/migrate-check.sh`.

## Layout

```text
cmd/api/           — wiring
internal/<feature>/ — домен
internal/db/sqlc/  — generated
db/migrations/     — SQL
db/queries/        — sqlc
api/openapi.yaml   — контракт REST
```

Перед новой зависимостью — сравнить 2–3 варианта; значимый выбор — записать здесь кратко.
