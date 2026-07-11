# Programs

**Статус:** реализовано  
**Код:** `internal/program/`

## Назначение

Шаблоны программ тренера: недели → дни → блоки → упражнения; publish создаёт замороженные `program_versions`.

## БД

`programs`, `program_weeks`, `program_week_days`, `program_week_day_blocks`, `program_week_day_block_exercises`, `program_versions` (+ version-дерево), `program_assignments`.

## API

Контракт: `api/openapi.yaml` (тег Programs).

Неочевидные правила:

- `POST /programs` → `draft`, пустое `name`, неделя 1 с **7 пустыми днями** автоматически.
- `PUT …/reorder` (недели, дни, блоки, упражнения в блоке): body — **полный** упорядоченный список id siblings (каждый ровно один раз); частичный список → 400 `invalid reorder: … count mismatch`.
- Статусы: `draft` → `published` → `archived`; нет `published` → `draft`.
- Publish из `archived` — только смена статуса, без новой версии.
- Publish (draft): валидация name/category/difficulty, ≥1 неделя; в каждом непустом дне — блоки с упражнениями (`single`: 1 упражнение; группа: ≥1); `sets`/`reps` опциональны (можно не передавать или `null`), при значении — ≥ 1.
- `training_days_count` в `Program` / `ProgramDetail`: число дней с ≥1 блоком или упражнением (пустые дни не считаются); в списке — из SQL, в `GET /programs/{id}` — из загруженных недель.
- Published: in-place edit + `has_unpublished_changes`; publish-update — новая версия.
- Trainer видит свои; admin — все.
- `POST /programs/{id}/assignments/sync` — только владелец программы (`created_by`); admin без владения → `403` (как assign).

## См. также

- [program-blocks.md](program-blocks.md) — блоки в дне
- [trainer-clients.md](trainer-clients.md) — назначение клиенту
