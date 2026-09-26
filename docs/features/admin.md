# Admin

**Status:** implemented  
**Code:** `internal/admin/`

## Purpose

The `admin` role is separate from `trainer` and `client`. Admins manage the global exercise catalog and trainer plans; everything else is view-only.

## Permissions

- Exercises: CRUD **global** exercises; can view but not edit trainer private exercises.
- Programs, clients: view only (lists and details); mutations return `403`.
- Plans: grant and revoke permanent plan grants to trainers.

## Database

`user_roles` table with trigger `user_roles_admin_exclusive_trg` (migration `000024`): the `admin` role is incompatible with `trainer` or `client`.

## API

Contract: `api/openapi.yaml`.

- `PUT /admin/trainers/{user_id}/plan`, `DELETE /admin/trainers/{user_id}/plan` — grant or revoke a plan; see [subscriptions.md](subscriptions.md).
- No endpoint exists to grant the `admin` role (role is set only via database or migration).

## See also

- [auth.md](auth.md)
- [subscriptions.md](subscriptions.md)
