# Trainer clients (program assignment)

**Статус:** реализовано  
**Код:** `internal/trainerclient/`

## Назначение

Назначение замороженной версии программы клиенту тренера.

## БД

`trainer_clients`, `program_assignments`, `trainers`.

## API

Контракт: `api/openapi.yaml` — `GET /trainer/clients/{client_user_id}/program-assignment`, `PUT /trainer/clients/program-assignment`, `GET /trainer/clients` (список).

`GET /trainer/clients`: пагинация (`page`, `limit`), поиск по `display_name` (`q`, ILIKE), сортировка `sort_by=name|linked_at`, `sort_order=asc|desc` (по умолчанию `linked_at` desc). Тренер видит только своих клиентов; **admin** — всех клиентов, привязанных к любому тренеру (уникально по `client_user_id`). Если admin сам привязан к клиенту как тренер — в строке его связь (`linked_at`, `status`, `program_assignment`); иначе — последняя связь по `linked_at`. Поле `trainer_user_id` — чей user id у выбранной связи; фронт сравнивает с `user_id` из `/auth/me`, чтобы разрешить assign только «своим» клиентам. Поле `avatar_url` — подписанный URL прокси фото из Telegram (пустой, если фото нет).

**Admin:** расширен только список (`GET /trainer/clients`). Назначение программ (`PUT …/program-assignment`, `GET …/program-assignment`) — как у тренера: только клиенты, привязанные к **этому** admin через его инвайт (`trainer_clients`), только **свои** опубликованные программы (`created_by` + `published`). Чужие клиенты в bulk → `skipped: not_linked`; чужая программа → `403`. Sync (`POST /programs/{id}/assignments/sync`) — тоже только владелец программы (`created_by`); admin на чужой программе → `403`.

`GET /trainer/clients/{client_user_id}/avatar` — прокси аватара (`exp`, `sig` из `avatar_url`); для `<img src>`, без Bearer.

Неочевидные правила:

- Одна активная `program_assignments` на `(trainer_id, client_user_id)` — **одна строка** в БД; `reassign` обновляет её (новый `completion_cycle_id` при смене `program_id`), `clear` удаляет.
- Повторный `PUT` с тем же `program_id` — skip `already_assigned` (без UPDATE).
- Версия — последняя замороженная; снятие — `program_id: null` в PUT.
- Назначение: `PUT /trainer/clients/program-assignment` — `client_user_ids` (1–100) + `program_id`; ответ `assigned` / `cleared` / `skipped` (как sync). Один клиент — массив из одного id.
- `program_assignment` в списке: `assignment_id`, `program_id`, `program_version_id`, `assigned_at`, `program_name`, `program_name_ru` (из замороженной версии), `is_behind_latest` (можно sync, если `true`; `assignment_id` — в body sync).
- Sync активных назначений — эндпоинты программы (`POST .../assignments/sync`).

## См. также

- [trainer-invites.md](trainer-invites.md) — приглашение и связь с клиентом
- [programs.md](programs.md)
