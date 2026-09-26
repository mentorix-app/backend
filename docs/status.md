# Project status

Lists only **implemented** features, aligned with code. Plans are added as work begins.

## Implemented

| Feature | Link |
| ------- | ---- |
| Exercise blocks in a program day (single / groups, merge, move) | [features/program-blocks.md](features/program-blocks.md) |
| Per-client block visibility (`block_key`, client list on block) | [features/program-block-visibility.md](features/program-block-visibility.md) |
| Infrastructure (health, config, slog, CORS, proxy) | [features/health.md](features/health.md), [architecture.md](architecture.md) |
| Auth (register, login, refresh, logout, me, rate limit) | [features/auth.md](features/auth.md) |
| Admin (separate role from trainer; global exercises, plans, view-only) | [features/admin.md](features/admin.md) |
| Trainer plans (free/advance/elite, quotas, read-only, admin grant, `/plans`, subscription in `/auth/me`) | [features/subscriptions.md](features/subscriptions.md) |
| Exercises (global + trainer, scope, quota, soft delete) | [features/exercises.md](features/exercises.md) |
| Programs (CRUD, publish, discard-unpublished, versions, assignments, sync) | [features/programs.md](features/programs.md) |
| Program assignment to client | [features/trainer-clients.md](features/trainer-clients.md) |
| Trainer-to-client invites (Telegram deep link) | [features/trainer-invites.md](features/trainer-invites.md) |
| Telegram bot: /start, accept, menu, program, mark workouts, push (phases 1–4) | [features/telegram-bot.md](features/telegram-bot.md), [features/workout-completions.md](features/workout-completions.md) |
| Mark program days (day_key, cycle, completion log) | [features/workout-completions.md](features/workout-completions.md) |
| Trainer analytics (client: progress/activity/feed; programs: aggregates, drop-off, week matrix) | [features/trainer-analytics.md](features/trainer-analytics.md) |
| Client stats page signed from Telegram (`GET /client/analytics`, button «📊 Статистика») | [features/client-analytics.md](features/client-analytics.md) |
| Trainer responses to workout results (comment + push to Telegram) | [features/workout-comments.md](features/workout-comments.md) |
| OpenAPI contract + apicheck | [architecture.md](architecture.md), `api/openapi.yaml` |
| CI/CD (`make check-ci`, Deploy Hook stage, migrate preDeploy) | [environments.md](environments.md), [maintenance.md](maintenance.md) |
| sqlc store + integration tests | `internal/db/` |
| Render stage (Blueprint) | [environments.md](environments.md), [`render.yaml`](../render.yaml) |
| Garbage cleanup (one-row assignments, auto version purge, orphaned block visibility rules, invites, stale Redis, janitor) | [features/garbage-cleanup.md](features/garbage-cleanup.md) |

_Migrations: version 26 (`./scripts/migrate-check.sh`)._

