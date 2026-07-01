# Документация Mentorix Backend

## Общее

| Документ | Содержание |
| -------- | ---------- |
| [product.md](product.md) | Продукт, роли, единый аккаунт |
| [architecture.md](architecture.md) | Монолит, стек, layout репозитория |
| [environments.md](environments.md) | Окружения, env, Render, seed |
| [status.md](status.md) | Реестр реализованного (только факты) |
| [maintenance.md](maintenance.md) | Как вести docs, rules, status |

## Фичи

| Документ | Область |
| -------- | ------- |
| [features/health.md](features/health.md) | Health probes |
| [features/auth.md](features/auth.md) | Аутентификация |
| [features/exercises.md](features/exercises.md) | Справочник упражнений |
| [features/programs.md](features/programs.md) | Программы тренировок |
| [features/trainer-clients.md](features/trainer-clients.md) | Назначение программы клиенту |
| [features/admin.md](features/admin.md) | Admin API |

Контракт REST: [`api/openapi.yaml`](../api/openapi.yaml).

## QA

- **Перед коммитом:** актуальность docs/rules — [qa-before-push.mdc](../.cursor/rules/qa-before-push.mdc) §A; `make docs-check`.
- **Перед push:** `make check` или `make check-ci` — тот же файл §B, [`scripts/check.sh`](../scripts/check.sh).
