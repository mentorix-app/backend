# Admin

**Статус:** реализовано  
**Код:** `internal/admin/`

## Назначение

Выдача роли `admin` пользователю.

## БД

`user_roles`.

## API

Контракт: `api/openapi.yaml` — `POST /admin/users/{user_id}/roles/admin`.

Неочевидные правила:

- Вызывает trainer (выдаёт `admin` другому пользователю).

## См. также

- [auth.md](auth.md)
