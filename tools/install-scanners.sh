#!/usr/bin/env bash
# Installs the SAST/SCA scanners used by tools/security-scan.sh (#2936).
#
# Usage: ./tools/install-scanners.sh            # everything that is missing
#        ./tools/install-scanners.sh --force    # reinstall even if present
#
# Release binaries over `go install`, for two reasons:
#
#  1. On this dev host `go install <tool>@latest` hangs indefinitely: the module
#     proxy is reachable but sum.golang.org is cut off at the TLS handshake
#     (SSL_ERROR_SYSCALL), so the checksum lookup for a module that is not
#     already in the cache never completes. GitHub releases download fine.
#  2. A pinned release plus its published checksum is reproducible; `@latest`
#     silently drifts between a developer's machine and CI.
#
# Binaries land in tools/bin/ (gitignored) rather than GOPATH/bin so the pinned
# versions here are what runs, not whatever a developer installed years ago.
# security-scan.sh puts that directory first on PATH.
#
# Versions are overridable for a one-off bump:
#   GOSEC_VERSION=2.30.0 ./tools/install-scanners.sh --force
set -euo pipefail

GOSEC_VERSION="${GOSEC_VERSION:-2.29.0}"
OSVSCANNER_VERSION="${OSVSCANNER_VERSION:-2.5.1}"
GITLEAKS_VERSION="${GITLEAKS_VERSION:-8.30.1}"
# govulncheck publishes no release binaries, so it stays a `go install`. It is
# usually already in the module cache (the repo's own CI installs it), which is
# why that one path does work here.
GOVULNCHECK_PKG="${GOVULNCHECK_PKG:-golang.org/x/vuln/cmd/govulncheck@latest}"

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BIN="$ROOT/tools/bin"
# Downloads land here, not in a fresh `mktemp -d`, so `curl -C -` can resume a
# transfer across *runs* and not merely across retries within one. On this host
# the release link runs at ~50 KB/s and dies often enough that three separate
# attempts at the 28 MB osv-scanner each threw away 17-21 MB and restarted from
# zero. Filenames carry the version, so a pinned bump never resumes into the
# previous release's bytes, and a checksum mismatch deletes the file rather than
# leaving a poisoned partial to be resumed forever.
CACHE="$BIN/.cache"
mkdir -p "$BIN" "$CACHE"

FORCE=0
[ "${1:-}" = "--force" ] && FORCE=1

say() { printf '\n\033[1m── %s\033[0m\n' "$*"; }

# fetch <url> <dest> — curl with retries.
#
# The flags are not cargo cult; each one is a failure seen on this host pulling
# release-assets.githubusercontent.com, where the link runs at ~50 KB/s and a
# 28 MB scanner takes ten minutes:
#   --http1.1    HTTP/2 aborts the long transfer with PROTOCOL_ERROR (err 1)
#   --tls-max 1.2  measurably faster and steadier here than a 1.3 handshake
#   -C -         resume instead of restarting from zero when a retry fires
#   --speed-*    give up on a stall (60s under 1 KB/s) but never on mere slowness
# Deliberately no --max-time: a slow success beats a timed-out failure.
fetch() {
  curl -sSfL --http1.1 --tls-max 1.2 -C - \
    --retry 5 --retry-delay 5 --retry-all-errors \
    --connect-timeout 20 --speed-limit 1024 --speed-time 60 \
    -o "$2" "$1"
}

# verify <file> <sums-url> <name-in-sums> — refuse to install an unverified
# binary. A security scanner fetched over an unauthenticated channel is exactly
# the kind of supply-chain hole these tools are meant to find.
verify() {
  local file="$1" sums_url="$2" name="$3" tmp expected actual
  tmp=$(mktemp)
  fetch "$sums_url" "$tmp"
  expected=$(awk -v n="$name" '$2 == n || $2 == "*" n {print $1; exit}' "$tmp")
  rm -f "$tmp"
  if [ -z "$expected" ]; then
    echo "  ✗ $name absent from the published checksums — refusing to install" >&2
    return 1
  fi
  actual=$(sha256sum "$file" | cut -d' ' -f1)
  if [ "$expected" != "$actual" ]; then
    echo "  ✗ checksum mismatch for $name" >&2
    echo "      expected $expected" >&2
    echo "      actual   $actual" >&2
    # Drop the cached copy: it is either truncated or tampered with, and keeping
    # it would make every later run resume the same bad bytes and fail again.
    rm -f "$file"
    return 1
  fi
  echo "  · checksum ok"
}

