# Historical baseline measurements

## 2026-07-18

Isolated local Compose: engine 2 vCPU/1GiB, 1,000 documents, separate same-host
generator with 4,096 workers, 10s stages and 5s warmup. Mix: HTML 40%, API 25%,
search 10%, link resolution 8%, recent changes 7%, editions 5%, view writes 5%.
Pass criteria: p95 ≤1s, error ≤1%, achieved rate ≥95%, zero missed requests.

| Target RPS | Actual RPS | p95 | Error | Result |
| --- | --- | --- | --- | --- |
| 100 | 100 | 143ms | 0% | Pass |
| 250 | 250 | 170ms | 0% | Pass |
| 500 | 374 | 5,011ms | 39.26% | Fail |
| 375 | 370 | 193ms | 0% | Pass |
| 437 | 383 | 3,713ms | 1.58% | Fail |
| 406 | 384 | 1,366ms | 0% | Fail |
| 390 | 383 | 433ms | 0% | Pass |

The short-stage boundary was 390 ≤ sustainable RPS <406. This excludes TLS,
Caddy, Cloudflare and Internet latency and is not production capacity.

## Daily 10× peak model

Same engine limit, 12,000 generator workers:

| Daily model | Target RPS | Actual | p95 | Error | Missed | Result |
| --- | --- | --- | --- | --- | --- | --- |
| 1,002,240 | 116 | 116 | 147ms | 0% | 0 | Pass |
| 10,005,120 | 1,158 | 891 | 3,012ms | 87.70% | 0 | Fail |
| 100,008,000 | 11,575 | 2,736 | 9,882ms | 97.86% | 79,750 | Fail |

The final stage saturated both engine and generator. It is not an accurate
11,575-RPS engine-throughput measurement. Later refinement passed 376 RPS and
failed 506 RPS; the passing stage corresponds to ~3.24m modeled requests/day,
not actual visitors or a guaranteed daily capacity. Re-run matched workloads and
at least a 10-minute lower-rate soak before making operational decisions.
