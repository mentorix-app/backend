#!/usr/bin/env bash
# Live API smoke over public checks only: health, readiness, a protected route
# without a token, and Google sign-in with a junk token. It needs no credentials.
#
# Usage:
#   ./scripts/smoke.sh
#   SMOKE_BASE_URL=https://mentorix-api-stage.onrender.com ./scripts/smoke.sh
#
# Env:
#   SMOKE_BASE_URL   default http://localhost:8080 (no trailing slash)
#   SMOKE_REQUIRE=1  fail if API is unreachable or Google sign-in is not configured
#                    (default: skip both when running locally)
#   SMOKE_WAIT_SECONDS  poll /health up to N seconds (default 0 = once)
#   SMOKE_EXPECT_COMMIT  git commit that /health must report; keep polling within
#                        SMOKE_WAIT_SECONDS until it matches, fail on timeout (default: any)
#
# Exit 0 with a line starting "  SKIP:" means the local API was down and smoke did not run.
set -euo pipefail

BASE="${SMOKE_BASE_URL:-http://localhost:8080}"
BASE="${BASE%/}"
require="${SMOKE_REQUIRE:-0}"
wait_seconds="${SMOKE_WAIT_SECONDS:-0}"

expect_commit="${SMOKE_EXPECT_COMMIT:-}"

echo "=== Smoke (${BASE}) ==="

ready=0
reachable=0
last_commit="(none)"
deadline=$((SECONDS + wait_seconds))
while true; do
  if body="$(curl -sf --max-time 10 "$BASE/health" 2>/dev/null)"; then
    reachable=1
    if [[ -z "$expect_commit" ]]; then
      ready=1
      break
    fi
    commit="$(printf '%s' "$body" | python3 -c "
import sys, json
try:
    print(json.load(sys.stdin).get('commit') or '')
except ValueError:
    print('')
" 2>/dev/null || true)"
    commit="${commit:-(none)}"
    if [[ "$commit" != "$last_commit" ]]; then
      echo "  /health commit: $commit"
      last_commit="$commit"
    fi
    if [[ "$commit" == "$expect_commit" ]]; then
      ready=1
      break
    fi
  fi
  if (( SECONDS >= deadline )); then
    break
  fi
  sleep 5
done

if (( reachable && ! ready )); then
  echo "FAIL: /health commit did not match within ${wait_seconds}s: expected $expect_commit, last seen $last_commit" >&2
  exit 1
fi

if (( ! ready )); then
  if [[ "$require" == "1" ]]; then
    echo "FAIL: API not reachable at $BASE/health" >&2
    exit 1
  fi
  echo "  SKIP: API not running on $BASE, smoke not run"
  exit 0
fi

echo "  GET /health OK"
curl -sf --max-time 10 "$BASE/health/ready" | python3 -c "
import sys, json
d = json.load(sys.stdin)
assert d.get('status') in ('ready', 'not_ready', 'no_dependencies_configured')
print('  GET /health/ready OK:', d['status'])
"

# expect_error METHOD PATH STATUS CODE [JSON_BODY]: the request must answer STATUS with
# error code CODE. Prints the status and code it got; returns 1 on a mismatch.
expect_error() {
  local method="$1" path="$2" want_status="$3" want_code="$4" body="${5:-}"
  local out status code
  local args=(-s --max-time 10 -o - -w '\n%{http_code}' -X "$method" "$BASE$path")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data-binary "$body")
  fi
  out="$(curl "${args[@]}" 2>/dev/null || true)"
  status="${out##*$'\n'}"
  # Only a plain snake_case code goes on: the value is echoed into the CI log.
  code="$(printf '%s' "${out%$'\n'*}" | python3 -c "
import sys, json, re
try:
    code = json.load(sys.stdin).get('code') or ''
except (ValueError, AttributeError):
    code = ''
print(code if isinstance(code, str) and re.fullmatch(r'[a-z0-9_]{1,64}', code) else '')
" 2>/dev/null || true)"
  [[ "$status" =~ ^[0-9]{3}$ ]] || status="(none)"
  last_status="$status"
  last_code="$code"
  [[ "$status" == "$want_status" && "$code" == "$want_code" ]]
}

if expect_error GET /auth/me 401 missing_authorization; then
  echo "  GET /auth/me without a token OK: 401 missing_authorization"
else
  echo "FAIL: GET /auth/me without a token answered $last_status ${last_code:-(no code)}, want 401 missing_authorization" >&2
  exit 1
fi

# A junk token never reaches Google: the verifier rejects it before any network call.
if expect_error POST /auth/google 401 invalid_id_token '{"id_token":"smoke-not-a-jwt"}'; then
  echo "  POST /auth/google with a junk token OK: 401 invalid_id_token"
elif [[ "$last_status" == "503" && "$last_code" == "provider_not_configured" && "$require" != "1" ]]; then
  echo "  SKIP: POST /auth/google check, Google sign-in is not configured on $BASE"
else
  echo "FAIL: POST /auth/google with a junk token answered $last_status ${last_code:-(no code)}, want 401 invalid_id_token" >&2
  exit 1
fi

echo "Smoke OK"