# skip <tool> — true when the binary is already there and --force was not given.
skip() {
  if [ "$FORCE" = "0" ] && [ -x "$BIN/$1" ]; then
    echo "  · $1 already installed ($("$BIN/$1" --version 2>&1 | head -1))"
    return 0
  fi
  return 1
}

install_gosec() {
  say "gosec $GOSEC_VERSION (SAST, Go)"
  skip gosec && return 0
  local base="https://github.com/securego/gosec/releases/download/v$GOSEC_VERSION"
  local tarball="gosec_${GOSEC_VERSION}_linux_amd64.tar.gz"
  local tmp
  fetch "$base/$tarball" "$CACHE/$tarball"
  verify "$CACHE/$tarball" "$base/gosec_${GOSEC_VERSION}_checksums.txt" "$tarball"
  tmp=$(mktemp -d)
  tar -xzf "$CACHE/$tarball" -C "$tmp" gosec
  install -m 0755 "$tmp/gosec" "$BIN/gosec"
  rm -rf "$tmp"
  echo "  ✓ $BIN/gosec"
}

install_osvscanner() {
  say "osv-scanner $OSVSCANNER_VERSION (SCA, all ecosystems)"
  skip osv-scanner && return 0
  local base="https://github.com/google/osv-scanner/releases/download/v$OSVSCANNER_VERSION"
  # A bare binary, not an archive — the checksum file names it the same way. The
  # published name carries no version, so the cache copy adds one.
  local asset="osv-scanner_linux_amd64"
  local cached="$CACHE/osv-scanner_${OSVSCANNER_VERSION}_linux_amd64"
  fetch "$base/$asset" "$cached"
  verify "$cached" "$base/osv-scanner_SHA256SUMS" "$asset"
  install -m 0755 "$cached" "$BIN/osv-scanner"
  echo "  ✓ $BIN/osv-scanner"
}

install_gitleaks() {
  say "gitleaks $GITLEAKS_VERSION (secrets)"
  skip gitleaks && return 0
  local base="https://github.com/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION"
  # x64, not amd64: gitleaks names its linux asset differently from the others.
  local tarball="gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
  local tmp
  fetch "$base/$tarball" "$CACHE/$tarball"
  verify "$CACHE/$tarball" "$base/gitleaks_${GITLEAKS_VERSION}_checksums.txt" "$tarball"
  tmp=$(mktemp -d)
  tar -xzf "$CACHE/$tarball" -C "$tmp" gitleaks
  install -m 0755 "$tmp/gitleaks" "$BIN/gitleaks"
  rm -rf "$tmp"
  echo "  ✓ $BIN/gitleaks"
}

install_govulncheck() {
  say "govulncheck (SCA with reachability, Go)"
  if [ "$FORCE" = "0" ] && command -v govulncheck >/dev/null 2>&1; then
    echo "  · govulncheck already on PATH ($(command -v govulncheck))"
    return 0
  fi
  # Plain `go`, not the repo's pinned go1.25.9: govulncheck's own go.mod asks for
  # a newer toolchain and GOTOOLCHAIN=auto has to be free to fetch one.
  if GOBIN="$BIN" go install "$GOVULNCHECK_PKG"; then
    echo "  ✓ $BIN/govulncheck"
  else
    echo "  ✗ go install failed. On a host where sum.golang.org is unreachable" >&2
    echo "    this hangs or errors; the module cache needs a working checksum" >&2
    echo "    path. CI installs it fine — see .github/workflows/ci.yml." >&2
    return 1
  fi
}

install_gosec
install_osvscanner
install_gitleaks
install_govulncheck

say "done"
echo "  scanners in $BIN"
echo "  run: make scan"
