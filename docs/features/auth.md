# Auth

**Status:** implemented  
**Code:** `internal/auth/`

## Purpose

Email and password registration and login, Google and Apple sign-in for the mobile app, JWT access token, opaque refresh token (HttpOnly cookie for the web cabinet, JSON body for the app), roles via `user_roles`.

## Database

`users`, `auth_identities`, `auth_refresh_sessions`, `user_roles`.

## API

Contract: `api/openapi.yaml` (Auth tag).

Key details:

- Access token in JSON. Web cabinet: refresh token only in the cookie; frontend uses `credentials: 'include'`.
- `POST /auth/google` takes a Google ID token. The first sign-in creates a user with the `client` role and an identity `(google, sub)`; later sign-ins return the same user. Name comes from the token (trimmed, cut to 100 characters). Email is stored only when Google marks it verified, and accounts are never joined by email, so an unverified or missing email leaves `email` empty in the response. Answers: `400` bad body or empty `id_token`, `401` token rejected, `429`, `503` when `GOOGLE_CLIENT_IDS` is empty. It uses the login rate limit.
- The ID token must be signed by Google, issued by `https://accounts.google.com`, unexpired, and addressed to one of the ids in `GOOGLE_CLIENT_IDS` (comma separated). Google's signing keys are fetched with a 5 second timeout. Any failure answers `401`, including a Google outage.
- `POST /auth/apple` works the same way with an Apple ID token (issuer `https://appleid.apple.com`) and an identity `(apple, sub)`. `APPLE_CLIENT_IDS` (comma separated) holds the iOS bundle id and, for Android, the Services ID. Apple does not put the name in the token, so the app sends it as the optional `name` in the request body. It is trimmed, cut to 100 characters without an error, used only when the account is created, and ignored for an existing account; a name claim in the token wins over it. Apple sends `email_verified` as a boolean or as a string, and both are accepted. Answers as for Google, with `503` when `APPLE_CLIENT_IDS` is empty.
- Google and Apple sign-in return `refresh_token` in the JSON body and set no cookie. `POST /auth/refresh` and `POST /auth/logout` accept `{"refresh_token": "..."}` as a JSON body: refresh then returns the rotated token in the body and logout revokes it, and neither sets or clears a cookie. Without a token in the body (no body, no JSON content type, empty value) both work as before with the cookie, and the response has no `refresh_token`. A body that cannot be read counts as no token and takes the cookie path.
- Rate limiting on login/register/refresh by IP when `REDIS_URL` is set. Refresh uses the login limit values (`AUTH_LOGIN_RATE_MAX`, `AUTH_LOGIN_RATE_WINDOW_SEC`) with its own counter key. If Redis is unavailable, requests are allowed and a warning is logged; sign-in does not depend on Redis.
- Limits: display name 100 characters (register, `PATCH /auth/me`), email 254 characters; counted in runes, over the limit answers `400`.
- Unknown paths answer `404`, except under `/programs`, `/exercises`, `/trainer` and `/admin`: those groups carry the JWT middleware on their prefix, so an unknown path there answers `401` without a token. In `/auth`, only the `/auth/me` and `/auth/logout-all` routes require a token.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all`, `GET /auth/me` and `PATCH /auth/me` (changes the display name) require Bearer JWT.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
