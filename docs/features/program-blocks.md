# Program blocks

**Статус:** реализовано  
**Код:** `internal/program/` (расширение)  
**Связано:** [programs.md](programs.md)

## Назначение

День программы — упорядоченный список **блоков**. Single = одно упражнение; группа = блок с `block_type` ≠ `single`, создаётся только через merge.

## БД

Дерево (см. [database-naming.mdc](../../.cursor/rules/database-naming.mdc)):

```text
program_week_days → program_week_day_blocks → program_week_day_block_exercises
program_version_week_days → program_version_week_day_blocks → program_version_week_day_block_exercises
```

### `program_week_day_blocks`

| Колонка | Тип | Примечание |
| ------- | --- | ---------- |
| `id` | uuid PK | |
| `program_week_day_id` | uuid FK → `program_week_days` | |
| `block_type` | text + CHECK | не `type` |
| `instruction` | text DEFAULT `''` | |
| `sort_order` | int | порядок в дне |
| `created_at` | timestamptz | |
| `modified_at` | timestamptz | |
| `modified_by` | uuid FK → `users` | nullable |

`block_type`: `single`, `emom`, `amrap`, `for_time`, `intervals`, `chipper`, `ladder`, `death_by`, `superset`, `complex`.

### `program_week_day_block_exercises`

- FK: `program_week_day_block_id` → `program_week_day_blocks`.
- Поля: `exercise_id`, `sort_order`, `sets`, `reps`, `instruction`, `created_at`.

Зеркало для version-дерева (см. naming rule).

**`sort_order`:** после любой операции, затрагивающей порядок (create, move, reorder, delete, merge, ungroup, extract), store нормализует siblings к уникальным `1..N` без дублей. Ответ `GET /programs/{id}` — дерево уже отсортировано по `sort_order`.

### Миграция данных

На каждое существующее упражнение дня — блок `single` + перенос строки. Publish-копирование включает блоки.

## Поведение (продукт)

| Действие | Результат |
| -------- | --------- |
| + упражнение в день (каталог, мультивыбор) | N блоков `single` (`POST .../blocks`) |
| + упражнение в группу | `POST .../blocks/{block_id}/exercises` (группа уже есть после merge) |
| Создать группу | только `POST .../blocks/merge` (2+ блока); default `complex`; instruction — склейка instruction участвующих **групп** (`\n\n`, пустые skip); только singles → `""` |
| Разгруппировать | каждое упражнение → `single` |
| Удалить упражнение из `single` | блок удаляется |
| Удалить из группы | группа остаётся (0/1/2+); **не** становится `single` |
| Перенос в другой день | только целый `single` или целая группа (⋮ → день) |
| DnD внутри дня | блоки; упражнения в группе; extract / в группу / между группами |

Упражнение в группе в другой день: сначала extract → `single`, потом перенос.

## Publish

- `single`: ровно 1 упражнение; `sets`/`reps` опциональны (можно не передавать или `null`), при значении — ≥ 1.
- группа (`block_type` ≠ `single`): ≥ 1 упражнение; пустая группа — ошибка; для каждого упражнения те же правила `sets`/`reps`.
- `instruction` группы может быть `""`.

Остальная валидация — [programs.md](programs.md).

## План реализации

### Фаза 1 — схема

1. `db/migrations/000014_program_day_blocks.up.sql` — таблицы блоков, alter exercises, version-дерево, data migration, drop `weight_kg`.
2. `db/queries/program.sql`, `program_version.sql` — CRUD блоков.
3. `sqlc generate`, `make schema-sync`.
4. Обновить `database-naming.mdc` (дерево), `architecture.md`, `status.md` (номер миграции).

### Фаза 2 — домен и чтение

5. Go: `Block`, `BlockType`, `Day.blocks[]`; убрать `weight_kg` из DTO.
6. Store: загрузка дня с блоками; snapshot в version publish.
7. Тесты store + integration.

### Фаза 3 — добавление (совместимость UI)

8. `POST .../days/{day_id}/exercises` — массив → N `single` блоков (как сейчас).
9. `POST .../blocks/{block_id}/exercises` — в группу.
10. Удаление упражнения / пустой single.

### Фаза 4 — блоки: merge, ungroup, patch

11. `POST .../days/{day_id}/merge` — `{ block_ids: [] }`; склейка instruction.
12. `POST .../blocks/{block_id}/ungroup`.
13. `PATCH .../blocks/{block_id}` — `block_type` (группа, не `single`), `instruction`; **single-блоки → 400**.

### Фаза 5 — перемещение и порядок

14. Reorder блоков в дне; reorder упражнений в блоке.
15. `POST .../blocks/{block_id}/move` — другой день (single или группа).
16. Extract упражнения → `single` в дне; move между блоками.

### Фаза 6 — контракт и QA

17. `api/openapi.yaml` — `ProgramDayBlock`, breaking: день через `blocks`.
18. Postman, `internal/apicheck/schema.go`.
19. Обновить [programs.md](programs.md); publish-валидация.
20. `make check`; строка в `status.md` → реализовано.

## API

Контракт — `api/openapi.yaml`. День = `blocks[]`. Single — обычный блок (`block_type: single`); для пользователя выглядит как одно упражнение, но API всегда оперирует блоками.

| Операция | API |
| -------- | --- |
| Добавить «упражнение» в день | `POST .../days/{day_id}/blocks` — всегда `single` + `exercise` |
| Создать группу | `POST .../blocks/merge` (2+ блока) |
| Обновить упражнение | `PUT .../blocks/{block_id}/exercises/{item_id}` |
| Удалить упражнение | `DELETE .../blocks/{block_id}/exercises/{item_id}` |
| Добавить в группу | `POST .../blocks/{block_id}/exercises` |
| merge / ungroup / move / reorder | см. [api-endpoints.mdc](../../.cursor/rules/api-endpoints.mdc) § Program week subtree; reorder — полный список id siblings |

## Вне MVP

Structured `settings` по `block_type`; таймеры клиента; `weight_kg`; undo merge.

## См. также

- [programs.md](programs.md) — программы, publish, версии
