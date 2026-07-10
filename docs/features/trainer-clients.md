# Trainer clients (program assignment)

**Статус:** реализовано  
**Код:** `internal/trainerclient/`

## Назначение

Назначение замороженной версии программы клиенту тренера.

## БД

`trainer_clients`, `program_assignments`, `trainers`.

## API

Контракт: `api/openapi.yaml` — `GET /trainer/clients/{client_user_id}/program-assignment`, `PUT /trainer/clients/program-assignment`, `GET /trainer/clients` (список).

`GET /trainer/clients`: пагинация (`page`, `limit`), поиск по `display_name` (`q`, ILIKE), сортировка `sort_by=name|linked_at`, `sort_order=asc|desc` (по умолчанию `linked_at` desc). Тренер видит только своих клиентов; **admin** — всех клиентов, привязанных к любому тренеру (уникально по `client_user_id`, назначение программы в списке — от последней связи, только для обзора). Поле `avatar_url` — подписанный URL прокси фото из Telegram (пустой, если фото нет).

**Admin:** расширен только список (`GET /trainer/clients`). Назначение программ (`PUT …/program-assignment`, `GET …/program-assignment`) — как у тренера: только клиенты, привязанные к **этому** admin через его инвайт (`trainer_clients`), только **свои** опубликованные программы (`created_by` + `published`). Чужие клиенты в bulk → `skipped: not_linked`; чужая программа → `403`.

`GET /trainer/clients/{client_user_id}/avatar` — прокси аватара (`exp`, `sig` из `avatar_url`); для `<img src>`, без Bearer.

Неочевидные правила:

- Одна активная `program_assignments` на `(trainer_id, client_user_id)`.
- Версия — последняя замороженная; снятие — `program_id: null` в PUT.
- Назначение: `PUT /trainer/clients/program-assignment` — `client_user_ids` (1–100) + `program_id`; ответ `assigned` / `cleared` / `skipped` (как sync). Один клиент — массив из одного id.
- Sync активных назначений — эндпоинты программы (`POST .../assignments/sync`).

## См. также

- [trainer-invites.md](trainer-invites.md) — приглашение и связь с клиентом
- [programs.md](programs.md)
