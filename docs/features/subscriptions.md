# Subscriptions (trainer plans)

**Status:** implemented (no payments yet; stores will be added later)  
**Code:** `internal/subscription/`

## Purpose

Plans `free`, `advance`, and `elite` for trainers with limits on their own exercises, active programs, and active clients. Global (admin) exercises do not count toward limits.

## Plans

| Limit | free | advance | elite |
| ----- | ---- | ------- | ----- |
| Own exercises | 10 | 50 | ∞ |
| Active programs (draft + published) | 3 | 15 | ∞ |
| Active clients | 3 | 15 | 50 |

## Database

`trainer_plan_entitlements` table has `plan_code`, `source` (one of `admin`, `app_store`, `google_play`), `status`, and `valid_until` (NULL means unlimited). The effective plan is the highest active entitlement; without active entitlements, the plan defaults to `free`.

## Quotas and read-only

- Quota checks run in a transaction: `LockTrainer` (advisory-like `SELECT … FOR UPDATE` on `trainers`) followed by `CheckQuota`. Exceeding a limit returns `409` with `{error: quota_exceeded, resource, plan, limit, usage}`.
- `OpCreate` blocks when `usage >= limit`; `OpMutate` (read-only after downgrade) blocks when `usage > limit`.
- Always allowed: reads, deleting exercises, archiving/deleting programs, clearing assignments, blocking clients.
- Invites are not created when the client limit is full; accepting an invite in the bot rechecks the quota (error message is «нет свободных мест»).

## API

Contract: `api/openapi.yaml`.

- `GET /auth/me` includes `subscription` field with `plan`, `source`, `valid_until`, `limits`, `usage`, and `permissions`; returns `null` if the user has no trainer profile.
- `GET /plans` returns the plan catalog (any JWT).
- `PUT /admin/trainers/{user_id}/plan` `{plan}` or `DELETE …/plan` grants a permanent admin plan (`source=admin`); setting `plan=free` in a PUT is equivalent to revoke.

## Future (app stores)

Payments will only be in mobile apps. App Store and Google Play adapters will write entitlements with the same schema (`source`, `valid_until`); admin grants remain a reward tool. When stores are enabled, current trainers will be reset to `free` except for admin grants. Receipt and webhook endpoints do not exist yet.

## See also

- [exercises.md](exercises.md), [programs.md](programs.md), [trainer-clients.md](trainer-clients.md)
- [admin.md](admin.md) — admin rights
