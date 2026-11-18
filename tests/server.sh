#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$root_dir/tests/lib/build-lock.sh"
runtime_dir="$root_dir/.local/test"
mode=run
case "${1:-}" in
  "") ;;
  --no-build) mode=run-existing ;;
  --build-only) mode=build ;;
  --check) mode=check ;;
  --help)
    echo "Usage: ./tests/server.sh [--no-build|--build-only|--check]"
    echo "Test-only settings: TECHPEDIA_TEST_PORT, TECHPEDIA_TEST_OWNER_USERNAME, TECHPEDIA_TEST_OWNER_PASSWORD"
    exit 0 ;;
  *) echo "Unknown option. Use --help." >&2; exit 2 ;;
esac
if [[ $# -gt 1 ]]; then echo "Only one option is accepted." >&2; exit 2; fi

test_port=${TECHPEDIA_TEST_PORT:-18090}
if [[ ! "$test_port" =~ ^[0-9]{4,5}$ ]]; then
  echo "TECHPEDIA_TEST_PORT must be an integer between 1024 and 65535." >&2
  exit 2
fi
test_port=$((10#$test_port))
if (( test_port < 1024 || test_port > 65535 )); then
  echo "TECHPEDIA_TEST_PORT must be between 1024 and 65535." >&2
  exit 2
fi
owner_name=${TECHPEDIA_TEST_OWNER_USERNAME:-}
owner_password=${TECHPEDIA_TEST_OWNER_PASSWORD:-}
if [[ -n "$owner_name" && -z "$owner_password" || -z "$owner_name" && -n "$owner_password" ]]; then
  echo "Set both test-owner variables, or neither. No production owner settings are inherited." >&2
  exit 2
fi
if [[ "$owner_password" == *replace-with-* ]]; then
  echo "Replace the example test-owner password." >&2
  exit 2
fi
for path in "$root_dir/.local" "$runtime_dir" "$runtime_dir/bin" "$runtime_dir/bin/techpedia" "$runtime_dir/data" "$runtime_dir/tmp" "$runtime_dir/server.log"; do
  if [[ -L "$path" ]]; then
    echo "Refusing a symlink in the local test runtime." >&2
    exit 1
  fi
done
if [[ -d "$runtime_dir/data" && -n "$(find "$runtime_dir/data" -type l -print -quit)" ]]; then
  echo "Refusing a symlink inside test data." >&2
  exit 1
fi
if [[ "$mode" == check ]]; then
  printf 'Address: 127.0.0.1:%s\nPublic URL: http://127.0.0.1:%s\nData: %s/data\nLog: %s/server.log\n' "$test_port" "$test_port" "$runtime_dir" "$runtime_dir"
  exit 0
fi
mkdir -p "$runtime_dir/bin" "$runtime_dir/data" "$runtime_dir/tmp"
chmod 700 "$runtime_dir" "$runtime_dir/bin" "$runtime_dir/data" "$runtime_dir/tmp"
cd "$root_dir"
if [[ "$mode" != run-existing ]]; then
  acquire_test_build_lock "$runtime_dir"
  npm run build --prefix web
  CGO_ENABLED="${CGO_ENABLED:-0}" go build -o "$runtime_dir/bin/techpedia" ./cmd/techpedia
  release_test_build_lock
fi
if [[ "$mode" == build ]]; then exit 0; fi
if [[ ! -x "$runtime_dir/bin/techpedia" ]]; then
  echo "No test binary exists. Run ./tests/server.sh --build-only first." >&2
  exit 1
fi
printf 'Test wiki: http://127.0.0.1:%s\nLog: %s/server.log\nPress Ctrl+C to stop.\n' "$test_port" "$runtime_dir" >&2
exec env -i \
  PATH="$PATH" \
  TMPDIR="$runtime_dir/tmp" \
  TECHPEDIA_ADDR="127.0.0.1:$test_port" \
  TECHPEDIA_PUBLIC_URL="http://127.0.0.1:$test_port" \
  TECHPEDIA_DATA_DIR="$runtime_dir/data" \
  TECHPEDIA_CSP_MODE=report-only \
  TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME="$owner_name" \
  TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD="$owner_password" \
  TECHPEDIA_MCP_ENABLED=false \
  TECHPEDIA_INTERFACE_AUTO_TRANSLATION=false \
  TECHPEDIA_ANALYTICS_ENABLED=false \
  "$runtime_dir/bin/techpedia" >> "$runtime_dir/server.log" 2>&1
