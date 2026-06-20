#!/usr/bin/env bash
set -euo pipefail

command="${1:-up}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ ! -f .env ]]; then
  echo "Missing .env. Copy .env.example to .env and set local DATABASE_URL first." >&2
  exit 1
fi

database_url="$(grep -E '^DATABASE_URL=' .env | head -n1 | cut -d= -f2- | tr -d '\r' | xargs)"
if [[ -z "$database_url" ]]; then
  echo "DATABASE_URL is not set in .env" >&2
  exit 1
fi

if ! command -v migrate >/dev/null 2>&1; then
  echo "migrate CLI not found. Install with:" >&2
  echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.2" >&2
  exit 1
fi

echo "Running migrations ($command)..."
migrate -path "db/migrations" -database "$database_url" "$command"
