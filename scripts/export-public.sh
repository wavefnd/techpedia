#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"
destination=${1:-}
if [[ $# -ne 1 || "$destination" != /* || -e "$destination" ]]; then
  echo "Usage: ./scripts/export-public.sh /absolute/nonexistent/destination" >&2
  exit 2
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "Commit and review the preparation first; export requires a clean tree." >&2
  exit 1
fi
while IFS= read -r -d '' path; do
  case "$path" in
    .env.example|internal/edition/data/*) ;;
    .env|.env.*|data/*|backups/*|.codex|.codex/*|.agents/*|.aws/*|.git/*|*/node_modules/*|internal/server/static/dist/*|docs/.vitepress/dist/*|docs/.vitepress/cache/*|build/*|*.pem|*.key|*.p12|*.pfx)
      echo "Forbidden public-export path: $path" >&2
      exit 1 ;;
  esac
done < <(git ls-tree -r --name-only -z HEAD)
mkdir -- "$destination"
git archive HEAD | tar -x -C "$destination"
git -C "$destination" init --initial-branch=main
git -C "$destination" add --all
git -C "$destination" commit -m "Initial public release of TechPedia"
echo "Created one local main commit at $destination. Review before publishing. Nothing was pushed."
