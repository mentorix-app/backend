# Telegram bot (client)

**Статус:** реализовано (MVP, фазы 1–4)  
**Код:** `internal/telegrambot/`, `internal/telegramnotify/` (входящие updates и push — в `cmd/api`)

## Назначение

Клиентский интерфейс в Telegram: accept invite, меню, просмотр программы, отметка дня («Тренировка выполнена»), push при назначении и обновлении программы.

## Решения

- Инвайт только deep link (`inv_<token>`), без ввода `@username`.
- Accept на `/start inv_<token>` — in-process вызов `trainerclient.Service.AcceptInvite`; сохраняется фото профиля Telegram (`avatar_file_path`), если есть.
- При сообщениях боту аватар обновляется, если пользователь сменил фото.
- **Webhook** на API (`POST /telegram/webhook`) — один Render Web Service, без отдельного worker.
- Библиотека: `go-telegram-bot-api/v5` (см. [architecture.md](../architecture.md)).
- Меню: reply keyboard (`Программа`, `Тренеры`, `Помощь`) + inline для выбора тренера и навигации по программе.
- **Активный тренер:** Redis (`active_trainer` key — см. [redis-naming.mdc](../../.cursor/rules/redis-naming.mdc)).
  - После accept инвайта — автоматически активный = тренер из инвайта.
  - При нескольких тренерах клиент меняет активного в «Тренеры» (inline); после выбора — одно сообщение с активным тренером и программой.
  - Один тренер — выбор не показываем, он активен по умолчанию.
- **Программа:** краткая информация → inline выбор недели → inline выбор дня → упражнения на день. В списках только недели/дни с ≥1 упражнением (пустые блоки и дни отдыха не показываются). Неделя с всеми выполненными тренировочными днями — «✅ Неделя N»; в выборе дней — «✅ День N». На экране дня: «Тренировка выполнена» / «✅ Тренировка выполнена»; «Следующий день» или «Следующая неделя»; на последнем дне программы кнопки навигации нет. Отметка дня — см. [workout-completions.md](workout-completions.md).

## Push-уведомления (исходящие)

Только два события; best-effort (без `BOT_TOKEN` или без Telegram у клиента — пропуск).

| Событие | Триггер |
| ------- | ------- |
| Назначена программа | `PUT /trainer/clients/program-assignment` с `program_id` и `client_user_ids` |
| Обновлена программа | `POST /programs/{id}/assignments/sync`, assignment в `synced` |

Не шлём: снятие программы (`program_id: null`), accept invite (welcome в боте), skipped sync.

## Меню

| Кнопка | Поведение |
| ------ | --------- |
| Программа | краткая информация + inline выбор недели → дня → упражнения; на экране дня — отметка тренировки, «Следующий день» / «Следующая неделя» |
| Тренеры | 1 тренер — карточка; 2+ — список + inline выбор активного |
| Помощь | справка |

Формат «Программа» / «Тренеры»: иконки в заголовке (`📅`/`👤`, `💪`, `📆`), упражнения `🏋 название - 3х8` с инструкцией курсивом; групповые блоки — иконка типа (`🧩 Комплекс:`, `🔗 Суперсет:`, `🎯 Skill Work:`, `🦾 Сила:`, `🏃 Кондишн:`, `🤸 Гимнастика:`, `🏅 Тяжёлая атлетика:` и т.д.), инструкция блока, упражнения `А.` / `Б.`. «Тренеры»: карточка `👤 Тренер:` + `💪 Программа:`; при нескольких — `✓` у активного, inline-выбор.

Программа не назначена → «Тренер пока не назначил программу». Нет дней с упражнениями → «Тренер пока не добавил тренировочные дни в программу».

## Webhook (Render)

Stage URL: `https://mentorix-api-stage.onrender.com/telegram/webhook` ([`render.yaml`](../../render.yaml)).

| Метод | Путь | Назначение |
| ----- | ---- | ---------- |
| POST | `/telegram/webhook` | updates от Telegram (`X-Telegram-Bot-Api-Secret-Token`) |

При старте API регистрирует webhook в Telegram (`setWebhook`).

## Фазы

| # | Содержание | Статус |
| - | ---------- | ------ |
| 1 | Invites backend | готово — [trainer-invites.md](trainer-invites.md) |
| 2 | Бот: `/start`, accept, меню | готово |
| 3 | Read API программы + экраны | готово |
| 4 | Push при назначении / sync | готово |
| 5 | Отметка тренировок клиентом | готово — [workout-completions.md](workout-completions.md) |

## Env (API на Render)

| Переменная | Обязательна | Назначение |
| ---------- | ----------- | ---------- |
| `BOT_TOKEN` | да | Telegram Bot API; push + webhook |
| `BOT_WEBHOOK_URL` | да | `https://<host>/telegram/webhook` |
| `BOT_WEBHOOK_SECRET` | да | секрет для заголовка Telegram |
| `TELEGRAM_BOT_USERNAME` | да | invite URL |
| `REDIS_URL` | да | активный тренер |

## См. также

- [trainer-invites.md](trainer-invites.md)
- [environments.md](../environments.md)
