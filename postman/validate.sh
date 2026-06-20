#!/usr/bin/env bash
# Validates Postman collection against Go route definitions.
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
echo "=== Expected routes from Go ==="
routes=(
  "GET /health"
  "GET /health/ready"
  "POST /auth/register"
  "POST /auth/login"
  "POST /auth/refresh"
  "GET /auth/me"
  "POST /auth/logout"
  "POST /auth/logout-all"
  "POST /admin/users/:user_id/roles/admin"
  "GET /exercises"
  "GET /exercises/:id"
  "POST /exercises"
  "PUT /exercises/:id"
  "DELETE /exercises/:id"
  "DELETE /exercises/all"
)
for r in "${routes[@]}"; do echo "  $r"; done

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
echo "=== OpenAPI paths (api/openapi.yaml) ==="
if [ -f api/openapi.yaml ]; then
  python3 <<'PY'
import re, sys
spec_paths = set()
with open("api/openapi.yaml") as f:
    for line in f:
        m = re.match(r"^  (/[^:]+):", line)
        if m:
            spec_paths.add(m.group(1))
expected = {
    "/health", "/health/ready",
    "/auth/register", "/auth/login", "/auth/refresh", "/auth/logout", "/auth/logout-all", "/auth/me",
    "/admin/users/{user_id}/roles/admin",
    "/exercises", "/exercises/{id}", "/exercises/all",
}
missing = expected - spec_paths
extra = spec_paths - expected
for p in sorted(spec_paths):
    print(f"  {p}")
if missing:
    print("FAIL: missing in OpenAPI:", sorted(missing))
    sys.exit(1)
if extra:
    print("WARN: extra OpenAPI paths:", sorted(extra))
print("OpenAPI path coverage: OK")
PY
else
  echo "  api/openapi.yaml not found — skip"
fi

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
  TOKEN=$(curl -sf -X POST "$BASE/auth/login" -H "Content-Type: application/json" -d '{"email":"trainer@test.com","password":"password123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || true)
  if [ -n "$TOKEN" ]; then
    echo "  POST /auth/login OK"
    curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/auth/me" | python3 -c "import sys,json; d=json.load(sys.stdin); print('  GET /auth/me OK: roles=', d.get('roles'))"
    curl -sf -H "Authorization: Bearer $TOKEN" "$BASE/exercises?page=1&limit=20&sort_by=name&sort_order=asc" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'items' in d and 'pagination' in d; print('  GET /exercises OK: total=', d['pagination']['total'])"
  else
    echo "  POST /auth/login skipped (user may not exist — run Register first)"
  fi
else
  echo "  API not running on $BASE — skip smoke"
fi
fi

echo ""
echo "Validation complete."
