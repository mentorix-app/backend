# Auth

**Статус:** реализовано  
**Код:** `internal/auth/`

## Назначение

Регистрация и вход email+пароль, JWT access, opaque refresh в HttpOnly cookie, роли через `user_roles`.

## БД

`users`, `auth_identities`, `auth_refresh_sessions`, `user_roles`.

## API

Контракт: `api/openapi.yaml` (тег Auth).

Неочевидные правила:

- Access в JSON; refresh только cookie; фронт — `credentials: 'include'`.
- Rate limit login/register/refresh по IP при `REDIS_URL`.
- Пароли: argon2id; JWT HS256.
- `POST /auth/logout-all` и `GET /auth/me` — Bearer JWT.

## См. также

- [product.md](../product.md) — единый аккаунт
- [environments.md](../environments.md) — env и cookie
