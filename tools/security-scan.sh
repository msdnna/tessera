#!/usr/bin/env bash
# SAST/SCA scan runner (#2936). One entry point for every component so the
# make targets, CI and the nightly agent all invoke the same thing.
#
# Usage: ./tools/security-scan.sh <backend|frontend|secrets|all>
#
# Every tool writes SARIF into reports/security/<tool>.sarif (gitignored, like
# the rest of reports/) and a human summary to stdout. SARIF because it is the
# one format all four tools already speak, and what GitHub's code-scanning UI
# ingests — no bespoke parsing on the CI side.
#
# Knobs:
#   SCAN_FAIL_ON=none|any   what makes this script exit non-zero (default: none)
#   SCAN_ALLOW_MISSING=1    a missing tool warns instead of failing
#
# On the default SCAN_FAIL_ON=none the script reports findings and exits 0. That
# is deliberate for now: the tools have never run against this tree, so a hard
# gate would paint develop red on day one. The ratchet that makes *new* findings
# fail is #2939 — until then this is a visibility step, and the summary below is
# the thing to read.
#
# A tool that is not installed is an ERROR, not a silent skip: a scan that
# quietly scans nothing and exits 0 is worse than no scan (same trap as the
# backend e2e tier that skipped itself green). `make scan-tools` installs them.
set -uo pipefail

TARGET="${1:?usage: security-scan.sh <backend|frontend|secrets|all>}"

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT="$ROOT/reports/security"
mkdir -p "$OUT"

# tools/bin first: that is where install-scanners.sh puts the pinned versions,
# and they must win over whatever a developer happens to have in GOPATH/bin.
# GOPATH/bin still trails it — govulncheck has no release binary and may already
# live there — and it is appended because make does not always pass it through.
GOPATH_BIN="$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin"
export PATH="$ROOT/tools/bin:$PATH:$GOPATH_BIN"

FAIL_ON="${SCAN_FAIL_ON:-none}"
ALLOW_MISSING="${SCAN_ALLOW_MISSING:-0}"

missing=0   # tools that could not be run
findings=0  # total results across every SARIF produced

say() { printf '\n\033[1m── %s\033[0m\n' "$*"; }

# have <tool> — true if runnable; otherwise records the gap and explains it.
have() {
  if command -v "$1" >/dev/null 2>&1; then
    return 0
  fi
  if [ "$ALLOW_MISSING" = "1" ]; then
    echo "  ! $1 not installed — skipped (SCAN_ALLOW_MISSING=1)" >&2
  else
    echo "  ✗ $1 not installed — run \`make scan-tools\`" >&2
    missing=$((missing + 1))
  fi
  return 1
}

