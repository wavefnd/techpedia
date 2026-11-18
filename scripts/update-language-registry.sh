#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
target="$root/internal/edition/data/language-subtag-registry.txt"
temporary=$(mktemp)
trap 'rm -f "$temporary"' EXIT

curl --fail --location --silent --show-error \
  https://www.iana.org/assignments/language-subtag-registry/language-subtag-registry \
  --output "$temporary"

grep -q '^File-Date: ' "$temporary"
grep -q '^Type: language$' "$temporary"
mv "$temporary" "$target"
trap - EXIT

head -n 1 "$target"
