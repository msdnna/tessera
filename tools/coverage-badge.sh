#!/usr/bin/env bash
# Parse a component's coverage report and emit a shields.io "endpoint" JSON
# under _badges/<component>.json. The badges branch (orphan) hosts these files;
# README references them via
#   https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/<owner>/<repo>/badges/<component>.json
#
# Usage: ./tools/coverage-badge.sh <component> <coverage-source>
#   backend/mcp → Go coverprofile  (e.g. backend/coverage.out)
#   web         → LCov info file   (e.g. frontend/coverage/lcov.info)
#   android     → JaCoCo XML report
set -euo pipefail

COMPONENT="${1:?component name required (backend|web|android|mcp)}"
SOURCE="${2:?coverage source path required}"

if [ ! -f "$SOURCE" ]; then
  echo "::warning::coverage source not found: $SOURCE — skipping badge" >&2
  exit 0
fi

GO_BIN="${GO:-go}"

case "$COMPONENT" in
  backend | mcp)
    # `go tool cover -func` resolves package paths in the profile against the
    # nearest go.mod. Run from the profile's own directory so it picks up the
    # component's go.mod instead of falling back to GOROOT.
    ABS_SOURCE=$(realpath "$SOURCE")
    MOD_DIR=$(dirname "$ABS_SOURCE")
    PCT=$(cd "$MOD_DIR" && "$GO_BIN" tool cover -func="$(basename "$ABS_SOURCE")" \
      | awk '/^total:/ {sub(/%/,"",$NF); print $NF}')
    ;;
  web)
    # LCov totals: LH = lines hit, LF = lines found (across all files).
    PCT=$(awk -F: '/^LH:/ {h+=$2} /^LF:/ {f+=$2} END {if(f>0) printf "%.1f", 100*h/f; else print "0"}' "$SOURCE")
    ;;
  android)
    PCT=$(python3 - "$SOURCE" <<'PY'
import sys, xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
for c in root.findall('counter'):
    if c.get('type') == 'INSTRUCTION':
        m, cv = int(c.get('missed')), int(c.get('covered'))
        total = m + cv
        print(f"{100*cv/total:.1f}" if total else "0")
        break
PY
)
    ;;
  *)
    echo "Unknown component: $COMPONENT (expected backend|web|android|mcp)" >&2
    exit 1
    ;;
esac

PCT_INT="${PCT%.*}"
COLOR=$(awk -v p="$PCT_INT" 'BEGIN{
  print p>=80 ? "brightgreen" \
    : p>=60 ? "yellowgreen" \
    : p>=40 ? "yellow" \
    : p>=20 ? "orange" \
    : "red"
}')

mkdir -p _badges
printf '{"schemaVersion":1,"label":"coverage","message":"%s%%","color":"%s"}\n' \
  "$PCT" "$COLOR" > "_badges/${COMPONENT}.json"

echo "[$COMPONENT] coverage=${PCT}% color=$COLOR → _badges/${COMPONENT}.json"
