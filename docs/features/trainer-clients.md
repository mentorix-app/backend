# Trainer clients (program assignment)

**Статус:** реализовано  
**Код:** `internal/trainerclient/`

## Назначение

Назначение замороженной версии программы клиенту тренера.

## БД

`trainer_clients`, `program_assignments`, `trainers`.

## API

Контракт: `api/openapi.yaml` — `GET/PUT /trainer/clients/{client_user_id}/program-assignment`, `GET /trainer/clients` (список).

`GET /trainer/clients`: пагинация (`page`, `limit`), поиск по `display_name` (`q`, ILIKE), сортировка `sort_by=name|linked_at`, `sort_order=asc|desc` (по умолчанию `linked_at` desc).

Неочевидные правила:

- Одна активная `program_assignments` на `(trainer_id, client_user_id)`.
- Версия — последняя замороженная; снятие — `program_id: null` в PUT.
- Sync активных назначений — эндпоинты программы (`POST .../assignments/sync`).

## См. также

- [trainer-invites.md](trainer-invites.md) — приглашение и связь с клиентом
- [programs.md](programs.md)
