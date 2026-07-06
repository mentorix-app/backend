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
- `video_url` — опционально; если задан, только HTTPS-ссылка на YouTube (`youtube.com`, `youtu.be`: `/watch`, `/embed`, `/shorts`, `/live`).

## См. также

- `.cursor/rules/database-naming.mdc`
