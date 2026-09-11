#!/usr/bin/env bash
# SAST/SCA scan runner (#2936). One entry point for every component so the
# make targets, CI and the nightly agent all invoke the same thing.
#
# Usage: ./tools/security-scan.sh <backend|frontend|secrets|android|desktop|all>
#
# Every tool writes SARIF into reports/security/<tool>.sarif (gitignored, like
# the rest of reports/) and a human summary to stdout. SARIF because it is the
# one format all four tools already speak, and what GitHub's code-scanning UI
# ingests — no bespoke parsing on the CI side.
#
# Knobs:
#   SCAN_FAIL_ON=none|any   what makes this script exit non-zero (default: none)
#   SCAN_ALLOW_MISSING=1    a missing tool warns instead of failing
#   SCAN_RATCHET=0          skip the baseline comparison (default: on)
#   SCAN_RATCHET_UPDATE=1   rewrite the baseline from this run instead
#   SCAN_RATCHET_DEPS=fail  make new dependency advisories fail too
#
# On the default SCAN_FAIL_ON=any-is-off the script does not fail on the mere
# existence of findings: the tree carries ~100 of them and triage is its own
# work, so a zero-findings gate would paint develop red on day one. What does
# fail is the ratchet (#2939): findings are frozen in tools/security-baseline.json
# and the list may shrink, never grow. See docs/security-scanning.md.
#
# A tool that is not installed is an ERROR, not a silent skip: a scan that
# quietly scans nothing and exits 0 is worse than no scan (same trap as the
# backend e2e tier that skipped itself green). `make scan-tools` installs them.
set -uo pipefail

TARGET="${1:?usage: security-scan.sh <backend|frontend|secrets|android|desktop|all>}"

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
produced=() # reports this run actually wrote — what the ratchet may compare
            # against. Scoping it to *this* run matters: reports/security keeps
            # the files of previous runs, and feeding the ratchet a stale
            # osv-android.sarif during a backend-only scan would compare the
            # android baseline against a report nobody regenerated.

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
  produced+=("$file")
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

# gradlew <task…> — the Android wrapper invoked the way the Makefile does it
# (local.env carries SDK/JDK paths and the optional SOCKS proxy). Duplicated here
# rather than shelling back into make so that CI and the agent can call this
# script directly.
gradlew() {
  (
    cd "$ROOT/android" || exit 1
    set -a
    # shellcheck disable=SC1091
    [ -f ./local.env ] && . ./local.env
    set +a
    ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}" \
    JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk-amd64}" \
    GRADLE_OPTS="${SOCKS_PROXY_HOST:+-DsocksProxyHost=$SOCKS_PROXY_HOST -DsocksProxyPort=$SOCKS_PROXY_PORT -DsocksProxyVersion=5} -Dorg.gradle.internal.http.socketTimeout=300000" \
    ./gradlew --no-daemon "$@"
  )
}

scan_android() {
  say "android — SAST (detekt) + SCA (Gradle dependencies)"

  # ── SAST: detekt with the SARIF reporter.
  # detekt already runs in `make lint-android`, but there it is a pass/fail gate on
  # style; here we want its findings in the same SARIF pile as everything else, so
  # the report is collected even when detekt exits non-zero (`|| true`) — a style
  # violation must not abort the security scan.
  echo "  · detekt (SARIF)"
  local detekt_sarif="$ROOT/android/app/build/reports/detekt/detekt.sarif"
  rm -f "$detekt_sarif"
  gradlew :app:detekt >"$OUT/detekt.log" 2>&1 || true
  if [ -s "$detekt_sarif" ]; then
    cp "$detekt_sarif" "$OUT/detekt.sarif"
    summarise "detekt" "$OUT/detekt.sarif"
  else
    echo "    ! detekt: no SARIF written — see $OUT/detekt.log" >&2
    missing=$((missing + 1))
  fi

  # ── SCA: osv-scanner over the Gradle dependency graph.
  # osv-scanner cannot read build.gradle — it needs a resolved list, and of the two
  # formats it understands (gradle.lockfile, gradle/verification-metadata.xml) only
  # the latter can be produced without switching dependency locking on for the build.
  #
  # The catch that dictates the rest of this function: Gradle writes that file to a
  # FIXED path inside the repo (android/gradle/verification-metadata.xml), and its
  # mere presence turns on dependency verification for every subsequent build — an
  # incomplete file then fails `make android` with checksum errors. So it is
  # generated, moved out to a scratch dir, and removed from the tree by a trap that
  # also fires on Ctrl-C. If the file already exists when we start, somebody
  # committed a real one: use it in place and generate nothing.
  if ! have osv-scanner; then
    return
  fi

  local verif="$ROOT/android/gradle/verification-metadata.xml"
  local scratch generated=0
  scratch=$(mktemp -d)
  mkdir -p "$scratch/gradle"

  if [ -f "$verif" ]; then
    cp "$verif" "$scratch/gradle/verification-metadata.xml"
    echo "  · osv-scanner (existing gradle/verification-metadata.xml)"
  else
    generated=1
    echo "  · generating the Gradle dependency list (resolves releaseRuntimeClasspath, takes a minute)"
    # shellcheck disable=SC2064
    trap "rm -f '$verif'" EXIT INT TERM
    gradlew --write-verification-metadata sha256 \
      :app:dependencies --configuration releaseRuntimeClasspath \
      >"$OUT/gradle-deps.log" 2>&1 || true
    if [ -s "$verif" ]; then
      cp "$verif" "$scratch/gradle/verification-metadata.xml"
    fi
    rm -f "$verif"
    trap - EXIT INT TERM
  fi

  if [ -s "$scratch/gradle/verification-metadata.xml" ]; then
    # Gradle records everything the *build* resolved, including its own plugin
    # classpath — unfiltered, all 94 findings of the first run were CVEs in AGP /
    # detekt / google-services tooling (netty, jdom2, jose4j) that never reaches
    # the APK. Narrow to the release runtime classpath so the report answers
    # "what do we ship". Only for a list this run generated: filtering a
    # committed metadata file against some earlier run's dependency report would
    # quietly drop entries that are on the classpath today.
    local kept
    if [ "$generated" = "1" ]; then
      kept=$(python3 "$ROOT/tools/gradle-deps-filter.py" \
        "$scratch/gradle/verification-metadata.xml" "$OUT/gradle-deps.log" \
        "$scratch/gradle/filtered.xml") || kept=""
      if [ -n "$kept" ] && [ -s "$scratch/gradle/filtered.xml" ]; then
        echo "  · shipped dependencies: ${kept% *} of ${kept#* } resolved artefacts (rest is build tooling)"
        mv "$scratch/gradle/filtered.xml" "$scratch/gradle/verification-metadata.xml"
      else
        # Better a noisy report than a silently empty one — but say which it is.
        echo "    ! could not narrow to the runtime classpath; scanning every resolved artefact" >&2
      fi
    fi
    echo "  · osv-scanner (Gradle dependencies)"
    (cd "$scratch" && osv-scanner scan source --format sarif \
      --lockfile gradle/verification-metadata.xml \
      > "$OUT/osv-android.sarif" 2>"$OUT/osv-android.log") || true
    summarise "osv-scanner (android)" "$OUT/osv-android.sarif"
  else
    echo "    ! osv-scanner (android): no dependency list — see $OUT/gradle-deps.log" >&2
    missing=$((missing + 1))
  fi
  rm -rf "$scratch"
}

