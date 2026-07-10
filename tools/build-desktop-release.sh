#!/usr/bin/env bash
# Build the Tessera desktop (Tauri) release bundles for Linux, sign the updater
# artifacts, and assemble the self-update manifest into ./desktop-dist/ for
# serving at https://tessera.msdnna.website/desktop/.
#
# Windows artifacts (.exe + .exe.sig) are built separately on a Windows machine
# (`cargo tauri build`) and dropped into ./desktop-dist/ before publishing; this
# script merges them into latest.json if present.
#
# Signing key (minisign, kept OUT of the repo, like the Android keystore):
#   TESSERA_SIGNING_KEY_FILE  (default ~/.tessera/tessera-desktop-updater.key)
#   TAURI_SIGNING_PRIVATE_KEY_PASSWORD  (empty for the default key)
# Note: Tauri reads the key from TAURI_SIGNING_PRIVATE_KEY (accepts a path or the
# key content); we point it at the file below.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$SCRIPT_DIR/.."
SRC_TAURI="$ROOT/desktop/src-tauri"
DIST="$ROOT/desktop-dist"
VERSION="$(tr -d '[:space:]' < "$ROOT/desktop/VERSION")"

# WSL: interop injects Windows dirs into PATH (…/System32/…/WindowsApps). While
# bundling the AppImage, linuxdeploy walks every PATH entry and its boost::filesystem
# throws "Permission denied" on those Windows paths → aborts with the opaque
# `failed to run linuxdeploy`. Drop /mnt/* from PATH for this script's environment
# (Linux tooling only; the .deb path doesn't need it but it's harmless).
# AppImage in WSL also needs `libfuse2t64` (fuse2 compat) + `patchelf` installed.
if grep -qi microsoft /proc/version 2>/dev/null; then
  PATH="$(printf '%s' "$PATH" | tr ':' '\n' | grep -v '^/mnt/' | paste -sd:)"
  export PATH
fi

: "${TESSERA_SIGNING_KEY_FILE:=$HOME/.tessera/tessera-desktop-updater.key}"
: "${TAURI_SIGNING_PRIVATE_KEY_PASSWORD:=}"

if [ ! -f "$TESSERA_SIGNING_KEY_FILE" ]; then
  echo "Error: signing key not found at $TESSERA_SIGNING_KEY_FILE" >&2
  echo "Generate once: cargo tauri signer generate -w \"$TESSERA_SIGNING_KEY_FILE\"" >&2
  exit 1
fi
export TAURI_SIGNING_PRIVATE_KEY="$TESSERA_SIGNING_KEY_FILE"
export TAURI_SIGNING_PRIVATE_KEY_PASSWORD

PUB_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
BASE_URL="https://tessera.msdnna.website/desktop"

BUNDLE="$SRC_TAURI/target/release/bundle"
mkdir -p "$DIST"

echo "==> Building Tessera desktop $VERSION — .deb (direct download)"
( cd "$SRC_TAURI" && cargo tauri build --bundles deb )
# Take the freshly-built .deb (the bundle dir holds only the current version).
DEB="$(find "$BUNDLE/deb" -maxdepth 1 -name '*.deb' | head -1 || true)"
[ -n "$DEB" ] && cp "$DEB" "$DIST/"

# The AppImage is the Linux self-update target (Tauri updates AppImages in place),
# but it needs a FUSE-capable environment (linuxdeploy) — this fails under some
# WSL setups. Attempt it; tolerate failure and just skip the Linux updater entry.
echo "==> Building AppImage (Linux self-update target)"
( cd "$SRC_TAURI" && APPIMAGE_EXTRACT_AND_RUN=1 NO_STRIP=1 cargo tauri build --bundles appimage ) \
  || echo "WARN: AppImage bundling failed (needs FUSE/linuxdeploy) — no Linux self-update this run"
APPIMAGE="$(find "$BUNDLE/appimage" -maxdepth 1 -name '*.AppImage' | head -1 || true)"
[ -n "$APPIMAGE" ] && cp "$APPIMAGE" "${APPIMAGE}.sig" "$DIST/" 2>/dev/null || true

