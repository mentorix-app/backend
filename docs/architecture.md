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
| Деплой | Render Blueprint ([`render.yaml`](../render.yaml)): stage `develop`; CI-gated Deploy Hook |
| Логи | `log/slog`, request ID |

Миграции: версия **25**, проверка `./scripts/migrate-check.sh`.

## Layout

```text
cmd/api/           — REST + Telegram webhook + push
internal/<feature>/ — домен
internal/db/sqlc/  — generated
db/migrations/     — SQL
db/queries/        — sqlc
api/openapi.yaml   — контракт REST
```

**Telegram SDK:** `github.com/go-telegram-bot-api/telegram-bot-api/v5` — де-факто стандарт Go; альтернативы `telebot` (выше уровень), raw HTTP (избыточно).
