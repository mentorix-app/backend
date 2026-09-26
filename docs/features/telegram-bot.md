# Telegram bot (client)

**Status:** implemented (MVP, phases 1–4)  
**Code:** `internal/telegrambot/`, `internal/telegramnotify/` (incoming updates and push in `cmd/api`)

## Purpose

Client interface in Telegram: accept invite, menu, view program, mark workout day («Тренировка выполнена»), push on assignment and program update.

## Design

- Invites use deep links only (`inv_<token>`), no `@username` entry.
- Accept via `/start inv_<token>` calls `trainerclient.Service.AcceptInvite` in-process; saves Telegram profile photo (`avatar_file_path`) if present.
- Avatar updates when the user sends a message if they've changed their photo.
- **Webhook** on API (`POST /telegram/webhook`) runs in one Render Web Service, no separate worker.
- Library: `go-telegram-bot-api/v5` (see [architecture.md](../architecture.md)).
- Menu: reply keyboard («Программа», «Тренеры», «Помощь») plus inline buttons for trainer selection and program navigation, plus «Статистика» when `CLIENT_ANALYTICS_PAGE_URL` is set.
- **Active trainer** stored in Redis (`active_trainer` key; see [redis-naming.md](../../.claude/rules/redis-naming.md)).
  - After accepting an invite, the active trainer is set to the trainer from the invite.
  - With multiple trainers, the client picks an active one in «Тренеры» (inline); after selection, one message shows the active trainer and program.
  - With one trainer, selection is hidden; they're active by default.
  - No active trainer (2+ trainers, Redis key missing or stale): «Тренеры» still shows the list with inline selection; «Программа» asks to pick a trainer in «Тренеры».
- **Program view:** summary, then inline week selection, then inline day selection, then exercises for the day. Only weeks and days with ≥1 exercise appear (empty blocks and rest days are hidden). A week with all workouts done shows «✅ Неделя N»; in day selection, «✅ День N». On the day screen: «Тренировка выполнена» or «✅ Тренировка выполнена»; «Следующий день» or «Следующая неделя»; the last day has no navigation buttons. Marking a workout is described in [workout-completions.md](workout-completions.md).
- **Block visibility:** clients see only blocks they can access; see [program-block-visibility.md](program-block-visibility.md). The `trainerclient.clientProgramView` calls `program.GetVersionDetailForClient`, which fetches the version and removes blocks the client cannot see. The `day_snapshot` created when marking a workout comes from the already-filtered day, so hidden blocks never enter the log.

## Push notifications (outgoing)

Two events only; best-effort (skipped if `BOT_TOKEN` is missing or client has no Telegram link).

| Event | Trigger |
| ------- | ------- |
| Program assigned | `PUT /trainer/clients/program-assignment` with `program_id` and `client_user_ids` |
| Program updated | `POST /programs/{id}/assignments/sync`, assignment moves to `synced` |

No push for: program removal (`program_id: null`), invite acceptance (welcome shown in bot), skipped sync.

## Menu

| Button | Behavior |
| ------ | --------- |
| «Программа» | Summary, then inline week selection → day selection → exercises; on day screen: mark workout, «Следующий день» or «Следующая неделя» |
| «Тренеры» | 1 trainer: card; 2+: list with inline selection for active trainer |
| «Статистика» | Inline URL button to trainer's analytics page; link valid 30 minutes — [client-analytics.md](client-analytics.md) |
| «Помощь» | Help text |

Format for «Программа» and «Тренеры»: icons in header (`📅` or `👤`, `💪`, `📆`); exercises shown as `🏋 name - 3x8` with instructions in italics. Group blocks show a type icon (`🧩 Комплекс:`, `🔗 Суперсет:`, `🎯 Skill Work:`, `🦾 Сила:`, `🏃 Кондишн:`, `🤸 Гимнастика:`, `🏅 Тяжёлая атлетика:`, etc.), block instructions, and exercises labeled `А.` and `Б.`. «Тренеры» shows a trainer card `👤 Тренер:` plus `💪 Программа:`; with multiple trainers, a `✓` marks the active one, with inline selection.

If no program is assigned: «<trainer name> пока не назначил программу». If the program has no workout days: «<trainer name> пока не добавил тренировочные дни в программу». In both cases, the trainer's display name is substituted.

## Webhook (Render)

Stage URL: `https://mentorix-api-stage.onrender.com/telegram/webhook` (see [`render.yaml`](../../render.yaml)).

| Method | Path | Purpose |
| ----- | ---- | ---------- |
| POST | `/telegram/webhook` | Updates from Telegram (verified with `X-Telegram-Bot-Api-Secret-Token`) |

The API registers the webhook with Telegram on startup via `setWebhook`.

## Phases

| # | Content | Status |
| - | ---------- | ------ |
| 1 | Invites backend | done — [trainer-invites.md](trainer-invites.md) |
| 2 | Bot: `/start`, accept, menu | done |
| 3 | Read API program + screens | done |
| 4 | Push on assignment / sync | done |
| 5 | Client marks workout day | done — [workout-completions.md](workout-completions.md) |

## Env (API on Render)

| Variable | Required | Purpose |
| ---------- | ----------- | ---------- |
| `BOT_TOKEN` | yes | Telegram Bot API; push and webhook |
| `BOT_WEBHOOK_URL` | yes | `https://<host>/telegram/webhook` |
| `BOT_WEBHOOK_SECRET` | yes | Secret for Telegram header |
| `TELEGRAM_BOT_USERNAME` | yes | Invite URL |
| `REDIS_URL` | yes | Active trainer storage |
| `CLIENT_ANALYTICS_PAGE_URL` | no | Enables «Статистика» button; empty = no button |

## See also

- [trainer-invites.md](trainer-invites.md)
- [environments.md](../environments.md)
