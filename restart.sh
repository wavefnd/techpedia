#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

pull_updates=true
if [[ "${1:-}" == "--no-pull" && $# -eq 1 ]]; then
  pull_updates=false
elif [[ $# -gt 0 ]]; then
  echo "Usage: ./restart.sh [--no-pull]" >&2
  exit 2
fi

if [[ "$pull_updates" == "true" ]]; then
  if ! command -v git >/dev/null 2>&1 || ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "A Git checkout is required for updates; use --no-pull to rebuild only." >&2
    exit 1
  fi
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Uncommitted changes; commit or resolve them before updating." >&2
    exit 1
  fi
  current_origin=$(git remote get-url origin)
  case "$current_origin" in
    https://github.com/wavefnd/techpedia.git|git@github.com:wavefnd/techpedia.git)
      ;;
    *)
      echo "Unexpected origin; configure the approved wavefnd/techpedia source explicitly." >&2
      exit 1
      ;;
  esac
  if [[ "$(git branch --show-current)" != "main" ]]; then
    echo "Source updates require main; switch branches explicitly or use --no-pull." >&2
    exit 1
  fi
  git fetch origin refs/heads/main:refs/remotes/origin/main
  git merge --ff-only origin/main
fi

exec "$root_dir/start.sh" --force-recreate
