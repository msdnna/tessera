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
  # Cargo.lock carries the crate's own version too, and it is committed. Leaving
  # it behind means the release commit ships a lock that disagrees with the
  # manifest, and the next `cargo` run rewrites it — so the bump resurfaces as
  # noise inside whatever change happens to build next (it already did: c643fecb,
  # 4883173d, 71f35971). Rewritten textually rather than with `cargo update`,
  # because bumping a version must not need a Rust toolchain on PATH; only the
  # package's own entry moves, which is exactly what cargo would write.
  lock="desktop/src-tauri/Cargo.lock"
  if [ -f "$cargo" ] && [ -f "$lock" ]; then
    crate="$(sed -n -E '0,/^name = "([^"]+)"/s//\1/p' "$cargo")"
    awk -v crate="$crate" -v new="$new" '
      # The package block is the unit: reset at its header so a crate whose entry
      # somehow lacks a version line cannot bleed the rewrite into the next one.
      /^\[\[package\]\]/ { in_crate = 0 }
      $0 == "name = \"" crate "\"" { in_crate = 1; print; next }
      in_crate && /^version = "/ { print "version = \"" new "\""; in_crate = 0; next }
      { print }
    ' "$lock" > "$lock.tmp" && mv "$lock.tmp" "$lock"
  fi
fi

echo "$service: $cur -> $new"
