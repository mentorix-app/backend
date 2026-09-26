# Auth

**Status:** implemented  
**Code:** `internal/auth/`

## Purpose

Email and password registration and login, JWT access token, opaque refresh in HttpOnly cookie, roles via `user_roles`.

## Database

`users`, `auth_identities`, `auth_refresh_sessions`, `user_roles`.

## API

Contract: `api/openapi.yaml` (Auth tag).

Key details:

- Access token in JSON; refresh only in cookie; frontend uses `credentials: 'include'`.
- Rate limiting on login/register/refresh by IP when `REDIS_URL` is set.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all` and `GET /auth/me` require Bearer JWT.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
