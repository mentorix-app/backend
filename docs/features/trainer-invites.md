# Trainer invites

**Статус:** реализовано  
**Код:** `internal/trainerclient/`

## Назначение

Приглашение клиентов тренером через Telegram deep link; создание связи `trainer_clients` после accept в боте.

## БД

`trainer_invites`, `trainer_clients`, `auth_identities` (`provider = telegram`), `user_roles` (`client`).

Новых миграций нет — таблицы из init + `000007`.

## API

Контракт: `api/openapi.yaml` (тег `trainer`).

Неочевидные правила:

- `POST /trainer/invites` требует `TELEGRAM_BOT_USERNAME`; ссылка `https://t.me/<bot>?start=inv_<token>`.
- TTL инвайта — `TRAINER_INVITE_TTL_DAYS` (default 7). При создании нового инвайта — inline purge expired/consumed (grace 7d) старых инвайтов тренера.
- Accept в боте (`/start inv_<token>`) — in-process `trainerclient.Service.AcceptInvite`, не HTTP; при наличии — сохраняется фото профиля Telegram.
- Accept: создаёт `user` + `auth_identity(telegram)` + роль `client`, если Telegram id новый; иначе линкует существующего user.
- Повторный accept того же инвайта тем же user — идемпотентный ok (`already_linked: true` если связь уже была).
- Инвайт, consumed другим user → 409; просрочен → 410.
- `GET /trainer/clients` — пагинированный список с опциональным active `program_assignment`; поиск по имени (`q`), сортировка по `name` или `linked_at`. Admin видит всех клиентов (dedupe, приоритет своей связи, `trainer_user_id`) — см. [trainer-clients.md](trainer-clients.md).
- Назначение программы — только после accept; см. [trainer-clients.md](trainer-clients.md).

## Фазы (Telegram client)

| Фаза | Статус |
| ---- | ------ |
| 1. Invites + clients list + accept | готово |
| 2. Минимальный Telegram-бот | готово |
| 3. Программа в боте (read-only) | готово |
| 4. Push при назначении / sync программы | готово |

## См. также

- [telegram-bot.md](telegram-bot.md) — бот клиента
- [trainer-clients.md](trainer-clients.md) — назначение программы
- [product.md](../product.md) — клиент через Telegram
