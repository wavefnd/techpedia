import { createGraphPhysics } from '../src/services/graphPhysics.ts'

// Diagnostic only: synthetic four-link topology, five warmup + twenty timed steps.
for (const count of [2000, 10000, 50000]) {
  const nodes = Array.from({ length: count }, (_, index) => ({
    id: String(index), radius: 5,
    x: Math.sin(index * 2.399) * Math.sqrt(index + 1) * 35,
    y: Math.cos(index * 2.399) * Math.sqrt(index + 1) * 35,
  }))
  const edges = []
  for (let index = 0; index < count; index++) for (let offset = 1; offset <= 4; offset++) {
    edges.push({ source: String(index), target: String((index + offset) % count), kind: 'link' })
  }
  const started = performance.now()
  const physics = createGraphPhysics({ nodes, edges, interval: 33 })
  const init = performance.now() - started
  let positions = new Float32Array(count * 2)
  const times = []
  for (let step = 0; step < 25; step++) {
    const begin = performance.now()
    positions = physics.step(positions).positions
    if (step >= 5) times.push(performance.now() - begin)
  }
  times.sort((a, b) => a - b)
  console.log(JSON.stringify({ nodes: count, springs: physics.links.length, init_ms: +init.toFixed(2),
    step_p50_ms: +times[10].toFixed(2), step_p95_ms: +times[18].toFixed(2), frame_bytes: positions.byteLength }))
}
