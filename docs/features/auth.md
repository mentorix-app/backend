# Auth

**Status:** implemented  
**Code:** `internal/auth/`

## Purpose

Email and password registration and login, JWT access token, opaque refresh token delivered in an HttpOnly cookie or in the JSON body, roles via `user_roles`.

## Database

`users`, `auth_identities`, `auth_refresh_sessions`, `user_roles`.

## API

Contract: `api/openapi.yaml` (Auth tag).

Key details:

- Access token in JSON. The refresh token has two delivery modes, see below.
- Rate limiting on login/register/refresh/logout by IP when `REDIS_URL` is set.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all` and `GET /auth/me` require Bearer JWT.

## Refresh token delivery

- Cookie (default, web cabinet): the refresh token is set as an HttpOnly cookie and the frontend uses `credentials: 'include'`. JSON responses carry no `refresh_token`.
- Body (mobile app): `register` and `login` accept `token_delivery: "body"` and return `refresh_token` in JSON with no cookie. The app sends it back as `{"refresh_token": "..."}` to `refresh` and `logout`. An unknown `token_delivery` value is a `400`.
- `refresh` takes the token from the body first and from the cookie otherwise. A body token is answered in JSON and leaves the cookie untouched, even when a cookie is also sent.
- An unreadable body, or a blank `refresh_token`, is ignored and the cookie is used.
- `logout` revokes the body token and, when a cookie is sent, the cookie session too, and then clears the cookie. With only a body token it sets no cookie.
- A `refresh` or `logout` request that acts on the cookie and carries an `Origin` header outside `CORS_ALLOW_ORIGINS` gets `403` and changes nothing, and it does not count against the rate limit. Entries are matched exactly as the browser sends the origin: scheme, host and port, with no wildcard patterns and no default port. The check is skipped when there is no `Origin` header and for requests that carry only a body token.
- `CORS_ALLOW_ORIGINS` must be set whenever the refresh cookie is `SameSite=none`. With an empty list or a `*` entry the check is off, and the API logs a warning at startup.
- Token responses from `register`, `login` and `refresh` are sent with `Cache-Control: no-store`.
- A refresh whose token came from the cookie never returns `refresh_token` in JSON, so page scripts cannot read it.
- A refresh token is single-use. A mobile client must not run two refresh calls at once: the second call fails with `401`.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
