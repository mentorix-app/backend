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
- Rate limiting on login/register/refresh by IP when `REDIS_URL` is set. If Redis is unavailable, requests are allowed and a warning is logged; sign-in does not depend on Redis.
- Limits: display name 100 characters (register, `PATCH /auth/me`), email 254 characters; counted in runes, over the limit answers `400`.
- Unknown paths answer `404`, except under `/programs`, `/exercises`, `/trainer` and `/admin`: those groups carry the JWT middleware on their prefix, so an unknown path there answers `401` without a token. In `/auth`, only the `/auth/me` and `/auth/logout-all` routes require a token.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all` and `GET /auth/me` require Bearer JWT.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
