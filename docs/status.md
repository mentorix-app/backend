# Статус проекта

Только **реализованное**, согласованное с кодом. Планы добавлять с нуля по мере работы.

## Реализовано

| Область | Ссылка |
| ------- | ------ |
| Блоки упражнений в дне программы (single / группы, merge, move) | [features/program-blocks.md](features/program-blocks.md) |
| Инфра (health, config, slog, CORS, proxy) | [features/health.md](features/health.md), [architecture.md](architecture.md) |
| Auth (register, login, refresh, logout, me, rate limit) | [features/auth.md](features/auth.md) |
| Admin (выдача admin) | [features/admin.md](features/admin.md) |
| Упражнения (list/get, admin CRUD, soft delete) | [features/exercises.md](features/exercises.md) |
| Программы (CRUD, publish, версии, assignments, sync) | [features/programs.md](features/programs.md) |
| Назначение программы клиенту | [features/trainer-clients.md](features/trainer-clients.md) |
| Инвайты тренер → клиент (Telegram deep link) | [features/trainer-invites.md](features/trainer-invites.md) |
| Telegram-бот: /start, accept, меню, программа, отметка тренировок, push (фазы 1–4) | [features/telegram-bot.md](features/telegram-bot.md), [features/workout-completions.md](features/workout-completions.md) |
| Отметка дней программы (day_key, cycle, журнал completions) | [features/workout-completions.md](features/workout-completions.md) |
| Контракт OpenAPI + Postman + apicheck | [architecture.md](architecture.md), `api/openapi.yaml` |
| CI (vet, test, build, lint, contract, integration, 85% coverage) | [maintenance.md](maintenance.md) |
| sqlc store + integration tests | `internal/db/` |
| Render dev | [environments.md](environments.md) |
| Garbage cleanup (assignments one-row, auto version purge, invites, Redis stale, janitor) | [features/garbage-cleanup.md](features/garbage-cleanup.md) |

_Миграции: версия 20 (`./scripts/migrate-check.sh`)._

