# Exercises

**Статус:** реализовано  
**Код:** `internal/exercise/`

## Назначение

Общий каталог упражнений.

## БД

`exercises` (soft delete: `deleted_at`).

## API

Контракт: `api/openapi.yaml` (тег Exercises).

Неочевидные правила:

- List/get — любой JWT; create/update/delete — только `admin`.
- Enum в БД/API: `snake_case` (`exercise_type`).

## См. также

- `.cursor/rules/database-naming.mdc`
