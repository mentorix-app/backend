#!/usr/bin/env bash
# Measure test coverage for internal packages (excludes internal/db/sqlc, internal/seed, cmd/api).
#
# Usage:
#   ./scripts/coverage.sh              report only
#   ./scripts/coverage.sh --min 85     fail if total coverage below threshold
#   ./scripts/coverage.sh --html       write coverage.html
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

min_pct=""
write_html=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --min)
      min_pct="${2:?--min requires a number}"
      shift 2
      ;;
    --min=*)
      min_pct="${1#--min=}"
      shift
      ;;
    --html)
      write_html=1
      shift
      ;;
    -h|--help)
      sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "Unknown option: $1 (try --help)" >&2
      exit 2
      ;;
  esac
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

packages=()
while IFS= read -r pkg; do
  packages+=("$pkg")
done < <(go list ./internal/... | grep -v '/sqlc$' | grep -v '/seed$')
if ((${#packages[@]} == 0)); then
  echo "No packages to measure" >&2
  exit 1
fi

coverpkg="$(IFS=,; echo "${packages[*]}")"
integration_coverpkg="mentorix-backend/internal/auth,mentorix-backend/internal/exercise,mentorix-backend/internal/program,mentorix-backend/internal/trainerclient,mentorix-backend/internal/db/pgconv,mentorix-backend/internal/health"

echo "=== Unit test coverage ==="
go test -count=1 -covermode=atomic -coverprofile="$tmp/unit.out" "${packages[@]}"

echo ""
echo "=== Store integration coverage ==="
if ! docker info >/dev/null 2>&1; then
  echo "WARN: Docker not available — skipping integration coverage merge" >&2
  cp "$tmp/unit.out" "$tmp/merged.out"
else
  go test -tags integration -count=1 -timeout 5m \
    -covermode=atomic \
    -coverpkg="$integration_coverpkg" \
    -coverprofile="$tmp/int.out" \
    ./internal/db/storetest/...
  go run github.com/wadey/gocovmerge@latest "$tmp/unit.out" "$tmp/int.out" >"$tmp/merged.out"
fi

echo ""
echo "=== Coverage by package ==="
go tool cover -func="$tmp/merged.out" | grep -E '^mentorix-backend/internal/' | grep -v '/sqlc/' || true

echo ""
total_line="$(go tool cover -func="$tmp/merged.out" | awk '/^total:/ {print $0}')"
echo "$total_line"

if [[ -n "$min_pct" ]]; then
  total="$(echo "$total_line" | awk '{gsub("%","",$3); print $3}')"
  awk -v t="$total" -v m="$min_pct" 'BEGIN {
    if (t+0 < m+0) { printf "FAIL: coverage %.1f%% is below minimum %.1f%%\n", t, m; exit 1 }
    printf "OK: coverage %.1f%% >= %.1f%%\n", t, m
  }'
fi

if ((write_html)); then
  go tool cover -html="$tmp/merged.out" -o coverage.html
  echo "Wrote coverage.html"
fi
