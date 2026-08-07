#!/usr/bin/env bash
# Создать скелет changelog-фрагмента для задачи.
# Usage: tools/changelog-add.sh <component> <task#> [slug] [bump]
#   component: backend|frontend|android|desktop|mcp
#   task#:     номер задачи Tessera
#   slug:      необязательный суффикс имени файла
#   bump:      patch|minor|major (необязательно; иначе выводится из типа коммита)
set -euo pipefail

comp="${1:?usage: changelog-add.sh <component> <task#> [slug] [bump]}"
task="${2:?нужен номер задачи}"
slug="${3:-}"
bump="${4:-}"

case "$comp" in
  backend|frontend|android|desktop|mcp) ;;
  *) echo "unknown component: $comp (want backend|frontend|android|desktop|mcp)" >&2; exit 1 ;;
esac

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="$root/changelog.d/$comp"
mkdir -p "$dir"

name="$task"
[ -n "$slug" ] && name="$task-$slug"
file="$dir/$name.md"

if [ -e "$file" ]; then
  echo "уже существует: changelog.d/$comp/$name.md" >&2
  exit 0
fi

{
  if [ -n "$bump" ]; then
    printf -- '---\nbump: %s\n---\n' "$bump"
  fi
  printf -- '- **<type>(<scope>): краткое описание (#%s).**\n' "$task"
} > "$file"

echo "создан: changelog.d/$comp/$name.md"
