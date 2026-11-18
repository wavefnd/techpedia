#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

if ! command -v python3 >/dev/null 2>&1; then
  echo "Required command not found: python3" >&2
  exit 1
fi

daily_volumes=${BENCH_DAILY_VOLUMES:-1000000,10000000,100000000}
peak_factor=${BENCH_PEAK_FACTOR:-10}
targets=$(python3 traffic_profile.py --daily "$daily_volumes" --peak-factor "$peak_factor" --format rps)

export BENCH_TARGETS=$targets
export BENCH_PEAK_FACTOR=$peak_factor
export BENCH_PROFILE_NAME=${BENCH_PROFILE_NAME:-daily-extreme}
export BENCH_CONTINUE_AFTER_FAILURE=${BENCH_CONTINUE_AFTER_FAILURE:-true}
export BENCH_STAGE_DURATION=${BENCH_STAGE_DURATION:-10s}
export BENCH_WARMUP_DURATION=${BENCH_WARMUP_DURATION:-5s}
export BENCH_REQUEST_TIMEOUT=${BENCH_REQUEST_TIMEOUT:-3s}
export BENCH_WORKERS=${BENCH_WORKERS:-12000}
export BENCH_LOADGEN_CPUS=${BENCH_LOADGEN_CPUS:-4.0}
export BENCH_LOADGEN_MEMORY=${BENCH_LOADGEN_MEMORY:-3072m}
export BENCH_REFINE_STEPS=${BENCH_REFINE_STEPS:-3}

python3 traffic_profile.py --daily "$daily_volumes" --peak-factor "$peak_factor" --format table
exec "$root_dir/run.sh"
