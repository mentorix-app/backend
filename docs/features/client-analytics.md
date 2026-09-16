# Client analytics page

**Статус:** реализовано  
**Код:** `internal/analytics/client_link.go`, `internal/analytics/` (`ClientSelfAnalytics`), `internal/telegrambot/` (кнопка «📊 Статистика»)

## Назначение

Клиент из Telegram-бота открывает в браузере страницу со своей статистикой у активного тренера. Бот выдаёт подписанную ссылку, фронт пересылает её query-строку на `GET /client/analytics` и рендерит ответ. Входа в веб у клиента нет: ссылка сама является пропуском на 30 минут.

## Безопасность

- Ссылка: `<CLIENT_ANALYTICS_PAGE_URL>?client_user_id&trainer_id&exp&sig`; `sig` — HMAC-SHA256 на `JWT_SECRET` с доменом `client_analytics:` (отдельно от аватар-ссылок). Подделать или подменить `client_user_id` / `trainer_id` нельзя.
- TTL 30 минут; отзыва конкретной ссылки нет. Пока ссылка жива, её можно переслать — принято осознанно, страница только на чтение.
- Связь `trainer_clients` и статус `blocked` проверяются при каждом запросе, не при выдаче.
- В сообщении бота ссылка спрятана в inline URL-кнопку.

## API

Контракт: `api/openapi.yaml` — `GET /client/analytics`. Без JWT. `400` — параметр отсутствует/не парсится, `401` — подпись или срок, `404` — клиент не связан с тренером или заблокирован.

Ответ: `client` (id, имя), `trainer` (`trainers.id`, имя), `current_assignment` и `activity` — те же схемы, что в тренерской аналитике ([trainer-analytics.md](trainer-analytics.md)), `recent_completions` — последние 30 тренировок с ответами тренера, `expires_at` — срок ссылки.

## Бот

Кнопка «📊 Статистика» есть только при заданном `CLIENT_ANALYTICS_PAGE_URL`. Тренер — активный, как для «Программа»; нет активного → «Выберите тренера в разделе «Тренеры»». Подробности меню — [telegram-bot.md](telegram-bot.md).

## Env

| Переменная | Обязательна | Назначение |
| ---------- | ----------- | ---------- |
| `CLIENT_ANALYTICS_PAGE_URL` | нет | Абсолютный `http(s)` URL страницы фронта без query; пусто — кнопки нет |

Origin страницы должен быть в `CORS_ALLOW_ORIGINS`.

## См. также

- [trainer-analytics.md](trainer-analytics.md) — те же расчёты со стороны тренера
- [telegram-bot.md](telegram-bot.md) — меню бота
