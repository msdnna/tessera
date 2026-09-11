#!/usr/bin/env bash
# Build the Tessera desktop Linux bundles (.deb + AppImage) inside an Ubuntu 22.04
# container so they link against glibc 2.35 (+ webkit2gtk-4.1) and run on older
# distros — notably Astra Linux SE 1.8 (Debian 12 / glibc 2.36). Building on the WSL
# host (Ubuntu 24.04 / glibc 2.39) instead pins GLIBC_2.3{7,8,9} symbols those
# systems lack, so the AppImage aborts with a glibc-too-old error there.
#
# Invoked by build-desktop-release.sh when DESKTOP_LINUX_DOCKER=1. The frontend must
# already be built (desktop variant) — the container has no Node and reuses the
# prebuilt frontend/dist (its beforeBuildCommand is neutered to a no-op). Signing key
# path + password are inherited from the parent script's environment.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="tessera-desktop-linux-build"
KEY="${TAURI_SIGNING_PRIVATE_KEY:?TAURI_SIGNING_PRIVATE_KEY must point to the minisign key file}"

if [ ! -f "$ROOT/frontend/dist/index.html" ]; then
  echo "Error: frontend/dist not built — run 'corepack yarn --cwd frontend build:desktop' first" >&2
  exit 1
fi

echo "==> Building the Linux build image ($IMAGE) — first run compiles the Tauri CLI (slow, then cached)"
docker build -t "$IMAGE" -f "$SCRIPT_DIR/desktop-linux.Dockerfile" "$SCRIPT_DIR"

echo "==> Bundling .deb + AppImage in the container (glibc 2.35 floor)"
# A named volume caches the crate registry/build across runs. CARGO_TARGET_DIR lives
# under the bind-mounted repo (target/ is gitignored) so artifacts land back on the
# host; the build runs as root, then chowns the tree back to the invoking user.
docker run --rm \
  -v "$ROOT":/work \
  -v "$KEY":/keys/updater.key:ro \
  -v tessera-desktop-cargo:/opt/rust/cargo/registry \
  -e TAURI_SIGNING_PRIVATE_KEY=/keys/updater.key \
  -e TAURI_SIGNING_PRIVATE_KEY_PASSWORD="${TAURI_SIGNING_PRIVATE_KEY_PASSWORD:-}" \
  -e CARGO_TARGET_DIR=/work/desktop/src-tauri/target/linux-docker \
  -e APPIMAGE_EXTRACT_AND_RUN=1 \
  -e NO_STRIP=1 \
  -e HOST_UID="$(id -u)" \
  -e HOST_GID="$(id -g)" \
  -w /work/desktop/src-tauri \
  "$IMAGE" bash -euc '
    # Reuse the host-built frontend/dist: neuter beforeBuildCommand (no Node here).
    printf "%s" "{\"build\":{\"beforeBuildCommand\":\"\"}}" > /tmp/nofront.json
    cargo tauri build --bundles deb     --config /tmp/nofront.json
    cargo tauri build --bundles appimage --config /tmp/nofront.json
    chown -R "$HOST_UID:$HOST_GID" /work/desktop/src-tauri/target/linux-docker
  '
