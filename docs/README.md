# Документация Mentorix Backend

## Общее

| Документ | Содержание |
| -------- | ---------- |
| [product.md](product.md) | Продукт, роли, единый аккаунт |
| [architecture.md](architecture.md) | Монолит, стек, layout репозитория |
| [environments.md](environments.md) | Local, stage, prod; CI/CD, Deploy Hooks, runbook |
| [status.md](status.md) | Реестр реализованного (только факты) |
| [maintenance.md](maintenance.md) | Как вести docs, rules, status |

## Фичи

| Документ | Область |
| -------- | ------- |
| [features/health.md](features/health.md) | Health probes |
| [features/auth.md](features/auth.md) | Аутентификация |
| [features/exercises.md](features/exercises.md) | Справочник упражнений |
| [features/programs.md](features/programs.md) | Программы тренировок |
| [features/program-blocks.md](features/program-blocks.md) | Блоки упражнений в дне |
| [features/trainer-clients.md](features/trainer-clients.md) | Назначение программы клиенту |
| [features/trainer-invites.md](features/trainer-invites.md) | Инвайты тренер → клиент (Telegram) |
| [features/telegram-bot.md](features/telegram-bot.md) | Telegram-бот для клиента |
| [features/workout-completions.md](features/workout-completions.md) | Отметка дней программы / история |
| [features/workout-comments.md](features/workout-comments.md) | Ответ тренера на результат тренировки |
| [features/trainer-analytics.md](features/trainer-analytics.md) | Аналитика тренера: клиент и программы |
| [features/garbage-cleanup.md](features/garbage-cleanup.md) | Автоочистка БД и Redis |
| [features/subscriptions.md](features/subscriptions.md) | Тарифы тренеров и квоты |
| [features/admin.md](features/admin.md) | Admin API |

Контракт REST: [`api/openapi.yaml`](../api/openapi.yaml).

## QA

- **Pre-commit:** `make install-hooks` — `make check-ci` ([`scripts/git-hooks/pre-commit`](../scripts/git-hooks/pre-commit)); входит в `make setup`.
- **Перед коммитом:** актуальность docs/rules — [CLAUDE.md](../CLAUDE.md) § Docs sync (`docs-check` внутри check-ci).
- **Перед push:** `make check` — [CLAUDE.md](../CLAUDE.md) § Technical QA, [`scripts/check.sh`](../scripts/check.sh).
- **CI/CD:** [environments.md](environments.md).
