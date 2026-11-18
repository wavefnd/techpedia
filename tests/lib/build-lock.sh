#!/usr/bin/env bash

test_build_lock=

release_test_build_lock() {
  if [[ -n "${test_build_lock:-}" ]]; then
    rmdir -- "$test_build_lock" 2>/dev/null || true
    test_build_lock=
  fi
}

acquire_test_build_lock() {
  local candidate="$1/build.lock"
  if ! mkdir -- "$candidate" 2>/dev/null; then
    echo "Another test build holds .local/test/build.lock. Wait for it to finish." >&2
    echo "After an interrupted build, confirm no build is active before removing that empty directory." >&2
    return 1
  fi
  test_build_lock=$candidate
  trap release_test_build_lock EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
}