# summarise <label> <sarif> — count results by level and add them to the total.
# The helper prints "<total>\t<human breakdown>" so one python call serves both
# the running total and the line the reader actually looks at.
summarise() {
  local label="$1" file="$2"
  if [ ! -s "$file" ]; then
    echo "    ! $label: no report written — see $OUT/$label.log" >&2
    missing=$((missing + 1))
    return
  fi
  local line n text
  line=$(python3 - "$file" <<'PY'
import collections, json, sys

try:
    with open(sys.argv[1]) as fh:
        sarif = json.load(fh)
except (OSError, json.JSONDecodeError) as exc:
    print(f"-1\tunreadable report ({exc})")
    sys.exit(0)

# SARIF puts the severity on the *rule*, and a result only overrides it with its
# own `level` when it differs — so an absent level means "whatever the rule says",
# not "warning". Resolve through the rule index before counting.
levels = collections.Counter()
total = 0
for run in sarif.get("runs", []):
    rules = {}
    driver = run.get("tool", {}).get("driver", {})
    for rule in driver.get("rules", []):
        rid = rule.get("id")
        if rid:
            rules[rid] = rule.get("defaultConfiguration", {}).get("level", "warning")
    for res in run.get("results", []):
        total += 1
        level = res.get("level") or rules.get(res.get("ruleId"), "warning")
        levels[level] += 1

if total:
    breakdown = ", ".join(f"{lvl}={cnt}" for lvl, cnt in sorted(levels.items()))
    print(f"{total}\t{total} finding(s): {breakdown}")
else:
    print("0\tclean")
PY
)
  n=${line%%	*}
  text=${line#*	}
  if [ "$n" = "-1" ]; then
    # A report we cannot parse is a broken scan, not a clean one.
    echo "    ! $label: $text" >&2
    missing=$((missing + 1))
    return
  fi
  findings=$((findings + n))
  echo "    $label: $text  → $(realpath --relative-to="$ROOT" "$file")"
}

scan_backend() {
  say "backend — SAST/SCA (Go)"

  # govulncheck: SCA with reachability. Unlike a plain lockfile scan it reports
  # only vulnerabilities whose vulnerable symbol is actually called, so its
  # findings are all actionable.
  if have govulncheck; then
    echo "  · govulncheck"
    (cd "$ROOT/backend" && govulncheck -format sarif ./... > "$OUT/govulncheck.sarif" 2>"$OUT/govulncheck.log") || true
    summarise "govulncheck" "$OUT/govulncheck.sarif"
  fi

  # gosec: the SAST half — hardcoded credentials, SQL string building, weak
  # crypto, unhandled errors on security-relevant calls.
  # -exclude-dir: generated sqlc output is not hand-edited, and the e2e suite
  # legitimately does things (fixed test credentials, http on localhost) that
  # would be findings in production code.
  if have gosec; then
    echo "  · gosec"
    (cd "$ROOT/backend" && gosec -quiet -fmt sarif -out "$OUT/gosec.sarif" \
      -exclude-dir=internal/db -exclude-dir=e2e ./... >"$OUT/gosec.log" 2>&1) || true
    summarise "gosec" "$OUT/gosec.sarif"
  fi
}

scan_frontend() {
  say "frontend — SCA (yarn.lock)"

  # osv-scanner over the lockfile: one binary that also covers Gradle and
  # Cargo.lock, which is why it was picked over an ecosystem-specific tool (#2938
  # reuses it for android/desktop).
  if have osv-scanner; then
    echo "  · osv-scanner (frontend/yarn.lock)"
    (cd "$ROOT" && osv-scanner scan source --format sarif --lockfile frontend/yarn.lock \
      > "$OUT/osv-frontend.sarif" 2>"$OUT/osv-frontend.log") || true
    summarise "osv-scanner" "$OUT/osv-frontend.sarif"
  fi

  # No local SAST for TS/Vue: semgrep ships via pip/docker only and neither is
  # reachable from this host. That half is covered by CodeQL in CI (see
  # .github/workflows/codeql.yml), which is the stronger analyser anyway.
  echo "  · SAST for TS/Vue runs in CI via CodeQL (semgrep is not installable here)"
}

scan_secrets() {
  say "secrets — gitleaks"

  # Working tree only (`dir`), not history: history rewriting is not on the
  # table, so a hit in an old commit is a rotate-the-credential task, not
  # something a pre-commit gate can act on. A one-off history audit is
  # `gitleaks git .` by hand — deliberately not a make target, so nobody wires
  # a minutes-long full-history walk into a routine scan.
  if have gitleaks; then
    echo "  · gitleaks (working tree)"
    # --config explicitly, not by auto-discovery: gitleaks only picks
    # .gitleaks.toml up implicitly for some scan modes, and a silently unused
    # config would put twelve build-artefact false positives back in the report.
    (cd "$ROOT" && gitleaks dir . --config .gitleaks.toml \
      --report-format sarif --report-path "$OUT/gitleaks.sarif" \
      --no-banner >"$OUT/gitleaks.log" 2>&1) || true
    summarise "gitleaks" "$OUT/gitleaks.sarif"
  fi
}

case "$TARGET" in
  backend) scan_backend ;;
  frontend) scan_frontend ;;
  secrets) scan_secrets ;;
  all)
    scan_backend
    scan_frontend
    scan_secrets
    ;;
  *)
    echo "Unknown target: $TARGET (expected backend|frontend|secrets|all)" >&2
    exit 2
    ;;
esac

say "summary"
echo "  reports: $OUT"
echo "  findings: $findings"
[ "$missing" -gt 0 ] && echo "  tools missing: $missing (run \`make scan-tools\`)"

if [ "$missing" -gt 0 ]; then
  exit 1
fi
if [ "$FAIL_ON" = "any" ] && [ "$findings" -gt 0 ]; then
  echo "  SCAN_FAIL_ON=any and there are findings → failing" >&2
  exit 1
fi
exit 0
