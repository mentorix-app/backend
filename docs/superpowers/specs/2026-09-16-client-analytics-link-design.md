# Страница статистики клиента по ссылке из Telegram

**Дата:** 2026-09-16
**Статус:** дизайн согласован, реализация не начата

## Задача

Клиент нажимает в боте кнопку «Статистика» и получает ссылку. По ссылке в
браузере открывается страница фронта, фронт вызывает новый эндпоинт и
рендерит аналитику клиента: текущая программа, прогресс, активность, последние
тренировки с ответами тренера.

Решения, принятые с заказчиком:

1. Без входа клиента в веб. Ссылка сама является пропуском на ограниченное
   время (подписанный URL, как у аватаров).
2. Данные в разрезе **активного тренера** — того же, что выбран в боте для
   «Программа».
3. Один эндпоинт: сводка плюс последние тренировки, без пагинации.
4. Страница одна и только для чтения. Если клиентский веб вырастет дальше
   одной страницы, схему авторизации надо будет заменить на обмен одноразового
   токена на JWT; текущий дизайн этого не предусматривает намеренно.

## Контекст

- Клиент — единый `user_id`, роль `client`; веб-входа у клиента нет, JWT
  выдаётся только по email+паролю (`internal/auth`).
- Аналитика по клиенту (`internal/analytics`, `Service.ClientAnalytics`)
  считается от `trainers.id` и `client_user_id`; тренерский хендлер сначала
  переводит `trainerUserID` → `trainers.id` через `Service.trainerID`.
- Бот (`internal/telegrambot`) знает только `telegram_user_id`. Сервис
  `trainerclient` резолвит `client_user_id` (`store.ClientUserIDByTelegram`) и
  активного тренера (`resolveActiveTrainerID`: один тренер → он; 2+ → Redis
  `active_trainer`, иначе `ErrActiveTrainerNotSet`).
- Подписанные ссылки уже есть: `trainerclient.BuildAvatarURL` /
  `VerifyAvatarURL` — HMAC-SHA256 на `JWT_SECRET`, префикс домена `avatar:`,
  параметры `exp` и `sig` в query, TTL 24 ч.
- Базового URL фронта в конфиге нет, только `CORS_ALLOW_ORIGINS`.

## Модель безопасности

Ссылка — bearer-секрет с коротким сроком жизни. Что гарантируется и что нет:

| Свойство | Как обеспечено |
| -------- | -------------- |
| Нельзя подделать ссылку или подставить чужой `client_user_id` / `trainer_id` | HMAC-SHA256 на `JWT_SECRET`; под подписью все три параметра и `exp` |
| Нельзя перепутать с аватар-ссылкой | Отдельный префикс домена `client_analytics:` |
| Утёкшая ссылка живёт недолго | TTL 30 минут, константа в коде |
| Сравнение подписи без timing-утечки | `hmac.Equal` |
| Разорванная связь с тренером или блокировка закрывают доступ | Проверка `trainer_clients` на каждом запросе, не при выдаче ссылки |

Что **не** гарантируется, принято заказчиком: в течение 30 минут ссылку можно
переслать или открыть повторно; отозвать конкретную ссылку нельзя (только
сменой `JWT_SECRET`). Ссылка оседает в истории чата и браузера; в сообщении
бота она спрятана в inline URL-кнопку, чтобы не светиться текстом.

## Ссылка

```text
<CLIENT_ANALYTICS_PAGE_URL>?client_user_id=<uuid>&trainer_id=<uuid>&exp=<unix>&sig=<hex>
```

- `CLIENT_ANALYTICS_PAGE_URL` — полный URL страницы фронта без query, например
  `https://app.example.com/stats`. Бот дописывает query. Пусто → фича выключена.
- `trainer_id` — `trainers.id` (не `user_id` тренера), как везде в Redis и БД.
- `exp` — unix-секунды, `now + 30m`.
- `sig = hex(HMAC-SHA256(JWT_SECRET, "client_analytics:" + client_user_id + ":" + trainer_id + ":" + exp))`.

Фронт не разбирает параметры: пересылает `location.search` на бэкенд как есть.

Код: `internal/analytics/client_link.go` — `BuildClientAnalyticsLink(pageURL,
secret string, clientUserID, trainerID uuid.UUID) string` и
`VerifyClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID,
exp int64, sig string) bool`. Пакет аналитики владеет данными, значит и
ссылкой. `Build` возвращает пустую строку, если `pageURL` или `secret` пусты.

## Бот

- Reply-клавиатура: «📊 Статистика» второй кнопкой в нижнем ряду рядом с
  «❓ Помощь». Кнопка есть только при заданном `CLIENT_ANALYTICS_PAGE_URL`:
  `mainMenuKeyboard()` получает флаг (метод бота или параметр), все вызовы
  проходят через него.
- Обработчик: два уже существующих метода сервиса `trainerclient` —
  `ClientUserIDByTelegram(ctx, telegramUserID)` и
  `GetTelegramActiveTrainer(ctx, telegramUserID)` (возвращает `TrainerID` и
  `DisplayName`; один тренер → он, 2+ → Redis, нет активного →
  `ErrActiveTrainerNotSet`). В интерфейс бота `trainerClient` добавляется
  `GetTelegramActiveTrainer`. Ошибки через `menuErrorText`:
  `ErrActiveTrainerNotSet` → «Выберите тренера в разделе «Тренеры».»,
  `ErrTelegramUserNotFound` (новая ветка) → «У вас пока нет тренеров. Откройте
  ссылку-приглашение от тренера.»
