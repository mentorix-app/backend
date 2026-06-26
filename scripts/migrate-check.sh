#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ ! -f .env ]]; then
  echo "Missing .env. Copy .env.example to .env first." >&2
  exit 1
fi

database_url="$(grep -E '^DATABASE_URL=' .env | head -n1 | cut -d= -f2- | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
if [[ -z "$database_url" ]]; then
  echo "DATABASE_URL is not set in .env" >&2
  exit 1
fi

if ! command -v migrate >/dev/null 2>&1; then
  echo "migrate CLI not found. Install with:" >&2
  echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.2" >&2
  exit 1
fi

expected=0
for f in db/migrations/*.up.sql; do
  [[ -f "$f" ]] || continue
  num="$(basename "$f" | sed -E 's/^([0-9]+).*/\1/')"
  num=$((10#$num))
  if (( num > expected )); then
    expected=$num
  fi
done

echo "=== Migration check ==="

if ! version_out="$(migrate -path db/migrations -database "$database_url" version 2>&1)"; then
  echo "FAIL: cannot read migration version (is Postgres up? DATABASE_URL correct?)" >&2
  echo "$version_out" >&2
  exit 1
fi

echo "  migrate: $version_out"

if echo "$version_out" | grep -qi 'dirty'; then
  echo "FAIL: database is in dirty state — resolve before running migrate up" >&2
  exit 1
fi

current=0
if [[ "$version_out" =~ ^([0-9]+) ]]; then
  current=$((10#${BASH_REMATCH[1]}))
fi

echo "  DB version: $current"
echo "  Expected:   $expected"

if (( current < expected )); then
  echo "FAIL: migrations behind — run: make migrate" >&2
  exit 1
fi

if (( current > expected )); then
  echo "FAIL: DB version ($current) is ahead of repo ($expected)" >&2
  exit 1
fi

echo "Status: OK (up to date)"
