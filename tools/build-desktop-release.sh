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
find "$BUNDLE/deb" -maxdepth 1 -name '*.deb' -exec cp {} "$DIST/" \; 2>/dev/null || true

# The AppImage is the Linux self-update target (Tauri updates AppImages in place),
# but it needs a FUSE-capable environment (linuxdeploy) — this fails under some
# WSL setups. Attempt it; tolerate failure and just skip the Linux updater entry.
echo "==> Building AppImage (Linux self-update target)"
( cd "$SRC_TAURI" && APPIMAGE_EXTRACT_AND_RUN=1 NO_STRIP=1 cargo tauri build --bundles appimage ) \
  || echo "WARN: AppImage bundling failed (needs FUSE/linuxdeploy) — no Linux self-update this run"

LINUX_BLOCK=""
APPIMAGE="$(find "$BUNDLE/appimage" -maxdepth 1 -name '*.AppImage' | head -1 || true)"
if [ -n "$APPIMAGE" ] && [ -f "${APPIMAGE}.sig" ]; then
  cp "$APPIMAGE" "${APPIMAGE}.sig" "$DIST/"
  LINUX_BLOCK=$(cat <<JSON
    "linux-x86_64": {
      "signature": "$(cat "${APPIMAGE}.sig")",
      "url": "$BASE_URL/$(basename "$APPIMAGE")"
    }
JSON
)
  echo "==> Linux updater artifact: $(basename "$APPIMAGE")"
fi

# Merge any Windows artifacts already dropped into $DIST (built on Windows).
WIN_EXE="$(find "$DIST" -maxdepth 1 -name '*-setup.exe' | head -1 || true)"
WIN_BLOCK=""
if [ -n "$WIN_EXE" ] && [ -f "${WIN_EXE}.sig" ]; then
  WIN_BLOCK=$(cat <<JSON
    "windows-x86_64": {
      "signature": "$(cat "${WIN_EXE}.sig")",
      "url": "$BASE_URL/$(basename "$WIN_EXE")"
    }
JSON
)
  echo "==> Merged Windows artifact: $(basename "$WIN_EXE")"
else
  echo "==> No Windows artifact in $DIST (build it on Windows and re-run to include)"
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
# Separate from the signed `platforms` updater block above (which the Tauri
# updater consumes and which lists only the in-place-updatable AppImage). This
# `downloads` block needs no signatures and lists EVERY artifact present in
# $DIST so the website's "Скачать приложение" button can offer all Linux formats
# (AppImage / .deb / future .rpm) and the Windows installer. The updater ignores
# unknown top-level keys, so co-locating it here keeps one published manifest.
dl_array() {
  # Args: <format> <path>...; prints comma-joined {"format","url"} for real files.
  local fmt="$1" out="" first=1 f; shift
  for f in "$@"; do
    [ -f "$f" ] || continue
    [ $first -eq 1 ] || out+=","
    first=0
    out+="$(printf '{"format":"%s","url":"%s/%s"}' "$fmt" "$BASE_URL" "$(basename "$f")")"
  done
  printf '%s' "$out"
}
join_csv() { # comma-join non-empty args
  local out="" a
  for a in "$@"; do
    [ -n "$a" ] || continue
    [ -z "$out" ] && out="$a" || out="$out,$a"
  done
  printf '%s' "$out"
}
# AppImage first — it's the recommended (in-place self-updating) Linux variant.
LINUX_DL="$(join_csv \
  "$(dl_array appimage "$DIST"/*.AppImage)" \
  "$(dl_array deb "$DIST"/*.deb)" \
  "$(dl_array rpm "$DIST"/*.rpm)")"
WIN_DL="$(dl_array exe "$DIST"/*-setup.exe)"

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
