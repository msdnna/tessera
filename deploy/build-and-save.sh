#!/usr/bin/env bash
# Build production images on the DEV box and save them to a tarball for transfer
# to the server (which has no source tree). Run from the repo root or anywhere:
#   bash deploy/build-and-save.sh
#
# Output: deploy/dist/tessera-images-<be>-<fe>.tar.gz  + a paste-ready .env hint.
# Ship it:  scp deploy/dist/tessera-images-*.tar.gz user@server:/opt/tessera/
# Load it:  docker load -i tessera-images-*.tar.gz   (on the server)
#
# Note: postgres:17-alpine and caddy:2-alpine are public — the server pulls
# those itself. Only our two images travel by tar.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BE_VER="$(cat backend/VERSION)"
FE_VER="$(cat frontend/VERSION)"
BACKEND_IMAGE="tessera-backend:${BE_VER}"
FRONTEND_IMAGE="tessera-frontend:${FE_VER}"
OUT="deploy/dist/tessera-images-be${BE_VER}-fe${FE_VER}.tar.gz"

# Build metadata for GET /api/version and the sidebar version tooltip. Derived
# here (the build contexts are ./backend and ./frontend, which have no .git) and
# passed to both images as build args. A shallow/CI checkout without git still
# builds — the values just fall back to empty and the UI omits them.
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo '')"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "==> Build metadata: commit=${GIT_COMMIT:-<none>} date=${BUILD_DATE}"

echo "==> Building ${BACKEND_IMAGE} (distroless, prod Dockerfile)"
docker build -f backend/Dockerfile.prod \
  --build-arg "GIT_COMMIT=${GIT_COMMIT}" --build-arg "BUILD_DATE=${BUILD_DATE}" \
  -t "${BACKEND_IMAGE}" backend

echo "==> Building ${FRONTEND_IMAGE}"
# Context is the repo root (not ./frontend): the help centre globs docs/help at
# build time (#2786). The root .dockerignore keeps the context to frontend/+docs/.
docker build -f frontend/Dockerfile \
  --build-arg "GIT_COMMIT=${GIT_COMMIT}" --build-arg "BUILD_DATE=${BUILD_DATE}" \
  -t "${FRONTEND_IMAGE}" .

echo "==> Saving images to ${OUT}"
mkdir -p deploy/dist
docker save "${BACKEND_IMAGE}" "${FRONTEND_IMAGE}" | gzip > "${OUT}"

echo
echo "==> Done. Tarball: ${OUT}  ($(du -h "${OUT}" | cut -f1))"
echo "    Put these in the server's deploy/.env:"
echo "      BACKEND_IMAGE=${BACKEND_IMAGE}"
echo "      FRONTEND_IMAGE=${FRONTEND_IMAGE}"
