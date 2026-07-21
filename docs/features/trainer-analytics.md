# Trainer analytics

**Статус:** реализовано  
**Код:** `internal/analytics/`

## Назначение

Два экрана аналитики для тренера: по клиенту (карточка, прогресс текущей программы, активность, лента тренировок) и по программам (список с агрегатами + детальный разбор одной программы). Только чтение; источник — журнал `client_workout_completions` и активные `program_assignments`.

## БД

`client_workout_completions`, `program_assignments`, `program_versions` + `program_version_week_days`/`_blocks` (знаменатель прогресса), `trainer_clients`, `programs`. Запросы: `db/queries/analytics.sql`. Новых таблиц/миграций нет.

## API

Контракт: `api/openapi.yaml` — `GET /trainer/clients/{client_user_id}/analytics`, `GET /trainer/clients/{client_user_id}/completions`, `GET /trainer/programs/analytics`, `GET /trainer/programs/{program_id}/analytics`. Все — только роль `trainer` (admin → 403).

Доступ: клиент должен быть в `trainer_clients` тренера, программа — `created_by` тренера; иначе `404`.

Неочевидные правила:

- **Прогресс текущей программы** — по `completion_cycle_id` назначения против дней назначенной замороженной версии. Cycle переживает sync версии, поэтому прогресс охватывает все версии одного назначения; при смене `program_id` цикл новый.
- **Тренировочные дни** (знаменатель) — дни версии, где есть хотя бы один блок. Выполненным считается день версии, чей `day_key` есть в журнале цикла; выполненные дни, удалённые из текущей версии, в процент не входят. `completion_percent` ограничен 100, один знак после запятой.
- **Активность клиента** (`activity`) — по всем циклам и программам у данного тренера (журнал не удаляется). `by_program` — суммы за всё время по каждой программе, имя из снапшота журнала (удалённые программы показываются, `program_id: null`).
- **`week_streak`** — подряд идущие календарные недели (Пн–Вс, UTC) с ≥1 тренировкой; текущая незавершённая неделя без тренировки серию не рвёт.
- **Лента** (`/completions`) — пагинация + `from`/`to` (RFC3339 или `YYYY-MM-DD`, `to` эксклюзивно), сортировка `completed_at` desc; `is_current_cycle` — принадлежность текущему назначению. `items[].comments` — ответы тренера ([workout-comments.md](workout-comments.md)).
- **Список программ** — все неудалённые программы тренера (включая черновики); `total_completions` по `program_id` журнала (все версии/циклы); `avg_completion_percent` — среднее по активным назначенцам, `null` без клиентов. Сортировка `sort_by=name|last_activity` (default `last_activity` desc).
- **Деталь программы** — `clients` (активные назначенцы, прогресс текущего цикла, без пагинации — ограничено квотой клиентов), `weeks` — сдачи текущих циклов по `week_number` (drop-off). `avatar_url` — как в `GET /trainer/clients`.

## См. также

- [workout-completions.md](workout-completions.md) — журнал и правила cycle/day_key
- [trainer-clients.md](trainer-clients.md) — список клиентов и назначения
- [programs.md](programs.md) — версии и sync
