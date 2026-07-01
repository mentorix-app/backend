# Health

**Статус:** реализовано  
**Код:** `internal/health/`

## Назначение

Liveness и readiness для оркестрации и мониторинга.

## БД

Нет.

## API

Контракт: `api/openapi.yaml` (paths `/health`, `/health/ready`).

Неочевидные правила:

- `/health` — всегда ok; `/health/ready` — database/redis `ok` или `skipped` если URL пустой.

## См. также

- [environments.md](../environments.md) — локальная проверка
