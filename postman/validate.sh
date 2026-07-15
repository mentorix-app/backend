#!/usr/bin/env bash
# Validates Postman collection and OpenAPI against live Go route registration.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

collection="postman/mentorix-backend.postman_collection.json"

echo "=== JSON syntax ==="
python3 -m json.tool "$collection" > /dev/null
python3 -m json.tool postman/mentorix-local.postman_environment.json > /dev/null
python3 -m json.tool postman/mentorix-render-stage.postman_environment.json > /dev/null
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
for f in ["postman/mentorix-local.postman_environment.json", "postman/mentorix-render-stage.postman_environment.json"]:
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
  ./scripts/smoke.sh
fi

echo ""
echo "Validation complete."
