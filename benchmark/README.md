# Isolated load benchmark

This is a separate Compose project/network with a disposable BadgerDB volume.
It does not use production data. The default fixture has 1,000 roughly 4KiB
TechMark documents; the engine is limited to 2 vCPU/1GiB. A separate Go load
generator sends mixed HTML/API/search/link/recent-change/language/view traffic.
Python provides profiles and a standalone HTML/SVG report without external packages.

## Run

The load generator's Go test sources are stored separately in
`tests/go/benchmark/`. Run `./tests/go.sh --module benchmark test ./...` from the
repository root to check them without starting any load test or containers.

Requires Docker Engine/Compose and Python 3:

```sh
cd benchmark
./run.sh
```

Default stages are 100, 250, 500, 1,000, 2,000, 4,000 and 8,000 RPS at 20s each.
After the first failed stage, four binary-refinement steps narrow the boundary;
set `BENCH_REFINE_STEPS` to change this. Results are saved in
`results/latest.json`, `results/latest.html` and timestamped copies. Cleanup removes
only benchmark containers/network/volume. Do not point this tool at production.

```sh
BENCH_TARGETS=50,100 BENCH_STAGE_DURATION=5s BENCH_WARMUP_DURATION=2s ./run.sh
python3 visualize.py results/latest.json results/latest.html
```

## Daily-volume profile

`./extreme.sh` models 1m/10m/100m requests/day at a default 10× peak, corresponding
to 116/1,158/11,575 target RPS. It continues after failures, then refines.

```sh
BENCH_DAILY_VOLUMES=10m,100m,1b BENCH_PEAK_FACTOR=5 ./extreme.sh
```

Requests are not visitors. Actual per-visit request counts depend on cache and
feature mix. Increasing load also requires generator CPU/memory/workers.

A stage passes only when configured latency/error/achieved-rate/missed-request
criteria pass. Short stages, same-host networking, cache warmth and generator
saturation limit conclusions. Use long soak tests on matched hardware before
planning capacity; do not claim a production SLA from these fixtures.
See [historical results](RESULTS.md).
