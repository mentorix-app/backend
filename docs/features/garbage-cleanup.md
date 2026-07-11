# Garbage cleanup

**Статус:** реализовано  
**Код:** `internal/program/`, `internal/trainerclient/`, `internal/cleanup/`, `cmd/janitor/`

## Назначение

Автоудаление данных, которые перестали быть нужны после операций или по TTL.

## Assignments

- Одна строка `program_assignments` на `(trainer_id, client_user_id)`.
- `assign` / `reassign` — `INSERT` или `UPDATE` той же строки.
- `clear` (`program_id: null`) — `DELETE` строки.
- Миграция `000019`: purge `cancelled`, unique без `WHERE status = 'active'`.

## Версии программ

После `assign`/`reassign`/`clear`/`sync` — best-effort `CleanupProgramVersions` для затронутых программ (удаляет версии без active assignments, кроме единственной).

При `DELETE /programs/{id}` — сначала DELETE assignments программы, cleanup версий, затем soft delete.

## Инвайты

При `POST /trainer/invites` — inline purge expired/consumed (grace 7d) инвайтов тренера.

## Redis

`mentorix:telegram:active_trainer:{id}` — `Delete` при stale в `resolveActiveTrainerID`.

## Janitor

`go run ./cmd/janitor` (cron): purge stale `trainer_invites`, `auth_refresh_sessions` (30d).

Требует `DATABASE_URL`.

## См. также

- [trainer-clients.md](trainer-clients.md)
- [programs.md](programs.md)
- [trainer-invites.md](trainer-invites.md)
