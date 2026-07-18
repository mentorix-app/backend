# Subscriptions (тарифы тренеров)

**Статус:** реализовано (без оплат; сторы подключим позже)  
**Код:** `internal/subscription/`

## Назначение

Тарифы `free` / `advance` / `elite` для тренеров: лимиты на **свои** упражнения, активные программы и активных клиентов. Глобальные (админские) упражнения в лимиты не входят.

## Тарифы

| Лимит | free | advance | elite |
| ----- | ---- | ------- | ----- |
| Свои упражнения | 10 | 50 | ∞ |
| Активные программы (draft+published) | 3 | 15 | ∞ |
| Активные клиенты | 3 | 15 | 50 |

## БД

`trainer_plan_entitlements` (`plan_code`, `source` ∈ `admin|app_store|google_play`, `status`, `valid_until NULL` = бессрочно). Эффективный план — **максимальный** активный entitlement; без активных — `free`.

## Квоты и read-only

- Проверка в транзакции: `LockTrainer` (advisory-подобный `SELECT … FOR UPDATE` по `trainers`) + `CheckQuota`; превышение → `409` `{error: quota_exceeded, resource, plan, limit, usage}`.
- `OpCreate` — блок при `usage >= limit`; `OpMutate` (read-only после даунгрейда) — блок при `usage > limit`.
- Всегда разрешено: чтение, удаление упражнений, archive/delete программ, снятие назначения, блокировка клиента.
- Инвайт не создаётся при заполненном лимите клиентов; accept в боте повторно проверяет квоту (текст ошибки — «нет свободных мест»).

## API

Контракт: `api/openapi.yaml`.

- `GET /auth/me` — поле `subscription` (`plan`, `source`, `valid_until`, `limits`, `usage`, `permissions`); `null`, если нет профиля тренера.
- `GET /plans` — каталог тарифов (любой JWT).
- `PUT /admin/trainers/{user_id}/plan` `{plan}` / `DELETE …/plan` — admin-грант бессрочного тарифа (`source=admin`); `plan=free` в PUT эквивалентен revoke.

## Будущее (сторы)

Оплата только в мобильных приложениях: адаптеры App Store / Google Play будут писать те же entitlements (`source`, `valid_until`), admin-гранты остаются инструментом поощрения. При подключении сторов текущие тренеры сбрасываются на `free` (кроме admin-грантов). Receipt/webhook-эндпоинтов сейчас нет.

## См. также

- [exercises.md](exercises.md), [programs.md](programs.md), [trainer-clients.md](trainer-clients.md)
- [admin.md](admin.md) — admin-права
