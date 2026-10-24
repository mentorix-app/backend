# Environments

1. **Local** — Postgres + Redis, `make run` or `make dev`; see [Env](#env) for env files.
2. **Stage** — `develop` → CI → Deploy Hook → `mentorix-api-stage`.
3. **Prod** — later (`main` + separate services); not yet in [`render.yaml`](../render.yaml).

Do not commit secrets.

## Names (stage)

| Component | Stage |
| --------- | ----- |
| Web | `mentorix-api-stage` → `https://mentorix-api-stage.onrender.com` |
| Postgres / Redis | `mentorix-db-stage` / `mentorix-redis-stage` |
| Frontend | `https://dashboard-u7fz.onrender.com` — origin in `CORS_ALLOW_ORIGINS`, base for `CLIENT_ANALYTICS_PAGE_URL` |

Plans: web **starter**, Postgres **basic-256mb** (PG 16), Redis **starter**, region **frankfurt**.

## Release flow (current)

```text
feature/* → PR → develop → CI → Deploy stage → smoke
```

- Required check: `check` job (`make check-ci`).
- Migrations: [`scripts/render-migrate.sh`](../scripts/render-migrate.sh) in `preDeployCommand`.
  After `migrate up` it runs `cmd/refresh-fingerprints`, which rewrites each stored program version fingerprint to match its stored tree, so a release that changes the fingerprint does not flag published programs as changed. The step is idempotent and non-fatal: it stops after 2 minutes, and a failure prints a warning and lets the deploy go on, because the migrations are already applied. `migrate up` itself fails closed.
- The deploy job calls the Render deploy hook with `ref` set to the commit that passed CI, so stage always gets that exact commit. A manual run of the workflow deploys the head of the branch it was started from.
- Post-deploy smoke ([`scripts/smoke.sh`](../scripts/smoke.sh)) polls `GET /health` for up to `SMOKE_WAIT_SECONDS` and passes only when its `commit` equals the commit under deploy (`SMOKE_EXPECT_COMMIT`, set by the workflows). A failed build or pre-deploy migration leaves the previous instance serving the old commit, so the smoke job fails on timeout and names the expected and last seen commit. After that it checks only public answers and needs no account: `GET /health/ready`, `401 missing_authorization` from `GET /auth/me` without a token, and `401 invalid_id_token` from `POST /auth/google` with a junk token (stage fails the run when Google sign-in is not configured; a local run skips that check).
- `commit` in `/health` is `RENDER_GIT_COMMIT`, which Render sets at runtime. It is omitted when the variable is empty (local runs) and is not part of `/health/ready`.
- Local: pre-commit runs `make check-quick`; before push run `make check`.
- Workflow [`deploy-prod.yml`](../.github/workflows/deploy-prod.yml) — manual only (`workflow_dispatch`), no prod on Render yet.

## GitHub setup (one-time)

**Branch protection** (`develop`): require PR; required status `check`.

| Environment | Secrets |
| --- | --- |
| `staging` | `RENDER_DEPLOY_HOOK_STAGE` |

Deploy Hook: Render → `mentorix-api-stage` → Settings → Deploy Hook.

## Render Blueprint

1. Push [`render.yaml`](../render.yaml) (stage only) → Dashboard Blueprint sync.
2. Set `sync: false` secrets on stage API; connect Deploy Hook to Environment `staging`.
3. Smoke test: `GET https://mentorix-api-stage.onrender.com/health`.

If `mentorix-api` / `mentorix-db` / `mentorix-redis` already appear in Dashboard, delete them manually (Blueprint sync will not remove them).

### Runbook

| Problem | Action |
| --- | --- |
| migrate dirty | Check Render logs → fix / careful `migrate force` → redeploy |
| `migrate down` of 000033 fails | It cannot run while `client_workout_completions` has rows with `source = 'app'`. It rolls back and leaves the database dirty at 32 with the schema still at 33; recovery is `migrate force 33`. Do not run the down on stage once the app has marks |
| bad stage | fix-forward to `develop` or Rollback in Dashboard |

## Env

Local: single `.env` file, priority `shell > .env` (loaded in `internal/config.LoadDotenv`, scripts via `scripts/lib/env.sh`). Variables already set in shell are never overridden.

| File | In git | Content |
| --- | --- | --- |
| `.env.example` | yes | template for `.env`; `make setup` copies it if `.env` does not exist |
| `.env` | no | all local: ports, localhost Postgres/Redis, CORS, TTL, `JWT_SECRET`, `BOT_*` |
| `.env.stage` | no | only external `DATABASE_URL` (`sslmode=require`) and `REDIS_URL` (`rediss://`) for stage, for `make psql-stage` / `make redis-stage`; API does not read it |

Stage: files not used, all from Render.

| Source | Destination |
| --- | --- |
| `render.yaml` | stage links + cookie/proxy + `BOT_WEBHOOK_URL`; `ipAllowList` for DB and Key Value open to `0.0.0.0/0` for local tools |
| Dashboard `sync: false` | `JWT_SECRET`, `BOT_*`, `BFF_SHARED_SECRET`, `TELEGRAM_BOT_USERNAME`, `CORS_ALLOW_ORIGINS`, `TRAINER_INVITE_TTL_DAYS`, `CLIENT_ANALYTICS_PAGE_URL` |

Variables added to `render.yaml` with `sync: false` do not appear in Dashboard automatically — set them manually (Environment → Add).

`GOOGLE_CLIENT_IDS`, `APPLE_CLIENT_IDS`, `APPLE_ANDROID_PACKAGE`, `APPLE_TEAM_ID`, `APPLE_KEY_ID` and `APPLE_PRIVATE_KEY` are not in `render.yaml`. Add them in the Dashboard once the values exist; until the client ids are set `POST /auth/google` and `POST /auth/apple` answer `503`, and without `APPLE_ANDROID_PACKAGE` so does `POST /auth/apple/callback`.

The last three sign the Apple client secret for the revocation in `DELETE /auth/me`. Set all three or none; a partial set stops the start. `APPLE_TEAM_ID` and `APPLE_KEY_ID` are 10 characters of `A-Z` and `0-9`. `APPLE_PRIVATE_KEY` is the text of the `.p8` file (PKCS#8, curve P-256); it may be stored on one line with `\n` for the line breaks. The key goes into the Dashboard only, never into the repository or a chat, and no error text repeats any part of it. Without them an account with an Apple sign-in answers `503` on `DELETE /auth/me`; other accounts are deleted as usual.

`TRUSTED_PROXY_CIDRS` decides which address counts as the client's: rate limit keys and `remote_ip` in the request log use it, unless the dashboard's server forwards the browser address with the shared secret (see `BFF_SHARED_SECRET` below). It is a comma-separated list of CIDRs, single IPs and the keywords `private`, `loopback` and `cloudflare`; empty means the TCP peer. With CIDRs or `private`, the server walks `X-Forwarded-For` from the right and takes the first untrusted hop. `cloudflare` adds one step: when that hop is a Cloudflare edge address (ranges copied into `internal/http/cloudflare.go`, with the date), the client is the `CF-Connecting-IP` header, which Cloudflare sets itself. A missing or invalid header, or a hop outside the list, keeps the hop. Stage sits behind Cloudflare and Render's proxy, so it uses `private,cloudflare`. Cloudflare ranges are not added to the walk because a Cloudflare Worker reaches the origin from them with an `X-Forwarded-For` of its own choosing.

`BFF_SHARED_SECRET` lets the dashboard's server forward the browser address. When it is empty, both headers are ignored. When it is set (at least 32 bytes, otherwise startup stops), a request whose `X-Mentorix-BFF-Secret` header equals it and whose `X-Mentorix-Client-IP` header is an IP address is counted by that address in the auth rate limits (Google, Apple, Telegram link and refresh). The zone is dropped from the address, and an IPv4-mapped IPv6 address is read as IPv4. The request log adds `client_ip` next to `remote_ip`. A forwarded address that arrives with a missing or wrong secret, or with the right secret and an address that does not parse or is `0.0.0.0`, `::` or a multicast address, is ignored: the request goes on with the address from `TRUSTED_PROXY_CIDRS`, and the API logs one warning, `bff client ip rejected`, with `reason` (`secret_mismatch` or `invalid_address`) and `request_id`. The secret and the address are never logged. The same value must be set on the dashboard's server. Keep it out of chats, tickets and the repository.

Stage uses the default `AUTH_LOGIN_RATE_MAX=20`. The dashboard's server sends both headers, so each browser address gets its own login and refresh counters.

Datastore CLI: `make psql` / `make redis` (local), `make psql-stage` / `make redis-stage` (Render; `ARGS="-c 'select 1'"`).
For `.env.stage` values: Dashboard → instance → External URL, or `render pg get` / `render kv get … --include-sensitive-connection-info`.

Frontend local to stage API: base URL `https://mentorix-api-stage.onrender.com`, `credentials: 'include'`; origin `http://localhost:3000` must be in stage `CORS_ALLOW_ORIGINS`.

Integration: `TEST_DATABASE_URL` → `mentorix_test` (in `.env`). Frontend: access JSON, refresh in HttpOnly cookie, `credentials: 'include'`.