scan_desktop() {
  say "desktop — SCA (Cargo.lock)"

  # cargo-audit is the ecosystem-native choice and was the one named in the task,
  # but it is not installable here the way everything else is: there is no release
  # binary, so it means `cargo install` compiling ~200 crates from crates.io on
  # every machine and in CI. osv-scanner is already pinned by install-scanners.sh
  # and serves the same RustSec advisories through OSV (RustSec is an OSV feed), so
  # Rust reuses the binary the frontend already uses. If the desktop app ever needs
  # the licence/ban side of cargo-deny, that is a different tool and a new decision.
  if have osv-scanner; then
    echo "  · osv-scanner (desktop/src-tauri/Cargo.lock)"
    (cd "$ROOT" && osv-scanner scan source --format sarif --lockfile desktop/src-tauri/Cargo.lock \
      > "$OUT/osv-desktop.sarif" 2>"$OUT/osv-desktop.log") || true
    summarise "osv-scanner (desktop)" "$OUT/osv-desktop.sarif"
  fi

  # SAST for Rust: clippy already runs in `make lint-desktop` with `-D warnings`,
  # i.e. it is a hard gate there and would add nothing as a report here.
  echo "  · SAST for Rust is the clippy gate in \`make lint-desktop\` (-D warnings)"
}

case "$TARGET" in
  backend) scan_backend ;;
  frontend) scan_frontend ;;
  secrets) scan_secrets ;;
  android) scan_android ;;
  desktop) scan_desktop ;;
  all)
    scan_backend
    scan_frontend
    scan_secrets
    scan_android
    scan_desktop
    ;;
  *)
    echo "Unknown target: $TARGET (expected backend|frontend|secrets|android|desktop|all)" >&2
    exit 2
    ;;
esac

say "summary"
echo "  reports: $OUT"
echo "  findings: $findings"
[ "$missing" -gt 0 ] && echo "  tools missing: $missing (run \`make scan-tools\`)"

if [ "$missing" -gt 0 ]; then
  # Stop before the ratchet: a partial run would read as "these findings are
  # gone" and invite somebody to bake the gap into the baseline.
  exit 1
fi

ratchet_rc=0
if [ "${SCAN_RATCHET:-1}" != "0" ] && [ "${#produced[@]}" -gt 0 ]; then
  if [ "${SCAN_RATCHET_UPDATE:-0}" = "1" ]; then
    say "baseline update"
    python3 "$ROOT/tools/security-ratchet.py" update "${produced[@]}" || ratchet_rc=$?
  else
    say "ratchet (tools/security-baseline.json)"
    python3 "$ROOT/tools/security-ratchet.py" check "${produced[@]}" || ratchet_rc=$?
  fi
fi
if [ "$ratchet_rc" -ne 0 ]; then
  exit "$ratchet_rc"
fi
if [ "$FAIL_ON" = "any" ] && [ "$findings" -gt 0 ]; then
  echo "  SCAN_FAIL_ON=any and there are findings → failing" >&2
  exit 1
fi
exit 0
