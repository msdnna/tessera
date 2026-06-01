#!/usr/bin/env bash
# Bump a service's semantic version.
# Usage: tools/bump-version.sh <backend|frontend|android> [patch|minor|major]
set -euo pipefail

service="${1:?usage: bump-version.sh <service> [patch|minor|major]}"
bump="${2:-patch}"

case "$service" in
  backend)  file="backend/VERSION" ;;
  frontend) file="frontend/VERSION" ;;
  android)  file="android/VERSION" ;;
  *) echo "unknown service: $service (want backend|frontend|android)" >&2; exit 1 ;;
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
echo "$service: $cur -> $new"
