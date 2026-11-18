#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

docker compose --profile production down --remove-orphans
echo "TechPedia stopped. The data directory is preserved."
