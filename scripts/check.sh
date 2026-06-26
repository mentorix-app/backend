#!/usr/bin/env bash
# Run all local quality checks (CI steps + migrate-check, integration tests, API smoke).
#
# Usage:
#   ./scripts/check.sh              full suite (default)
#   ./scripts/check.sh --ci         CI parity only (no migrate-check, integration, smoke)
#   ./scripts/check.sh --no-smoke   skip live API smoke (API need not be running)
#   ./scripts/check.sh --no-integration
#   ./scripts/check.sh --no-migrate-check
#
# Prerequisites (full suite):
#   - Go toolchain, .env with DATABASE_URL (migrate-check)
#   - Docker (Postgres for migrate-check; Testcontainers for integration)
#   - API on http://localhost:8080 for smoke (or use --no-smoke)
#   - sqlc and golangci-lint on PATH, or installed automatically via go install / go run
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

ci_only=0
skip_smoke=0
skip_integration=0
skip_migrate_check=0

for arg in "$@"; do
  case "$arg" in
    --ci) ci_only=1 ;;
    --no-smoke) skip_smoke=1 ;;
    --no-integration) skip_integration=1 ;;
    --no-migrate-check) skip_migrate_check=1 ;;
    -h|--help)
      sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "Unknown option: $arg (try --help)" >&2
      exit 2
      ;;
  esac
done

if (( ci_only )); then
  skip_smoke=1
  skip_integration=1
  skip_migrate_check=1
fi

export PATH="$(go env GOPATH)/bin:$PATH"

SQLC_VERSION=v1.29.0
LINT_VERSION=v1.62.2

failed=0

step() {
  echo ""
  echo "=== $1 ==="
}

ok() {
  echo "OK"
}

fail() {
  echo "FAIL: $1" >&2
  failed=1
}

ensure_sqlc() {
  if ! command -v sqlc >/dev/null 2>&1; then
    echo "Installing sqlc ${SQLC_VERSION}..."
    go install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}"
  fi
}

run_lint() {
  if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run ./...
  else
    echo "Running golangci-lint ${LINT_VERSION} via go run..."
    go run "github.com/golangci/golangci-lint/cmd/golangci-lint@${LINT_VERSION}" run ./...
  fi
}

step "gofmt"
if unformatted=$(gofmt -l .); [[ -n "$unformatted" ]]; then
  echo "$unformatted"
  fail "gofmt"
else
  ok
fi

step "go vet"
if go vet ./...; then ok; else fail "go vet"; fi

step "go test"
if go test ./... -count=1; then ok; else fail "go test"; fi

step "go build"
if go build -o /dev/null ./...; then ok; else fail "go build"; fi

step "sqlc generate (up to date)"
ensure_sqlc
if sqlc generate && git diff --exit-code internal/db/sqlc/; then
  ok
else
  fail "sqlc generate (up to date)"
fi

step "go generate internal/db (up to date)"
if go generate ./internal/db/... && git diff --exit-code internal/db/sqlc/; then
  ok
else
  fail "go generate internal/db (up to date)"
fi

step "golangci-lint"
if run_lint; then ok; else fail "golangci-lint"; fi

step "contract validation (postman + apicheck)"
if (( skip_smoke )); then
  if SKIP_SMOKE=1 ./postman/validate.sh; then ok; else fail "contract validation"; fi
else
  if ./postman/validate.sh; then ok; else fail "contract validation"; fi
fi

if (( ! skip_migrate_check )); then
  step "migrate-check"
  if ./scripts/migrate-check.sh; then ok; else fail "migrate-check"; fi
fi

if (( ! skip_integration )); then
  step "store integration tests"
  if go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1; then
    ok
  else
    fail "store integration tests"
  fi
fi

echo ""
if (( failed )); then
  echo "Some checks failed." >&2
  exit 1
fi

echo "All checks passed."
