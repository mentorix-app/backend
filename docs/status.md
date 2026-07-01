# Статус проекта

Только **реализованное**, согласованное с кодом. Планы добавлять с нуля по мере работы.

## Реализовано

| Область | Ссылка |
| ------- | ------ |
| Инфра (health, config, slog, CORS, proxy) | [features/health.md](features/health.md), [architecture.md](architecture.md) |
| Auth (register, login, refresh, logout, me, rate limit) | [features/auth.md](features/auth.md) |
| Admin (выдача admin) | [features/admin.md](features/admin.md) |
| Упражнения (list/get, admin CRUD, soft delete) | [features/exercises.md](features/exercises.md) |
| Программы (CRUD, publish, версии, assignments, sync) | [features/programs.md](features/programs.md) |
| Назначение программы клиенту | [features/trainer-clients.md](features/trainer-clients.md) |
| Контракт OpenAPI + Postman + apicheck | [architecture.md](architecture.md), `api/openapi.yaml` |
| CI (vet, test, build, lint, contract, integration, 85% coverage) | [maintenance.md](maintenance.md) |
| sqlc store + integration tests | `internal/db/` |
| Render dev | [environments.md](environments.md) |
| Dev seed | [environments.md](environments.md) |

_Миграции: версия 13 (`./scripts/migrate-check.sh`)._
