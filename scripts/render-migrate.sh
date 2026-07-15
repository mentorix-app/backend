#!/usr/bin/env bash
# Render pre-deploy: apply golang-migrate migrations against DATABASE_URL.
# Fail closed — a dirty or failed migrate aborts the deploy.
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
