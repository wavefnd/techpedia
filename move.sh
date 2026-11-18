#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
project_name=$(basename "$root_dir")
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
output=""
allow_dirty=false

for argument in "$@"; do
  case "$argument" in
    --allow-dirty)
      allow_dirty=true
      ;;
    -*)
      echo "Usage: ./move.sh [--allow-dirty] [output.tar.gz]" >&2
      exit 2
      ;;
    *)
      if [[ -n "$output" ]]; then
        echo "Usage: ./move.sh [--allow-dirty] [output.tar.gz]" >&2
        exit 2
      fi
      output=$argument
      ;;
  esac
done

output=${output:-"$(dirname "$root_dir")/techpedia-transfer-${timestamp}.tar.gz"}
archive_listing=$(mktemp)
trap 'rm -f "$archive_listing"' EXIT

if [[ -d "$root_dir/.git" && "$allow_dirty" != "true" ]]; then
  if [[ -n "$(git -C "$root_dir" status --porcelain)" ]]; then
    echo "Uncommitted changes; source transfer cancelled." >&2
    echo "Commit changes first, or use --allow-dirty for a private working-tree transfer." >&2
    exit 1
  fi
fi

if [[ "$output" != /* ]]; then
  output="$(pwd)/$output"
fi
mkdir -p "$(dirname "$output")"

if [[ -e "$output" ]]; then
  echo "Destination already exists: $output" >&2
  exit 1
fi

private_excludes=(--exclude="${project_name}/.env")
for candidate in "$root_dir"/.env.*; do
  if [[ -e "$candidate" && "${candidate##*/}" != ".env.example" ]]; then
    private_excludes+=(--exclude="${project_name}/${candidate##*/}")
  fi
done

tar "${private_excludes[@]}" \
  --exclude="${project_name}/data" \
  --exclude="${project_name}/.env" \
  --exclude="${project_name}/.idea" \
  --exclude="${project_name}/.codex" \
  --exclude="${project_name}/web/node_modules" \
  --exclude="${project_name}/docs/node_modules" \
  --exclude="${project_name}/docs/.vitepress/cache" \
  --exclude="${project_name}/docs/.vitepress/dist" \
  --exclude="${project_name}/backups" \
  --exclude="${project_name}/build" \
  --exclude="${project_name}/internal/server/static/dist" \
  --exclude="${project_name}/techpedia-transfer-*.tar.gz" \
  -czf "$output" \
  -C "$(dirname "$root_dir")" \
  "$project_name"

tar -tzf "$output" > "$archive_listing"
if ! grep -qx "${project_name}/.git/HEAD" "$archive_listing"; then
  rm -f "$output"
  echo "The private transfer is missing .git; archive cancelled." >&2
  exit 1
fi

echo "Private source transfer created: $output"
echo "Includes: source, Docker configuration, private Git history. Never publish this archive."
echo "Excludes: data, .env, dependencies, generated builds."
echo "Extract on the destination, copy .env.example to .env and configure it."
if [[ "$allow_dirty" == "true" ]]; then
  echo "Warning: uncommitted changes are included; restart.sh updates only a clean tree."
fi
