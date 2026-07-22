# Workout comments

**Статус:** реализовано  
**Код:** `internal/workoutcomment/`, чтение в `internal/analytics/`, push в `internal/telegramnotify/`

## Назначение

Тренер отвечает на результат тренировки клиента (чат в вебе). Пока **один ответ
на результат**; повторная попытка → 409. Расширение до мини-чата — удалить
UNIQUE-ограничение и проверку в сервисе, контракт API не меняется.

## БД

- `client_workout_completion_comments` — ответ тренера
  (`client_workout_completion_id` FK CASCADE, `trainer_id`, `comment_text`,
  UNIQUE по completion).

Миграция: `000025_workout_completion_comments`.

## API

- `POST /trainer/clients/{client_user_id}/completions/{completion_id}/comments`
  (`createTrainerCompletionComment`) — тело `{"text"}` (1–2000 символов), 201 →
  `{id, text, created_at}`. Completion вне пары тренер/клиент → 404; ответ уже
  есть → 409.
- Ответы возвращаются в `items[].comments` ленты
  `GET /trainer/clients/{client_user_id}/completions` (oldest first, одним
  запросом на страницу).

## Telegram

После сохранения — best-effort push клиенту: имя тренера, программа
(`program_name_ru`), неделя/день, цитата `result_text` (до 500 символов) и текст
ответа. Нет Telegram-привязки — тихий skip.

## См. также

- [workout-completions.md](workout-completions.md)
- [trainer-analytics.md](trainer-analytics.md)
