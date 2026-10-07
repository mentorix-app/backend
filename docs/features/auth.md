# Auth

**Status:** implemented  
**Code:** `internal/auth/`

## Purpose

Email and password registration and login, sign-in with an Apple or Google ID token, JWT access token, opaque refresh token delivered in an HttpOnly cookie or in the JSON body, roles via `user_roles`.

## Database

`users`, `auth_identities`, `auth_refresh_sessions`, `user_roles`.

## API

Contract: `api/openapi.yaml` (Auth tag).

Key details:

- Access token in JSON. The refresh token has two delivery modes, see below.
- Rate limiting on login/register/refresh/logout by IP when `REDIS_URL` is set.
- Passwords: argon2id; JWT signed with HS256.
- `POST /auth/logout-all`, `GET /auth/me` and `POST /auth/me/roles` require Bearer JWT.
- `roles` in `GET /auth/me` is always an array, empty for an account that has not chosen a role. `email` in token and profile responses is an empty string when the account has no primary email.

## Apple and Google sign-in

- `POST /auth/social-login` takes the ID token the app got from the Apple or Google SDK. The API verifies the signature against the provider's keys, the issuer, the expiry, and that one of the token's audiences is in `APPLE_CLIENT_IDS` or `GOOGLE_CLIENT_IDS` (comma separated). A provider with an empty list is off and answers `400`.
- Provider keys are fetched on the first sign-in and cached, so the API starts even when a provider is unreachable. Each key request has a 5 second timeout. A key request that fails, times out, gets a non-200 answer, or gets a body that is cut off, over 1 MiB or not JSON fails that sign-in with `503` "sign-in provider unavailable" (the cause goes to the request log only). A token that does not verify is `401`.
- The account key is the token's `sub` stored in `auth_identities` (`provider` is `apple` or `google`). The email never selects an account.
- A known identity gets `200`. An unknown one creates a user with that identity and no roles, and gets `201`. Rate limited like login. Refresh token delivery works as for login.
- Email rule: `primary_email` is stored only when the provider marks the email verified (Apple may send it as the string `"true"`). If a verified email already belongs to another user, the response is `409` and nothing is created. An unverified or missing email is not compared and `primary_email` stays empty.
- The display name comes from the request `name`, then from the token's `name` claim. Apple sends the name to the app only once, so the app passes it in `name`. A request `name` over 100 characters, or one with a NUL character, is `400`; a longer token name is cut to 100, and a token name with a NUL character is ignored.
- One account per email, ignoring case, enforced by a unique index on `users.primary_email`. Two first sign-ins for one identity at the same moment produce one user. Two simultaneous first sign-ins with the same verified email through different providers end with one account and one `409`. `POST /auth/register` with an email that any account holds is `409`.
- Role choice: `POST /auth/me/roles` with `client` or `trainer` adds that role. `trainer` also creates the trainer profile, so `GET /auth/me` then shows the free plan. Repeating a call changes nothing. `admin` or any other value is `400`. An admin account gets `409`, because admin is exclusive with the other roles. A deleted user gets `404`.

## Refresh token delivery

- Cookie (default, web cabinet): the refresh token is set as an HttpOnly cookie and the frontend uses `credentials: 'include'`. JSON responses carry no `refresh_token`.
- Body (mobile app): `register`, `login` and `social-login` accept `token_delivery: "body"` and return `refresh_token` in JSON with no cookie. The app sends it back as `{"refresh_token": "..."}` to `refresh` and `logout`. An unknown `token_delivery` value is a `400`.
- `refresh` takes the token from the body first and from the cookie otherwise. A body token is answered in JSON and leaves the cookie untouched, even when a cookie is also sent.
- An unreadable body, or a blank `refresh_token`, is ignored and the cookie is used.
- `logout` revokes the body token and, when a cookie is sent, the cookie session too, and then clears the cookie. With only a body token it sets no cookie.
- A `refresh` or `logout` request that acts on the cookie and carries an `Origin` header outside `CORS_ALLOW_ORIGINS` gets `403` and changes nothing, and it does not count against the rate limit. Entries are matched exactly as the browser sends the origin: scheme, host and port, with no wildcard patterns and no default port. The check is skipped when there is no `Origin` header and for requests that carry only a body token.
- `CORS_ALLOW_ORIGINS` must be set whenever the refresh cookie is `SameSite=none`. With an empty list or a `*` entry the check is off, and the API logs a warning at startup.
- Token responses from `register`, `login`, `social-login` and `refresh` are sent with `Cache-Control: no-store`.
- A refresh whose token came from the cookie never returns `refresh_token` in JSON, so page scripts cannot read it.
- A refresh token is single-use. A mobile client must not run two refresh calls at once: the second call fails with `401`.

## See also

- [product.md](../product.md) — unified account
- [environments.md](../environments.md) — environment and cookie settings
