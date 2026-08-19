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

## Правила видимости блоков (`program_block_clients`)

- `clear`/`reassign` (`SetClientProgramAssignment`) — в той же транзакции `DeleteProgramBlockClientsForClient` удаляет строки клиента для **прежней** `program_id` (не новой).
- Правило-сирота — `block_key`, которого нет ни в рабочей копии (`program_week_day_blocks`), ни в одной сохранившейся версии (`program_version_week_day_blocks`) программы; оба условия обязательны, версии — потому что клиент может быть назначен на версию, куда рабочая копия уже не смотрит.
- `PurgeOrphanProgramBlockClientsForProgram` (scoped) вызывается из `CleanupProgramVersions` и из best-effort прохода после `assign`/`reassign`/`clear`/`sync` — момент, когда у `block_key` может исчезнуть последняя версия.
- `PurgeOrphanProgramBlockClients` (глобальный) — то же самое без `program_id`, вызывается из `cleanup.Run`. Модель данных — [program-blocks.md](program-blocks.md).

## Инвайты

При `POST /trainer/invites` — inline purge expired/consumed (grace 7d) инвайтов тренера.

## Redis

`mentorix:telegram:active_trainer:{id}` — `Delete` при stale в `resolveActiveTrainerID`.

## Janitor

`go run ./cmd/janitor`: purge stale `trainer_invites`, `auth_refresh_sessions` (30d), orphan `program_block_clients` (global, unscoped).

Требует `DATABASE_URL`. Ничем не запланирован (нет cron в `render.yaml`, нет в `.github/workflows/`) — safety net, а не основной механизм; правила видимости блоков в первую очередь чистит best-effort проход выше.

## См. также

- [trainer-clients.md](trainer-clients.md)
- [programs.md](programs.md)
- [trainer-invites.md](trainer-invites.md)