- Бот строит ссылку через интерфейс `ClientAnalyticsLinker` с одним методом
  `BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string`;
  реализация — тонкая обёртка в `internal/analytics`, связывание в `cmd/api`.
- Сообщение: `📊 Ваша статистика у тренера <имя>\n\nСсылка действует 30 минут.`
  плюс inline-клавиатура с одной URL-кнопкой «Открыть статистику». Reply-меню
  остаётся.
- Текст «Помощь» дополняется строкой про «📊 Статистика».

## Эндпоинт

`GET /client/analytics?client_user_id=&trainer_id=&exp=&sig=` — новый префикс
`/client/`, зеркально к `/trainer/`. Без JWT-middleware и без роли.

| Условие | Ответ |
| ------- | ----- |
| Любой параметр отсутствует или не парсится | 400 |
| Подпись не сходится или `exp` в прошлом | 401 |
| Пары `(trainer_id, client_user_id)` нет в `trainer_clients` или `status = blocked` | 404 |
| OK | 200 `ClientSelfAnalytics` |

Хендлер в `internal/analytics/handlers.go` рядом с тренерскими; монтируется в
существующем `Handlers.Mount`.

### Ответ `ClientSelfAnalytics`

```text
client              { client_user_id, display_name }
trainer             { trainer_id, display_name }
current_assignment  AnalyticsAssignment | null      — как у тренера
activity            AnalyticsActivity               — как у тренера
recent_completions  ClientCompletionItem[]          — как у тренера, ≤ 30, completed_at desc
expires_at          date-time                       — из exp, чтобы фронт показал «ссылка истекла» без запроса
```

Не отдаём `avatar_url` (подписан под тренерский маршрут), `status`,
`linked_at`, `last_active_at`, пагинацию.

### Сервис

- Из `Service.ClientAnalytics` выделяется `clientAnalyticsByTrainerID(ctx,
  trainerID, clientUserID)`; тренерский путь зовёт её после `trainerID(...)`,
  клиентский — напрямую. Поведение тренерского эндпоинта не меняется.
- Новый `Service.ClientSelfAnalytics(ctx, clientUserID, trainerID)`:
  заголовок через `store.ClientHeader` (нет строки → `ErrClientNotFound`;
  `status = blocked` → тоже `ErrClientNotFound`, проверка в сервисе), имя
  тренера — новый метод аналитического store `TrainerDisplayName(ctx,
  trainerID)` поверх уже существующего sqlc-запроса
  `GetTrainerUserDisplayName` (новый SQL не нужен), сводка через общую часть,
  лента через `store.ListClientCompletions` с `page=1, limit=30` без фильтра
  дат, комментарии через `commentsByCompletionIDs`.
- Новых таблиц и миграций нет. Redis не используется.

## Конфиг и окружение

- `internal/config`: `ClientAnalyticsPageURL` из `CLIENT_ANALYTICS_PAGE_URL`,
  необязательная. Валидация: если задана, должна быть абсолютным `http(s)` URL
  без query и fragment.
- `render.yaml` + Dashboard `sync: false`; `docs/environments.md` — строка в
  таблицу env.
- Origin страницы фронта должен быть в `CORS_ALLOW_ORIGINS`; отдельной
  настройки нет.

## Тесты

- `client_link_test.go`: build/verify круг; истёкший `exp`; порча каждого из
  `client_user_id`, `trainer_id`, `exp`, `sig`; подпись аватара с теми же
  параметрами не проходит; пустой секрет или URL → пустая ссылка.
- Хендлер с фейковым сервисом: 400 / 401 / 404 / 200; в 200 есть
  `expires_at` и не больше 30 тренировок.
- Сервис: по образцу `service_internal_test.go`; `blocked` → `ErrClientNotFound`;
  тренерский `ClientAnalytics` после рефакторинга проходит прежние тесты.
- Бот (`bot_test.go`): кнопка есть при URL и нет без него; нажатие → сообщение
  с URL-кнопкой и валидной подписью; нет тренеров и нет активного — прежние
  тексты; `Помощь` содержит новую строку.
- Контракт: `api/openapi.yaml` (тег `analytics`, схема `ClientSelfAnalytics`),
  Postman, `internal/apicheck/schema.go`; `SKIP_SMOKE=1 ./postman/validate.sh`.

## Документация

- Новый `docs/features/client-analytics.md` (ссылка из `docs/README.md`):
  назначение, модель безопасности в две строки, правила ответа.
- `docs/status.md` — строка «Статистика клиента по ссылке из Telegram».
- `docs/features/telegram-bot.md` — кнопка в таблице меню, env.
- `docs/features/trainer-analytics.md` — «См. также» на новую доку.
- `docs/environments.md` — env.
- Новых Redis-ключей и таблиц нет, правила не меняются.

## Вне объёма

- Логин клиента в веб, refresh, сессии.
- Отзыв выданной ссылки.
- Telegram Mini App.
- Пагинация и фильтры ленты на клиентской странице.
- Аналитика по всем тренерам сразу.
