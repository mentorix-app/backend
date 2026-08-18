---
name: changelog
description: Use when asked for a changelog for the frontend or Telegram ("чейнджлог для фронта", "changelog по паттерну") — produces a single Telegram-ready message in chat describing API-visible changes since a commit.
---

# Changelog format (frontend / Telegram)

Output a single Telegram-ready message **in chat** — never a file.

Gather the diff first: `git log --oneline <from>..HEAD`, then
`git diff <from>..HEAD -- api/openapi.yaml` for the contract change.
Ask for the base commit when it was not given.

## Template

```text
📝 **Changelog** (с `<commit>`, `<branch>`):

🔹 **<Тема/эндпоинт>** — что изменилось для клиента API: поля, типы, статусы

🔹 **Breaking — <тема>:** старое → новое; что править на фронте

Контракт: `api/openapi.yaml` (<operationId / схемы>)
```

## Rules

- Header exactly `📝 **Changelog**` + commit hash and branch in parentheses.
- One `🔹` bullet per change; bold lead-in, then compact details in one paragraph.
- Audience — API client (frontend): endpoints, request/response fields, types,
  error statuses (400/403/404/409) that need UI handling.
- No internal kitchen: migrations, sqlc, tests, package names, CI.
- Breaking changes marked explicitly: `**Breaking — …:**`.
- Last line points to `api/openapi.yaml` with operationId / schema names.
- Language: Russian.

## Example bullet

🔹 **`GET /trainer/clients/{client_user_id}/completions`** — в каждом item новое
поле `comments: [{ id, text, created_at }]` (всегда массив, отсортирован по времени)
