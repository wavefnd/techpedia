#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

export BENCH_UID=${BENCH_UID:-$(id -u)}
export BENCH_GID=${BENCH_GID:-$(id -g)}
project=${BENCH_PROJECT_NAME:-techpedia-benchmark-${BENCH_UID}}
compose=(docker compose -p "$project" -f compose.yml)

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

for command_name in docker python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Required command not found: $command_name" >&2
    exit 1
  fi
done

mkdir -p results
cleanup

echo "[1/5] Build benchmark images"
"${compose[@]}" build seed engine

echo "[2/5] Seed isolated data"
"${compose[@]}" run --rm seed

echo "[3/5] Start resource-limited engine"
"${compose[@]}" up -d engine
for _ in {1..60}; do
  if "${compose[@]}" exec -T engine wget -q -O /dev/null http://127.0.0.1:8080/api/health 2>/dev/null; then
    break
  fi
  sleep 1
done
if ! "${compose[@]}" exec -T engine wget -q -O /dev/null http://127.0.0.1:8080/api/health 2>/dev/null; then
  echo "Benchmark engine health check failed." >&2
  "${compose[@]}" logs --tail=100 engine >&2
  exit 1
fi

echo "[4/5] Run staged load test"
"${compose[@]}" run --rm loadgen

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
cp results/latest.json "results/result-${timestamp}.json"
echo "[5/5] Render Python result report"
python3 visualize.py results/latest.json results/latest.html
cp results/latest.html "results/report-${timestamp}.html"
echo "Results: $root_dir/results/latest.json"
echo "Report: $root_dir/results/latest.html"
