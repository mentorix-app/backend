# Go code

Optional general style: [Google Go Style](https://google.github.io/styleguide/go/) (not project canon).

## Layout

- `cmd/<name>/` — wiring only; inject deps, no business logic.
- `internal/<feature>/` — handler → service → sqlc store.
- `internal/db/sqlc/` — generated; do not hand-edit.
- `pkg/` — only if actually needed externally.

## Quality

- Handle all errors; wrap with `fmt.Errorf("context: %w", err)`.
- Pass `context.Context` into DB and external calls.
- No `panic` in business logic.
- Inject dependencies; no globals for app state.
- Validate at HTTP boundary.
- Secrets in env only.
- Comments: English, non-obvious only; concise GoDoc on exports when needed.

## Performance & concurrency

Use Go's concurrency when work is independent and I/O-bound; keep goroutines bounded. Simplest correct solution first — no concurrency by default.

### When to parallelize

- Independent I/O: DB reads, HTTP, Redis, files.
- Embarrassingly parallel transforms without shared mutable state.
- Fan-out inside a request or job — with a **fixed upper bound**.

### Patterns

| Pattern | Use when |
| ------- | -------- |
| Worker pool | Many similar jobs; N workers + job `chan`, stop on `ctx.Done()` |
| `golang.org/x/sync/errgroup` | Fan-out with first-error cancel and `ctx` propagation |
| Bounded semaphore | Limit concurrency without full pool (`chan struct{}` capacity N) |
| `sync.WaitGroup` | Wait-for-all when error coordination is manual |

### Safety & limits

- Propagate `context.Context` from HTTP handler / caller; stop on cancel.
- No unbounded `go func()` per request.
- DB parallelism must respect **pgx pool** size.
- Document concurrent-safety on exported APIs (mutators are not safe unless stated).
- Tests: no `t.Fatal` from child goroutines.

### Speed

- Prefer sqlc batch queries / single round-trips over N+1.
- Preallocate slices when length is known (`make([]T, 0, n)`).
- `sync.Pool` only after profiling shows allocation pressure.
- Use `go test -bench` / `pprof` before hand-tuning.

### Design & review

- For multi-step flows: can independent steps run in parallel with a bounded pool?
- Prefer in-process worker pool over Redis queue / new service when work fits the monolith.
- Extract shared pool helper to `internal/...` only after **2+ call sites**.

## Naming (Go / JSON / enums)

| Layer | Rule | Example |
| ----- | ---- | ------- |
| Go exported | PascalCase | `NameRu`, `ExerciseType` |
| Enum const | type + snake value | `ExerciseTypeStrength = "strength"` |
| JSON | explicit `json:"snake_case"` | `access_token` |
| Domain types | not `Type` | `ExerciseType`, `ProgramStatus` |

Never rely on default JSON field names.

## Tests

- Table-driven for business rules.
- Handler tests with fake store/service.
- Integration: `go test -tags integration ./internal/db/storetest/...`
- Contract: `internal/apicheck`, `postman/validate.sh`

## Tooling

`gofmt`, `goimports`, `go vet`, golangci-lint. After SQL changes: `schema-sync`, `sqlc generate`.
