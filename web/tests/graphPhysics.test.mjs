import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createGraphPhysics, createPackedGraphPhysics, graphPhysicsDelay } from '../src/services/graphPhysics.ts'

const node = (id, x, y, radius = 8) => ({ id, x, y, radius })
const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y)
const run = (physics, count) => { let frame; for (let i = 0; i < count; i++) frame = physics.step(); return frame }

test('springs pull linked nodes closer while unlinked nodes repel without prescribed destinations', () => {
  const nodes = [node('a', -200, 0), node('b', 200, 0)]
  const linked = createGraphPhysics({ nodes, edges: [{ source: 'a', target: 'b', kind: 'link' }], interval: 33 })
  const separate = createGraphPhysics({ nodes, edges: [], interval: 33 })
  run(linked, 400); run(separate, 400)
  assert.ok(distance(...linked.nodes) < 150)
  assert.ok(distance(...separate.nodes) > 400)
  assert.deepEqual(nodes, [node('a', -200, 0), node('b', 200, 0)])
})

test('soft collision separates overlapping nodes and damping makes a small system quiet, not frozen', () => {
  const physics = createGraphPhysics({ nodes: [node('a', -1, 0, 20), node('b', 1, 0, 20)], edges: [{ source: 'a', target: 'b', kind: 'link' }], interval: 33 })
  const frame = run(physics, 1200)
  assert.ok(distance(...physics.nodes) >= 40)
  assert.equal(frame.stable, true)
  assert.equal(physics.step().positions.length, 4)
  physics.resume(); assert.equal(physics.step().stable, false)
})

test('self, invalid and repeated reciprocal links do not create stronger springs', () => {
  const nodes = [node('a', -100, 0), node('b', 100, 0)]
  const edges = [{ source: 'a', target: 'b', kind: 'link' }]
  const input = { nodes, edges, interval: 33 }
  const a = createGraphPhysics(input)
  const b = createGraphPhysics({ ...input, edges: [...edges, ...edges, { source: 'b', target: 'a', kind: 'translation' }, { source: 'a', target: 'a' }, { source: 'a', target: 'missing' }] })
  assert.equal(b.links.length, 1)
  assert.deepEqual(run(a, 100).positions, run(b, 100).positions)
})

test('dense connected areas emerge through springs without a community classifier', () => {
  const nodes = Array.from({ length: 12 }, (_, i) => node(String(i), Math.sin(i * 3) * 150, Math.cos(i * 7) * 150))
  const edges = []
  for (const start of [0, 6]) for (let i = start; i < start + 6; i++) for (let j = i + 1; j < start + 6; j++) edges.push({ source: String(i), target: String(j), kind: 'link' })
  edges.push({ source: '5', target: '6', kind: 'link' })
  const physics = createGraphPhysics({ nodes, edges, interval: 33 })
  run(physics, 1500)
  const centroid = (group) => ({ x: group.reduce((s, n) => s + n.x, 0) / group.length, y: group.reduce((s, n) => s + n.y, 0) / group.length })
  const a = physics.nodes.slice(0, 6), b = physics.nodes.slice(6)
  const ca = centroid(a), cb = centroid(b)
  const within = [...a.map((n) => distance(n, ca)), ...b.map((n) => distance(n, cb))].reduce((s, d) => s + d, 0) / 12
  assert.ok(distance(ca, cb) > within * 2)
})

test('frame pacing reduces CPU without an elapsed-time stop and empty input is safe', () => {
  assert.equal(graphPhysicsDelay(33, 1, false), 33)
  assert.equal(graphPhysicsDelay(66, 50, false), 250)
  assert.equal(graphPhysicsDelay(33, 2, true), 500)
  assert.equal(graphPhysicsDelay(1000, 2, true), 1000)
  assert.equal(createGraphPhysics({ nodes: [], edges: [], interval: 33 }).step().positions.length, 0)
})

test('ten thousand nodes remain in the simulation and coordinate storage is recycled', () => {
  const count = 10000
  const nodes = Array.from({ length: count }, (_, index) => node(String(index), (index % 100) * 30, Math.floor(index / 100) * 30))
  const edges = nodes.slice(1).map((entry, index) => ({ source: String(index), target: entry.id, kind: 'link' }))
  const physics = createGraphPhysics({ nodes, edges, interval: 33 })
  const buffer = new Float32Array(count * 2)
  const frame = physics.step(buffer)
  assert.equal(physics.nodes.length, count)
  assert.equal(physics.links.length, count - 1)
  assert.equal(frame.positions, buffer)
  assert.ok(frame.positions.every(Number.isFinite))
  assert.equal(physics.step(buffer).positions, buffer)
})

test('packed Worker input preserves coordinates, radii and deduplicated connectivity', () => {
  const physics = createPackedGraphPhysics({ nodes: Float32Array.from([-100, 0, 8, 100, 0, 12]), edges: Uint32Array.from([0, 1, 1, 0]), interval: 33 })
  assert.deepEqual(physics.nodes.map(({ x, y, radius }) => [x, y, radius]), [[-100, 0, 8], [100, 0, 12]])
  assert.equal(physics.links.length, 1)
  assert.equal(physics.step().positions.length, 4)
})
