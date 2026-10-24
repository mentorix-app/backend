#!/usr/bin/env bash
# Render pre-deploy: apply golang-migrate migrations against DATABASE_URL, then refresh the
# stored version fingerprints. Fail closed on migrate: a dirty or failed migrate aborts the deploy.
# The refresh is best effort: the migrations are already committed, and an aborted deploy would
# leave the old binary serving against the new schema. A failed refresh only warns; rerun it.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

MIGRATE_VERSION="${MIGRATE_VERSION:-v4.18.2}"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required for migrations" >&2
  exit 1
fi

export PATH="$(go env GOPATH)/bin:$PATH"

if ! command -v migrate >/dev/null 2>&1; then
  echo "Installing migrate ${MIGRATE_VERSION}..."
  go install -tags 'postgres' "github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}"
fi

echo "Running migrations..."
migrate -path db/migrations -database "$DATABASE_URL" up
echo "Migrations OK"

refresh_fingerprints() {
  if [[ -x ./bin/refresh-fingerprints ]]; then
    ./bin/refresh-fingerprints
  else
    go run ./cmd/refresh-fingerprints
  fi
}

echo "Refreshing version fingerprints..."
if refresh_fingerprints; then
  echo "Version fingerprints OK"
else
  echo "WARNING: version fingerprint refresh failed; continuing the deploy. Published programs may show unpublished changes until it is rerun (go run ./cmd/refresh-fingerprints)." >&2
fi
