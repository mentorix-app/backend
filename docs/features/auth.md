# Auth

**Status:** implemented  
**Code:** `internal/auth/`

## Purpose

Email and password registration and login, Google and Apple sign-in for the mobile app, linking a Telegram to an app account, JWT access token, opaque refresh token (HttpOnly cookie for the web cabinet, JSON body for the app), roles via `user_roles`.

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
- **Telegram link.** `POST /auth/telegram/link` (Bearer JWT, login rate limit, body `{code}`) joins a client who signed in with Google or Apple to the Telegram they already use. The client opens the bot with `/start link`; the bot answers with a one-time code (8 characters, valid 10 minutes, key in [redis-naming.md](../../.claude/rules/redis-naming.md)), and the client types it into the app. The direction is deliberate: the secret travels from the Telegram owner to the app, so nobody can push their own sign-in onto someone else's account. The code is read and deleted in one step, so it works once, and it is spent even when the link is then refused. Case and surrounding spaces are ignored. Outcomes, decided in one transaction:
  - The Telegram has no account yet: it is attached to the caller's account. The answer carries new tokens for the same user. Two parallel requests for the same account and Telegram both succeed.
  - The caller's account already holds this same Telegram: nothing changes, the answer still carries new tokens.
  - The Telegram already has an account and the caller's account is empty (no trainer link, no role besides `client`, no sign-in besides Google and Apple): the caller's Google and Apple identities move to the Telegram account, its email is filled from the caller's only when it has none, it gets the `client` role if missing, and the caller's user is deleted with its refresh sessions. The tokens in the answer belong to the Telegram account, so the app replaces its own.
  - The Telegram has an account and the caller's account is not empty: `409 telegram is linked to an account that cannot be merged`, nothing changes. The same answer is given when the Telegram account is an admin (an admin cannot hold the `client` role), or when the database refuses to delete the caller's user because other rows still reference it; the whole change is rolled back. Two accounts with data are resolved by hand.
  - The caller's account holds a different Telegram: `409 account already has another telegram`.
- Telegram link answers: `200` tokens in the body (`refresh_token` included, no cookie), `400 invalid or expired code` (empty, unknown, expired or already used), `401` no token, `404` the caller's account is gone, `409` as above, `429`, `503 telegram linking is unavailable` when Redis is not configured or down. Unlinking and moving a Telegram between accounts are not supported.
- `GET /auth/me` and `PATCH /auth/me` return `telegram_linked` (always present): true when the account has a Telegram sign-in, so the app can ask "already training through Telegram?" right after the first sign-in.
- Rate limiting on login/register/refresh by IP when `REDIS_URL` is set. Refresh uses the login limit values (`AUTH_LOGIN_RATE_MAX`, `AUTH_LOGIN_RATE_WINDOW_SEC`) with its own counter key. If Redis is unavailable, requests are allowed and a warning is logged; sign-in does not depend on Redis.
- Limits: display name 100 characters (register, `PATCH /auth/me`), email 254 characters; counted in runes, over the limit answers `400`.
- Unknown paths answer `404`, except under `/programs`, `/exercises`, `/trainer` and `/admin`: those groups carry the JWT middleware on their prefix, so an unknown path there answers `401` without a token. In `/auth`, only `/auth/me`, `/auth/logout-all` and `/auth/telegram/link` require a token.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all`, `GET /auth/me` and `PATCH /auth/me` (changes the display name) require Bearer JWT.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
