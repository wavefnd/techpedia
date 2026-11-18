#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

archive=${1:-}
confirmation=${2:-}
if [[ -z "$archive" || "$confirmation" != "--yes" || $# -ne 2 ]]; then
  echo "Usage: ./restore.sh backup.tar.gz --yes" >&2
  exit 2
fi
if [[ "$archive" != /* ]]; then
  archive="$root_dir/$archive"
fi
if [[ ! -f "$archive" ]]; then
  echo "Backup file not found: $archive" >&2
  exit 1
fi
if [[ -f "$archive.sha256" ]]; then
  (cd "$(dirname "$archive")" && sha256sum -c "$(basename "$archive").sha256")
fi

while IFS= read -r entry; do
  if [[ "$entry" == /* || "$entry" == *".."* || ( "$entry" != "data" && "$entry" != data/* ) ]]; then
    echo "Disallowed backup path: $entry" >&2
    exit 1
  fi
done < <(tar -tzf "$archive")

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
rollback="$root_dir/backups/pre-restore-${timestamp}.tar.gz"
mkdir -p "$root_dir/backups"
had_data=false

docker compose --profile production stop caddy techpedia >/dev/null 2>&1 || true
if [[ -d data ]]; then
  had_data=true
  tar -czf "$rollback" data
  (
    cd "$(dirname "$rollback")"
    sha256sum "$(basename "$rollback")" > "$(basename "$rollback").sha256"
  )
  rm -rf data
fi

if ! tar -xzf "$archive" -C "$root_dir"; then
  echo "Restore failed. Recovering the pre-restore backup." >&2
  rm -rf data
  if [[ "$had_data" == "true" ]]; then
    tar -xzf "$rollback" -C "$root_dir"
  fi
  exit 1
fi

./start.sh --force-recreate
if [[ "$had_data" == "true" ]]; then
  echo "Restore completed. Pre-restore data: $rollback"
else
  echo "Restore completed. No previous data directory existed."
fi
