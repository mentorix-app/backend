#!/usr/bin/env bash
# Validates Postman collection and OpenAPI against live Go route registration.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

collection="postman/mentorix-backend.postman_collection.json"

echo "=== JSON syntax ==="
python3 -m json.tool "$collection" > /dev/null
python3 -m json.tool postman/mentorix-local.postman_environment.json > /dev/null
python3 -m json.tool postman/mentorix-render-dev.postman_environment.json > /dev/null
echo "OK"

echo ""
echo "=== URL format (must be strings, not objects) ==="
python3 <<'PY'
import json, sys
c = json.load(open("postman/mentorix-backend.postman_collection.json"))
bad = []

def walk(items, folder=""):
    for i in items:
        if "item" in i:
            walk(i["item"], i["name"])
        else:
            url = i["request"]["url"]
            if not isinstance(url, str):
                bad.append(f"{folder}/{i['name']}: url is {type(url).__name__}, not str")
            auth = i["request"].get("auth", {}).get("type", "MISSING")
            method = i["request"]["method"]
            print(f"  {method:6} auth={auth:8} url={url if isinstance(url,str) else url.get('raw')}")

walk(c["item"])
if bad:
    for b in bad: print("FAIL:", b)
    sys.exit(1)
print("All URLs are strings: OK")
PY

echo ""
echo "=== Contract alignment (Go routes, OpenAPI paths, Postman, schemas) ==="
go test ./internal/apicheck/... -count=1
echo "OK"

echo ""
echo "=== Environment base_url ==="
python3 <<'PY'
import json
for f in ["postman/mentorix-local.postman_environment.json", "postman/mentorix-render-dev.postman_environment.json"]:
    e = json.load(open(f))
    vals = {v["key"]: v["value"] for v in e["values"]}
    base = vals.get("base_url", "")
    ok = base and not base.endswith("/")
    print(f"  {e['name']}: {base} {'OK' if ok else 'FAIL (empty or trailing slash)'}")
PY

echo ""
if [ "${SKIP_SMOKE:-}" = "1" ]; then
  echo "=== Smoke test ==="
  echo "  skipped (SKIP_SMOKE=1)"
else
  echo "=== Smoke test (localhost, if API running) ==="
BASE="http://localhost:8080"
if curl -sf "$BASE/health" > /dev/null 2>&1; then
  echo "  GET /health OK"
  curl -sf "$BASE/health/ready" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d.get('status') in ('ready','not_ready','no_dependencies_configured'); print('  GET /health/ready OK:', d['status'])"
  SMOKE_EMAIL="${SMOKE_EMAIL:-mentorix.app@proton.me}"
  SMOKE_PASSWORD="${SMOKE_PASSWORD:-Password01\$}"
  if [ -z "$SMOKE_EMAIL" ] || [ -z "$SMOKE_PASSWORD" ]; then
    echo "  POST /auth/login skipped (set SMOKE_EMAIL and SMOKE_PASSWORD to run smoke auth)"
  else
  TOKEN=$(curl -sf -X POST "$BASE/auth/login" -H "Content-Type: application/json" -d "{\"email\":\"$SMOKE_EMAIL\",\"password\":\"$SMOKE_PASSWORD\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || true)
  if [ -n "$TOKEN" ]; then
    echo "  POST /auth/login OK"
    curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/auth/me" | python3 -c "import sys,json; d=json.load(sys.stdin); print('  GET /auth/me OK: roles=', d.get('roles'))"
    curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/exercises?page=1&limit=20&sort_by=name&sort_order=asc" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'items' in d and 'pagination' in d; print('  GET /exercises OK: total=', d['pagination']['total'])"
    curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/programs?page=1&limit=20&sort_by=created_at&sort_order=desc" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'items' in d and 'pagination' in d; print('  GET /programs OK: total=', d['pagination']['total'])"
  else
    echo "  POST /auth/login skipped (user $SMOKE_EMAIL not found or wrong password — register first or set SMOKE_EMAIL/SMOKE_PASSWORD)"
  fi
  fi
else
  echo "  API not running on $BASE — skip smoke"
fi
fi

echo ""
echo "Validation complete."
