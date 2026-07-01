# Архитектура

## Модель

- **Монолит** Go: один деплой REST (Echo v4) + PostgreSQL + Redis.
- Telegram webhook — тот же бэкенд (когда будет).
- Без Supabase; свой auth в Go и Postgres.
- Микросервисы, публичный gRPC, полный OpenTelemetry — только по явному запросу.

## Стек

| Область | Выбор |
| ------- | ----- |
| HTTP | Echo v4 |
| БД | PostgreSQL, схема `mentorix`, pgx + sqlc, golang-migrate |
| Кэш/лимиты | Redis |
| Auth | JWT + opaque refresh (cookie), argon2id |
| Деплой | Render (dev: `develop`, prod позже: `main`) |
| Логи | `log/slog`, request ID |

Миграции: версия **13**, проверка `./scripts/migrate-check.sh`.

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
