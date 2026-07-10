# Telegram bot (client)

**Статус:** реализовано (MVP, фазы 1–4)  
**Код:** `internal/telegrambot/`, `internal/telegramnotify/` (входящие updates и push — в `cmd/api`)

## Назначение

Клиентский интерфейс в Telegram: accept invite, меню, просмотр программы, push при назначении и обновлении программы. **MVP без отметки тренировок клиентом.**

## Решения

- Инвайт только deep link (`inv_<token>`), без ввода `@username`.
- Accept на `/start inv_<token>` — in-process вызов `trainerclient.Service.AcceptInvite`; сохраняется фото профиля Telegram (`avatar_file_path`), если есть.
- При сообщениях боту аватар обновляется, если пользователь сменил фото.
- **Webhook** на API (`POST /telegram/webhook`) — один Render Web Service, без отдельного worker.
- Библиотека: `go-telegram-bot-api/v5` (см. [architecture.md](../architecture.md)).
- Меню: reply keyboard (`Сегодня`, `Программа`, `Тренеры`, `Помощь`) + inline для выбора тренера.
- **Активный тренер:** Redis (`mentorix:telegram:active_trainer:<telegram_user_id>`).
  - После accept инвайта — автоматически активный = тренер из инвайта.
  - При нескольких тренерах клиент меняет активного в «Тренеры» (inline).
  - Один тренер — выбор не показываем, он активен по умолчанию.
- **Сегодня:** день программы = `days_since(assigned_at)` по плоскому списку дней версии.

## Push-уведомления (исходящие)

Только два события; best-effort (без `BOT_TOKEN` или без Telegram у клиента — пропуск).

| Событие | Триггер |
| ------- | ------- |
| Назначена программа | `PUT /trainer/clients/{id}/program-assignment` с `program_id` |
| Обновлена программа | `POST /programs/{id}/assignments/sync`, assignment в `synced` |

Не шлём: снятие программы (`program_id: null`), accept invite (welcome в боте), skipped sync.

## Меню

| Кнопка | Поведение |
| ------ | --------- |
| Сегодня | тренировка на сегодня для активного тренера |
| Программа | вся назначенная программа (может быть несколько сообщений) |
| Тренеры | 1 тренер — карточка; 2+ — список + inline выбор активного |
| Помощь | справка |

Формат тренировки: иконка типа блока (`▫️` single, `🔗` суперсет, `⏱` EMOM и т.д.), подходы×повторы, инструкции блока и упражнения курсивом (если тренер указал).

Программа не назначена → «Тренер пока не назначил программу».

## Webhook (Render)

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
