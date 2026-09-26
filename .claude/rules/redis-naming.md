# Redis naming

Canon: prefix `const` in the package that owns the data.

## Key pattern

```text
mentorix:{domain}:{purpose}:{identifier}
```

| Segment | Rule | Example |
| ------- | ---- | ------- |
| `mentorix` | required global namespace | `mentorix:` |
| `{domain}` | feature area, `snake_case` | `rl`, `telegram` |
| `{purpose}` | what is stored | `active_trainer`, `login` |
| `{identifier}` | dynamic suffix | IP, `telegram_user_id` |

Rate limits add an action segment: `mentorix:rl:{action}:ip:{ip}` where `{action}` ∈ `login`, `register`, `refresh`.

## Values

| Use | Redis type | Stored value | DB alignment |
| --- | ---------- | ------------ | ------------ |
| Rate limit | string (`INCR`) | counter | — |
| Active trainer | string (`SET`/`GET`) | `trainers.id` UUID | `trainer_id`, not `trainer_user_id` |

Prefer plain string keys over hashes unless one identity needs multiple fields.

## TTL

- Rate-limit windows expire: set `EXPIRE` on first increment.
- Session-like state has no TTL (`0`): overwrite it on update or clear it explicitly (e.g. active trainer: `Delete` on stale read).

## Adding a key

1. Use a new `{domain}` segment; do not piggyback on another feature's prefix.
2. `const …KeyPrefix` in the owning package.
3. Add a row to the Current keys table below.
4. Feature docs describe *what* is stored, not the full naming spec ([docs-and-rules-maintenance.md](docs-and-rules-maintenance.md)).

## Current keys

| Key | Code | Value |
| --- | ---- | ----- |
| `mentorix:rl:login:ip:{ip}` | `internal/auth/rate_limit.go` | INCR + EXPIRE |
| `mentorix:rl:register:ip:{ip}` | `internal/auth/rate_limit.go` | INCR + EXPIRE |
| `mentorix:rl:refresh:ip:{ip}` | `internal/auth/rate_limit.go` | INCR + EXPIRE |
| `mentorix:telegram:active_trainer:{telegram_user_id}` | `internal/trainerclient/active_trainer.go` | `trainers.id` UUID |
| `mentorix:telegram:workout_pending:{telegram_user_id}` | `internal/workoutcompletion/pending.go` | JSON pending completion (TTL 30m) |

## Avoid

- Keys without `mentorix:` prefix
- `trainer_user_id` in values (use `trainers.id`)
- Generic segments: `cache`, `data`, `tmp`
- Hash fields mirroring SQL columns unless multiple fields per entity
