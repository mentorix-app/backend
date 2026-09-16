#!/usr/bin/env bash
set -euo pipefail

command="${1:-up}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# shellcheck source=scripts/lib/env.sh
source "$root/scripts/lib/env.sh"
require_env_files

database_url="$(env_value DATABASE_URL)"
if [[ -z "$database_url" ]]; then
  echo "DATABASE_URL is not set (shell or .env)" >&2
  exit 1
fi

if ! command -v migrate >/dev/null 2>&1; then
  echo "migrate CLI not found. Install with:" >&2
  echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.2" >&2
  exit 1
fi

echo "Running migrations ($command)..."
migrate -path "db/migrations" -database "$database_url" "$command"
