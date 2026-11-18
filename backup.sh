#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
output=${1:-"$root_dir/backups/techpedia-data-${timestamp}.tar.gz"}
if [[ $# -gt 1 || "$output" == -* ]]; then
  echo "Usage: ./backup.sh [output.tar.gz]" >&2
  exit 2
fi
if [[ "$output" != /* ]]; then
  output="$root_dir/$output"
fi
if [[ -e "$output" || -e "$output.sha256" ]]; then
  echo "Backup file already exists: $output" >&2
  exit 1
fi
if [[ ! -d data ]]; then
  echo "The data directory does not exist." >&2
  exit 1
fi

mkdir -p "$(dirname "$output")"
was_running=false
if docker compose ps --status running --services | grep -qx techpedia; then
  was_running=true
  docker compose stop techpedia
fi

restart_if_needed() {
  if [[ "$was_running" == "true" ]]; then
    docker compose start techpedia >/dev/null
  fi
}
trap restart_if_needed EXIT

tar -czf "$output" data
(
  cd "$(dirname "$output")"
  sha256sum "$(basename "$output")" > "$(basename "$output").sha256"
)

restart_if_needed
was_running=false
trap - EXIT

echo "Backup created: $output"
echo "Checksum: $output.sha256"