# Windows (NSIS) and future .rpm are built elsewhere and dropped into $DIST. Match
# the CURRENT release version so STALE installers from older builds left in $DIST
# are ignored — the manifest advertises exactly one version per format.
WIN_EXE="$(find "$DIST" -maxdepth 1 -name "*_${VERSION}_*-setup.exe" | head -1 || true)"
RPM="$(find "$DIST" -maxdepth 1 -name "*${VERSION}*.rpm" | head -1 || true)"

# --- Signed updater `platforms` block (consumed by the Tauri updater) ----------
# Absolute URLs (the updater requires them) + minisign signatures. AppImage is the
# in-place Linux update target; Windows is the NSIS installer.
LINUX_BLOCK=""
if [ -n "$APPIMAGE" ] && [ -f "${APPIMAGE}.sig" ]; then
  LINUX_BLOCK=$(cat <<JSON
    "linux-x86_64": {
      "signature": "$(cat "${APPIMAGE}.sig")",
      "url": "$BASE_URL/$(basename "$APPIMAGE")"
    }
JSON
)
  echo "==> Linux updater artifact: $(basename "$APPIMAGE")"
fi
WIN_BLOCK=""
if [ -n "$WIN_EXE" ] && [ -f "${WIN_EXE}.sig" ]; then
  WIN_BLOCK=$(cat <<JSON
    "windows-x86_64": {
      "signature": "$(cat "${WIN_EXE}.sig")",
      "url": "$BASE_URL/$(basename "$WIN_EXE")"
    }
JSON
)
  echo "==> Windows updater artifact: $(basename "$WIN_EXE")"
else
  echo "==> No Windows $VERSION artifact in $DIST (drop *_${VERSION}_*-setup.exe + .sig to include)"
fi

# Join the present platform blocks with a comma.
PLATFORMS="$LINUX_BLOCK"
if [ -n "$LINUX_BLOCK" ] && [ -n "$WIN_BLOCK" ]; then
  PLATFORMS="$LINUX_BLOCK,
$WIN_BLOCK"
elif [ -n "$WIN_BLOCK" ]; then
  PLATFORMS="$WIN_BLOCK"
fi
if [ -z "$PLATFORMS" ]; then
  echo "Error: no updater artifacts produced (no AppImage, no Windows .exe) — nothing to publish" >&2
  exit 1
fi

# --- Direct-download catalogue for the web login page --------------------------
# Consumed only by the website's "Скачать приложение" button — no signatures, and
# exactly ONE entry per (platform, format) at the current version (stale files in
# $DIST are ignored). Stored as bare FILENAMES so the site resolves them RELATIVE
# to whatever origin serves it (`<serverBase>/desktop/<file>`, same-origin on web)
# — no hard-coded public domain, mirroring how the Android APK link is built.
dl_obj() { # $1=format $2=path — prints {"format","file"} when the file exists
  [ -n "$2" ] && [ -f "$2" ] && printf '{"format":"%s","file":"%s"}' "$1" "$(basename "$2")"
}
join_csv() { # comma-join non-empty args
  local out="" a
  for a in "$@"; do
    [ -n "$a" ] || continue
    [ -z "$out" ] && out="$a" || out="$out,$a"
  done
  printf '%s' "$out"
}
# AppImage first — the recommended (in-place self-updating) Linux variant.
LINUX_DL="$(join_csv "$(dl_obj appimage "$APPIMAGE")" "$(dl_obj deb "$DEB")" "$(dl_obj rpm "$RPM")")"
WIN_DL="$(dl_obj exe "$WIN_EXE")"

cat > "$DIST/latest.json" <<JSON
{
  "version": "$VERSION",
  "notes": "See CHANGELOG for details.",
  "pub_date": "$PUB_DATE",
  "platforms": {
$PLATFORMS
  },
  "downloads": {
    "linux": [$LINUX_DL],
    "windows": [$WIN_DL]
  }
}
JSON

echo "==> Wrote $DIST/latest.json"
echo "==> Publish: copy $DIST/ to the server's ./desktop/ (served at $BASE_URL/)"
ls -1 "$DIST"
