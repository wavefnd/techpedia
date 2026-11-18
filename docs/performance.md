# Performance and measurement

TechPedia targets shared-CPU VPSs, slow networks and low-cost phones. Article text
must be readable before JavaScript. Optional tools stay out of ordinary reading
dependencies.

## Low-end regression profile

Use 360×640 at DPR 2, 400ms RTT, 400kbps download/upload, Chromium 6× CPU throttle,
cold cache and reduced motion. This is not an exact device simulation.

| Metric | Budget |
| --- | --- |
| FCP | ≤8s |
| Local uncompressed same-origin JavaScript | ≤240 KiB |
| CLS | ≤0.1 |
| Longest task under 6× throttle | ≤1s |
| Horizontal overflow | ≤1px |
| Readable body | >20 characters |

After building and serving an isolated article:

```sh
node tests/performance/low-end-audit.mjs 'http://127.0.0.1:18090/en/wiki/Example'
node tests/performance/low-end-audit.mjs --desktop 'http://127.0.0.1:18090/en/wiki/Example'
```

Set `CHROMIUM_PATH` if needed. The CDP script reports JSON. Test only your own
service and fixtures.

## Boundaries

Preserve SSR DOM through Vue mounting and reading/discussion changes. Public reads
do not wait for auth. Protected routes check permission before admin imports.
Cancel stale requests and ignore late generations on article/history changes.
Read headings from actual DOM; resolve/decorate links in yielding bounded batches.
Defer offscreen layout without removing source, anchors or print content.
Editor/admin/graph/QR/gallery code loads on demand. Bound caches and cold-render
work separately. Use responsive media variants and lazy/async image decoding.

## Historical evidence, not an SLA

A 2026-07-18 local 7,266-character/80-heading fixture measured FCP 5,340ms,
JavaScript 166 KiB, CLS 0.004 and longest task 926ms under the low-end profile.
A later production article with ~30,000 DOM nodes failed task budgets: DOM/style
cost, not only transfer size, mattered.

October 2026 warm microbenchmarks on Apple M4/macOS arm64, CGO disabled:

| Fixture | Baseline → improved | Cumulative allocation |
| --- | --- | --- |
| 8 summaries / 500 revisions | 76.634 → 0.063ms | ~108 MiB → 13.6 KiB |
| 8 search results / 500 matches | 5.006 → 0.095ms | ~8.14 MiB → 16.2 KiB |
| Warm render, 276KB source | 23.417 → 0.182ms | ~24.6 MiB → 16B |

Allocation is not peak RSS. Short-source latency did not improve in every case.
Search still scans candidates; it is not a persistent full-text inverted index.

```sh
CGO_ENABLED=0 go test ./internal/storage ./internal/techmark -run '^$' \
  -bench '^Benchmark(RevisionSummaryRead|WarmDocumentSearch|WarmArticleRender)$' \
  -benchmem -benchtime=200ms -count=3
```

A separate synthetic 239,019-character/20,835-node article measured FCP 7,468ms
and longest task 481ms after offscreen layout work. It was not the same production
article and does not establish deployed performance.

## Graph scaling

The public graph defaults to all document metadata, not a 1,500-node slice.
It avoids the bot analyzer's dense PageRank. Browser Worker physics uses recycled
numeric buffers and decreases update frequency as work grows. Full JSON transfer
and GPU memory still scale with size; unlimited nodes/FPS are not promised.

Historical synthetic server construction (excluding DB, JSON and HTTP):

| Documents / directed links | Time | Cumulative allocation |
| --- | --- | --- |
| 2,000 / 4,000 | 1.95ms | 2.19 MiB |
| 10,000 / 20,000 | 10.05ms | 12.15 MiB |
| 50,000 / 100,000 | 60.75ms | 61.06 MiB |

Node physics medians were 6.30/37.57/229.96ms for 2k/10k/50k nodes, not full browser
frames. A synthetic 10,001-document fixture was checked at desktop and 390px widths.
Those historical checks are not current real-phone verification.

```sh
go test ./internal/server -run '^$' -bench '^BenchmarkGraphSnapshot$' -benchmem -benchtime=3x
node --experimental-strip-types web/scripts/benchmark-graph.mjs
```

See `benchmark/README.md` for isolated load tests. Short synthetic stages are not
production capacity; requests are not unique visitors.
