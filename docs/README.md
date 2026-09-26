# Mentorix Backend documentation

## Overview

| Document | Content |
| -------- | ------- |
| [product.md](product.md) | Product, roles, unified account |
| [architecture.md](architecture.md) | Monolith, tech stack, repository layout |
| [environments.md](environments.md) | Local, stage, prod; CI/CD, Deploy Hooks, runbook |
| [status.md](status.md) | Registry of implemented features (facts only) |
| [maintenance.md](maintenance.md) | How to maintain docs, rules, status |

## Features

| Document | Area |
| -------- | ---- |
| [features/health.md](features/health.md) | Health probes |
| [features/auth.md](features/auth.md) | Authentication |
| [features/exercises.md](features/exercises.md) | Exercise catalog |
| [features/programs.md](features/programs.md) | Training programs |
| [features/program-blocks.md](features/program-blocks.md) | Exercise blocks in a day |
| [features/program-block-visibility.md](features/program-block-visibility.md) | Block visibility per client |
| [features/trainer-clients.md](features/trainer-clients.md) | Program assignment to client |
| [features/trainer-invites.md](features/trainer-invites.md) | Trainer-to-client invites (Telegram) |
| [features/telegram-bot.md](features/telegram-bot.md) | Telegram bot for clients |
| [features/workout-completions.md](features/workout-completions.md) | Marking program days and history |
| [features/workout-comments.md](features/workout-comments.md) | Trainer responses to workout results |
| [features/trainer-analytics.md](features/trainer-analytics.md) | Trainer analytics: clients and programs |
| [features/client-analytics.md](features/client-analytics.md) | Client stats page linked from Telegram |
| [features/garbage-cleanup.md](features/garbage-cleanup.md) | Automated database and Redis cleanup |
| [features/subscriptions.md](features/subscriptions.md) | Trainer plans and quotas |
| [features/admin.md](features/admin.md) | Admin API |

REST contract: [`api/openapi.yaml`](../api/openapi.yaml).

## QA

- **Pre-commit:** `make install-hooks` runs `make check-quick`: gofmt, vet, unit, lint ([`scripts/git-hooks/pre-commit`](../scripts/git-hooks/pre-commit)); included in `make setup`.
- **Before commit:** docs and rules current — [CLAUDE.md](../CLAUDE.md) § Docs sync (`docs-check` in check-ci).
- **Before push:** `make check` — [CLAUDE.md](../CLAUDE.md) § Technical QA, [`scripts/check.sh`](../scripts/check.sh).
- **CI/CD:** [environments.md](environments.md).
