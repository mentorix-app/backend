# Exercises

**Статус:** реализовано  
**Код:** `internal/exercise/`

## Назначение

Каталог упражнений: глобальные (админские, общие для всех) и приватные (тренерские, под квотой тарифа).

## БД

`exercises` (soft delete: `deleted_at`; `owner_trainer_id NULL` = глобальное, иначе — владелец-тренер).

## API

Контракт: `api/openapi.yaml` (тег Exercises).

Неочевидные правила:

- Доступ — `trainer` или `admin`. Admin: видит все, CRUD только глобальных. Trainer: видит глобальные + свои, CRUD только своих; создание/редактирование под квотой тарифа (`409 quota_exceeded`, см. [subscriptions.md](subscriptions.md)); удаление всегда разрешено.
- Ответ содержит `scope` (`global|private`) и `owner_user_id`; фильтр списка `?scope=`.
- В программу тренер может добавлять глобальные и **свои** упражнения; чужие приватные → 400. Существующие ссылки grandfathered — проверка только при добавлении/замене упражнения.
- Enum в БД/API: `snake_case` (`exercise_type`).
- `video_url` — опционально; если задан, только HTTPS-ссылка на YouTube (`youtube.com`, `youtu.be`: `/watch`, `/embed`, `/shorts`, `/live`).

## См. также

- [subscriptions.md](subscriptions.md) — квоты
- `.claude/rules/database-naming.md`
