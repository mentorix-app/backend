# Health

**Status:** implemented  
**Code:** `internal/health/`

## Purpose

Liveness and readiness for orchestration and monitoring.

## Database

None.

## API

Contract: `api/openapi.yaml` (paths `/health`, `/health/ready`).

Rules that are not obvious:

- `/health` — always ok; `/health/ready` — database/redis `ok` or `skipped` if URL empty.

## See also

- [environments.md](../environments.md) — local verification
