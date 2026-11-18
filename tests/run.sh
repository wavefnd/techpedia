#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$root_dir/tests/lib/build-lock.sh"
cd "$root_dir"
suite=${1:-all}
if [[ $# -gt 1 || ( "$suite" != all && "$suite" != unit && "$suite" != integration ) ]]; then
  echo "Usage: ./tests/run.sh [all|unit|integration]" >&2
  exit 2
fi
for path in "$root_dir/.local" "$root_dir/.local/test" "$root_dir/.local/test/tmp"; do
  if [[ -L "$path" ]]; then echo "Refusing a symlink in the test temporary path." >&2; exit 1; fi
done
mkdir -p "$root_dir/.local/test/tmp"
export TMPDIR="$root_dir/.local/test/tmp"

if [[ "$suite" != integration ]]; then
  acquire_test_build_lock "$root_dir/.local/test"
  npm run typecheck --prefix web
  npm run test --prefix web
  npm run build --prefix web
  ./tests/go.sh test ./...
  ./tests/go.sh vet ./...
  ./tests/go.sh --module benchmark test ./...
  release_test_build_lock
fi
if [[ "$suite" != unit ]]; then
  bash -n start.sh restart.sh stop.sh backup.sh restore.sh move.sh tests/run.sh tests/go.sh tests/server.sh tests/lib/build-lock.sh
  node --test tests/integration/*.test.mjs
fi
if [[ "$suite" == all ]]; then
  python3 -m unittest discover -s benchmark/tests
  npm run build --prefix docs
fi
