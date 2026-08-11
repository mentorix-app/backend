# Поддержка документации и правил

Зеркало [`.claude/rules/docs-and-rules-maintenance.md`](../.claude/rules/docs-and-rules-maintenance.md).

## Где что лежит

| Тип | Место |
| --- | ----- |
| Как делать (паттерны) | `.claude/rules/*.md` (грузятся по требованию) |
| Всегда в контексте + маршрутизация правил | `CLAUDE.md` |
| Повторяемый workflow | `.claude/skills/<name>/SKILL.md` |
| Что реализовано | `docs/status.md` |
| Фича (факты) | `docs/features/<name>.md` |
| Контракт API | `api/openapi.yaml` |

## Новая feature

1. `docs/features/<name>.md` + ссылка в [README.md](README.md).
2. После готовности — строка в [status.md](status.md).
3. Новый паттерн — в нужный rule-файл, не в docs.
4. Новый rule-файл невидим, пока на него нет строки в таблице `CLAUDE.md`.

## Лимиты

- Rule-файл: ~80–120 строк; skill: ~120; feature-doc: ~150; `CLAUDE.md`: ~120.
- Не копировать OpenAPI и naming rules в docs.

## Миграции

При добавлении `db/migrations/*.up.sql` обновить номер версии в [architecture.md](architecture.md) и [status.md](status.md) (один и тот же номер). Проверка: `./scripts/migrate-check.sh` (БД) и `./scripts/docs-check.sh` (docs).

## Перед коммитом

1. Сверить diff с docs/rules/status ([CLAUDE.md](../CLAUDE.md) § Docs sync).
2. Pre-commit / вручную: `make check-ci` (включает docs-check).

## Перед push

`make check` (полный: ещё migrate-check + smoke). CI на GitHub = `make check-ci`.

Release / Render: [environments.md](environments.md).

## status.md

- Только факты с кодом (+ OpenAPI для API).
- Секции «В работе» / «Запланировано» — только когда реально появятся.
