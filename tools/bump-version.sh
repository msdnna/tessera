#!/usr/bin/env bash
# Bump a service's semantic version.
# Usage: tools/bump-version.sh <backend|frontend|android|desktop|mcp> [patch|minor|major]
set -euo pipefail

service="${1:?usage: bump-version.sh <service> [patch|minor|major]}"
bump="${2:-patch}"

case "$service" in
  backend)  file="backend/VERSION" ;;
  frontend) file="frontend/VERSION" ;;
  android)  file="android/VERSION" ;;
  desktop)  file="desktop/VERSION" ;;
  mcp)      file="mcp/VERSION" ;;
  *) echo "unknown service: $service (want backend|frontend|android|desktop|mcp)" >&2; exit 1 ;;
esac

[ -f "$file" ] || { echo "missing $file" >&2; exit 1; }
cur="$(tr -d '[:space:]' < "$file")"
IFS=. read -r major minor patch <<< "$cur"

case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "unknown bump: $bump (want patch|minor|major)" >&2; exit 1 ;;
esac

new="${major}.${minor}.${patch}"
printf '%s\n' "$new" > "$file"

# Tauri reads the app version from Cargo.toml (tauri.conf.json omits `version`),
# so keep the crate version in lockstep with desktop/VERSION.
if [ "$service" = desktop ]; then
  cargo="desktop/src-tauri/Cargo.toml"
  if [ -f "$cargo" ]; then
    sed -i -E "0,/^version = \"[0-9]+\.[0-9]+\.[0-9]+\"/s//version = \"$new\"/" "$cargo"
  fi
fi

echo "$service: $cur -> $new"
