# Поддержка документации и правил

Зеркало [`.cursor/rules/docs-and-rules-maintenance.mdc`](../.cursor/rules/docs-and-rules-maintenance.mdc).

## Где что лежит

| Тип | Место |
| --- | ----- |
| Как делать (паттерны) | `.cursor/rules/*.mdc` |
| Что реализовано | `docs/status.md` |
| Фича (факты) | `docs/features/<name>.md` |
| Контракт API | `api/openapi.yaml` |

## Новая feature

1. `docs/features/<name>.md` + ссылка в [README.md](README.md).
2. После готовности — строка в [status.md](status.md).
3. Новый паттерн — в нужный `.mdc`, не в docs.

## Лимиты

- Rule-файл: ~80–120 строк; feature-doc: ~150; `index.mdc`: ~40.
- Не копировать OpenAPI и naming rules в docs.

## Миграции

При добавлении `db/migrations/*.up.sql` обновить номер версии в [architecture.md](architecture.md) и [status.md](status.md) (один и тот же номер). Проверка: `./scripts/migrate-check.sh` (БД) и `./scripts/docs-check.sh` (docs).

## Перед коммитом

1. Сверить diff с docs/rules/status ([qa-before-push.mdc](../.cursor/rules/qa-before-push.mdc) §A).
2. `./scripts/docs-check.sh` или `make docs-check`.

## Перед push

`make check` (полный) или `make check-ci` (как GitHub CI).

## status.md

- Только факты с кодом (+ OpenAPI для API).
- Секции «В работе» / «Запланировано» — только когда реально появятся.
