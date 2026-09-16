#!/usr/bin/env bash
# Open psql or redis-cli against the local or the Render stage datastore.
#
#   scripts/db-cli.sh psql  local|stage [psql args…]
#   scripts/db-cli.sh redis local|stage [redis-cli args…]
#
# local: DATABASE_URL / REDIS_URL with the usual precedence (shell > .env).
# stage: the same keys read only from .env.stage; the API never loads that file,
#        so tooling is its only reader and a local run cannot hit the stage database.
set -euo pipefail

tool="${1:-}"
target="${2:-}"
if [[ -z "$tool" || -z "$target" ]]; then
  echo "usage: $0 psql|redis local|stage [args…]" >&2
  exit 2
fi
shift 2

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
# shellcheck source=scripts/lib/env.sh
source "$root/scripts/lib/env.sh"

case "$tool" in
  psql) key=DATABASE_URL ;;
  redis) key=REDIS_URL ;;
  *) echo "unknown tool '$tool' (psql|redis)" >&2; exit 2 ;;
esac

case "$target" in
  local)
    require_env_files
    url="$(env_value "$key")"
    ;;
  stage)
    if [[ ! -f .env.stage ]]; then
      echo "Missing .env.stage — see docs/environments.md (External URLs from the Render Dashboard)." >&2
      exit 1
    fi
    url="$(env_file_value .env.stage "$key")"
    ;;
  *) echo "unknown target '$target' (local|stage)" >&2; exit 2 ;;
esac

if [[ -z "$url" ]]; then
  echo "$key is not set for target '$target'" >&2
  exit 1
fi

case "$tool" in
  psql)
    exec psql "$url" "$@"
    ;;
  redis)
    # External Render Key Value URLs are rediss://; redis-cli needs --tls for them.
    if [[ "$url" == rediss://* ]]; then
      exec redis-cli --tls -u "$url" "$@"
    fi
    exec redis-cli -u "$url" "$@"
    ;;
esac
