# Programs

**Статус:** реализовано  
**Код:** `internal/program/`

## Назначение

Шаблоны программ тренера: недели → дни → упражнения; publish создаёт замороженные `program_versions`.

## БД

`programs`, `program_weeks`, `program_days`, `program_day_exercises`, `program_versions` (+ дерево), `program_assignments`.

## API

Контракт: `api/openapi.yaml` (тег Programs).

Неочевидные правила:

- `POST /programs` → `draft`, пустое `name`, Week 1 / Day 1 автоматически.
- Статусы: `draft` → `published` → `archived`; нет `published` → `draft`.
- Publish из `archived` — только смена статуса, без новой версии.
- Publish (draft): валидация name/category/difficulty, ≥1 неделя, в каждом дне ≥1 упражнение, sets/reps > 0.
- Published: in-place edit + `has_unpublished_changes`; publish-update — новая версия.
- Trainer видит свои; admin — все.

## См. также

- [trainer-clients.md](trainer-clients.md) — назначение клиенту
