#!/usr/bin/env bash
# Validate docs/ and project .cursor/rules for size, links, and migration version sync.
#
# Usage: ./scripts/docs-check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

failed=0

fail() {
  echo "FAIL: $1" >&2
  failed=1
}

latest_migration() {
  local expected=0
  local f num
  for f in db/migrations/*.up.sql; do
    [[ -f "$f" ]] || continue
    num="$(basename "$f" | sed -E 's/^0*([0-9]+).*/\1/')"
    num=$((10#$num))
    if (( num > expected )); then
      expected=$num
    fi
  done
  echo "$expected"
}

echo "=== docs-check ==="

expected_mig="$(latest_migration)"
echo "Latest migration in repo: $expected_mig"

for f in docs/architecture.md docs/status.md; do
  if [[ ! -f "$f" ]]; then
    fail "missing $f"
    continue
  fi
  if ! grep -qE "(версия|version).*\*\*${expected_mig}\*\*|версия ${expected_mig}|version ${expected_mig}" "$f"; then
    fail "$f does not mention migration version $expected_mig (update after new migration)"
  fi
done

echo ""
echo "=== File size limits ==="

check_lines() {
  local file="$1"
  local max="$2"
  local label="$3"
  [[ -f "$file" ]] || return 0
  local n
  n="$(wc -l < "$file" | tr -d ' ')"
  if (( n > max )); then
    fail "$label $file has $n lines (max $max)"
  fi
}

for f in docs/*.md docs/features/*.md; do
  [[ -f "$f" ]] || continue
  if [[ "$f" == docs/features/* ]]; then
    check_lines "$f" 150 "feature-doc"
  else
    check_lines "$f" 100 "doc"
  fi
done

for f in .cursor/rules/*.mdc; do
  [[ -f "$f" ]] || continue
  check_lines "$f" 120 "rule"
done

check_lines ".cursor/rules/index.mdc" 40 "index rule"

echo ""
echo "=== Markdown links (docs/) ==="

python3 <<'PY' || failed=1
import re
import sys
from pathlib import Path

root = Path(".")
docs = root / "docs"
link_re = re.compile(r"\]\(([^)]+)\)")

errors = []
for md in sorted(docs.rglob("*.md")):
    text = md.read_text(encoding="utf-8")
    for raw in link_re.findall(text):
        target = raw.split("#", 1)[0].strip()
        if not target or target.startswith(("http://", "https://", "mailto:")):
            continue
        if target.startswith("/"):
            resolved = root / target.lstrip("/")
        else:
            resolved = (md.parent / target).resolve()
        if not resolved.exists():
            errors.append(f"{md}: broken link -> {target}")

if errors:
    for e in errors:
        print(f"FAIL: {e}", file=sys.stderr)
    sys.exit(1)
print("OK")
PY

echo ""
echo "=== Legacy references ==="

legacy_hits="$(rg -n 'AGENTS\.md|mentorix\.mdc' --glob '!scripts/docs-check.sh' . 2>/dev/null | rg -v 'old AGENTS\.md pattern' || true)"
if [[ -n "$legacy_hits" ]]; then
  echo "$legacy_hits" >&2
  fail "stale references to AGENTS.md or mentorix.mdc"
else
  echo "OK"
fi

echo ""
echo "=== Feature docs linked from docs/README.md ==="

python3 <<'PY' || failed=1
import re
import sys
from pathlib import Path

readme = Path("docs/README.md")
if not readme.exists():
    sys.exit(0)

text = readme.read_text(encoding="utf-8")
features = re.findall(r"features/([a-z0-9_-]+\.md)", text)
errors = []
for name in features:
    path = Path("docs/features") / name
    if not path.is_file():
        errors.append(f"docs/README.md links missing {path}")

if errors:
    for e in errors:
        print(f"FAIL: {e}", file=sys.stderr)
    sys.exit(1)
print("OK")
PY

echo ""
if (( failed )); then
  echo "docs-check failed." >&2
  exit 1
fi

echo "docs-check passed."
