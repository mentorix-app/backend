#!/usr/bin/env bash
# Live API smoke: health, ready, optional auth + list endpoints.
#
# Usage:
#   ./scripts/smoke.sh
#   SMOKE_BASE_URL=https://mentorix-api-stage.onrender.com ./scripts/smoke.sh
#   SMOKE_EMAIL=… SMOKE_PASSWORD=… ./scripts/smoke.sh
#
# Env:
#   SMOKE_BASE_URL   default http://localhost:8080 (no trailing slash)
#   SMOKE_EMAIL / SMOKE_PASSWORD  optional; enables login + GET exercises/programs
#   SMOKE_REQUIRE=1  fail if API is unreachable (default: skip when local down)
#   SMOKE_WAIT_SECONDS  poll /health up to N seconds (default 0 = once)
set -euo pipefail

BASE="${SMOKE_BASE_URL:-http://localhost:8080}"
BASE="${BASE%/}"
require="${SMOKE_REQUIRE:-0}"
wait_seconds="${SMOKE_WAIT_SECONDS:-0}"

echo "=== Smoke (${BASE}) ==="

ready=0
deadline=$((SECONDS + wait_seconds))
while true; do
  if curl -sf "$BASE/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if (( SECONDS >= deadline )); then
    break
  fi
  sleep 5
done

if (( ! ready )); then
  if [[ "$require" == "1" ]]; then
    echo "FAIL: API not reachable at $BASE/health" >&2
    exit 1
  fi
  echo "  API not running on $BASE — skip smoke"
  exit 0
fi

echo "  GET /health OK"
curl -sf "$BASE/health/ready" | python3 -c "
import sys, json
d = json.load(sys.stdin)
assert d.get('status') in ('ready', 'not_ready', 'no_dependencies_configured')
print('  GET /health/ready OK:', d['status'])
"

if [[ -z "${SMOKE_EMAIL:-}" || -z "${SMOKE_PASSWORD:-}" ]]; then
  echo "  auth smoke skipped (set SMOKE_EMAIL and SMOKE_PASSWORD)"
  exit 0
fi

TOKEN=$(
  curl -sf -X POST "$BASE/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$SMOKE_EMAIL\",\"password\":\"$SMOKE_PASSWORD\"}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || true
)

if [[ -z "$TOKEN" ]]; then
  echo "FAIL: POST /auth/login failed — check SMOKE_EMAIL/SMOKE_PASSWORD" >&2
  exit 1
fi

echo "  POST /auth/login OK"
curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/auth/me" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print('  GET /auth/me OK: roles=', d.get('roles'))
"
curl -sf -H "Authorization: Bearer $TOKEN" \
  "$BASE/exercises?page=1&limit=20&sort_by=name&sort_order=asc" | python3 -c "
import sys, json
d = json.load(sys.stdin)
assert 'items' in d and 'pagination' in d
print('  GET /exercises OK: total=', d['pagination']['total'])
"
curl -sf -H "Authorization: Bearer $TOKEN" \
  "$BASE/programs?page=1&limit=20&sort_by=created_at&sort_order=desc" | python3 -c "
import sys, json
d = json.load(sys.stdin)
assert 'items' in d and 'pagination' in d
print('  GET /programs OK: total=', d['pagination']['total'])
"

echo "Smoke OK"
