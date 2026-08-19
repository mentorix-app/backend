# Program blocks

**Статус:** реализовано  
**Код:** `internal/program/` (расширение)  
**Связано:** [programs.md](programs.md)

## Назначение

День программы — упорядоченный список **блоков**. Single = одно упражнение; группа = блок с `block_type` ≠ `single`, создаётся только через merge.

## БД

Дерево (см. [database-naming.md](../../.claude/rules/database-naming.md)):

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
| `block_key` | uuid NOT NULL DEFAULT `gen_random_uuid()` | стабильная идентичность блока между publish и discard |

`block_type`: `single`, `emom`, `amrap`, `for_time`, `intervals`, `chipper`, `ladder`, `death_by`, `superset`, `complex`, `skill_work`, `strength`, `conditioning`, `gymnastics`, `weightlifting`.

### `program_week_day_block_exercises`

- FK: `program_week_day_block_id` → `program_week_day_blocks`.
- Поля: `exercise_id`, `sort_order`, `sets`, `reps`, `instruction`, `created_at`.

Зеркало для version-дерева (см. naming rule).

### `program_block_clients` — видимость блока

| Колонка | Тип | Примечание |
| ------- | --- | ---------- |
| `id` | uuid PK | |
| `program_id` | uuid FK → `programs` | |
| `block_key` | uuid | не FK — стабильный ключ, а не `program_week_day_blocks.id` |
| `client_user_id` | uuid FK → `users` | |
| `created_at`, `created_by` | | |

UNIQUE `(program_id, block_key, client_user_id)`. Строк для `(program_id, block_key)` нет → блок виден всем клиентам программы — отдельного boolean-флага нет. `GET .../programs/{id}` возвращает `blocks[].client_user_ids`: пустой массив `[]`, никогда `null` — виден всем клиентам программы. `block_key` переживает publish и `discard-unpublished` (working copy ↔ frozen version), поэтому правило продолжает указывать на тот же блок в любой версии. Внутренний инвариант: `GetVersionDetail` заполняет `BlockKey` и применяет правила программы — это не HTTP-контракт, а то, что не даёт `discard-unpublished` пересоздать ключи блоков и молча сломать все правила.

Запись правил — `PUT .../blocks/{block_id}/clients`, полная замена списка; пустой список возвращает блок к общему доступу. День обязан сохранять хотя бы один общий блок: ограничение отклоняется `400` (`ErrLastSharedBlock`), если сделает день без общего блока — проверка идёт по рабочей копии **и** по каждой опубликованной версии с активным назначением, потому что клиенты сидят на этих версиях прямо сейчас, без publish. Проверка запускается только на переходе shared → restricted; смена состава клиентов уже ограниченного блока или снятие ограничения инвариант не ломает. `client_user_ids` в теле должны быть клиентами, назначенными на программу — иначе `400` (`ErrClientNotAssignedToProgram`).

**`sort_order`:** после любой операции, затрагивающей порядок (create, move, reorder, delete, merge, ungroup, extract), store нормализует siblings к уникальным `1..N` без дублей. Ответ `GET /programs/{id}` — дерево уже отсортировано по `sort_order`.

### Миграция данных

На каждое существующее упражнение дня — блок `single` + перенос строки. Publish-копирование включает блоки.

## Поведение (продукт)

| Действие | Результат |
| -------- | --------- |
| + упражнение в день (каталог, мультивыбор) | N блоков `single` (`POST .../blocks`) |
| + упражнение в группу | `POST .../blocks/{block_id}/exercises` (группа уже есть после merge) |
| Создать группу | только `POST .../blocks/merge` (2+ блока); default `complex`; instruction — склейка instruction участвующих **групп** (`\n\n`, пустые skip); только singles → `""`; **400** (`ErrValidation`), если у участников разные наборы клиентов (в т.ч. «все общие» ≠ «один ограничен») — единственно верного результата нет, склейка отклоняется |
| Разгруппировать | каждое упражнение → `single`, наследует список клиентов группы |
| Удалить упражнение из `single` | блок удаляется |
| Удалить из группы | группа остаётся (0/1/2+); **не** становится `single` |
| Перенос в другой день | только целый `single` или целая группа (⋮ → день) |
| DnD внутри дня | блоки; упражнения в группе; extract / в группу / между группами |

Упражнение в группе в другой день: сначала extract → `single` (наследует список клиентов группы), потом перенос.

## Publish

- `single`: ровно 1 упражнение; `sets`/`reps` опциональны (`null`/omit); при значении — строка: только цифры (`3`), один `/` (`5/4`) или один `-` (`3-6`).
- группа (`block_type` ≠ `single`): ≥ 1 упражнение; пустая группа — ошибка; для каждого упражнения те же правила `sets`/`reps`.
- `instruction` группы может быть `""`.

Остальная валидация — [programs.md](programs.md).

## План реализации

### Фаза 1 — схема

1. `db/migrations/000014_program_day_blocks.up.sql` — таблицы блоков, alter exercises, version-дерево, data migration, drop `weight_kg`.
2. `db/queries/program.sql`, `program_version.sql` — CRUD блоков.
3. `sqlc generate`, `make schema-sync`.
4. Обновить `database-naming.md` (дерево), `architecture.md`, `status.md` (номер миграции).

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
| merge / ungroup / move / reorder | см. [api-endpoints.md](../../.claude/rules/api-endpoints.md) § Program week subtree; reorder — полный список id siblings |
| Видимость блока | `PUT .../blocks/{block_id}/clients` — `client_user_ids`, полная замена |

## Вне MVP

Structured `settings` по `block_type`; таймеры клиента; `weight_kg`; undo merge.

## См. также

- [programs.md](programs.md) — программы, publish, версии
