import { forceCenter, forceCollide, forceLink, forceManyBody, forceSimulation } from 'd3-force'
import type { SimulationLinkDatum, SimulationNodeDatum } from 'd3-force'
import type { PhysicsInput, PhysicsNode, PhysicsSnapshot } from './graphLayout'

type Particle = PhysicsNode & SimulationNodeDatum
type Spring = SimulationLinkDatum<Particle>

export function createGraphPhysics(input: PhysicsInput) {
  const nodes: Particle[] = input.nodes.map((node) => ({ ...node, vx: 0, vy: 0 }))
  const ids = new Map(nodes.map((node, index) => [node.id, index]))
  const pairs = new Map<number, Set<number>>()
  const links: Spring[] = []
  for (const edge of input.edges) {
    if (edge.source === edge.target || !ids.has(edge.source) || !ids.has(edge.target)) continue
    const a = ids.get(edge.source)!, b = ids.get(edge.target)!
    const from = Math.min(a, b), to = Math.max(a, b)
    let targets = pairs.get(from)
    if (targets?.has(to)) continue
    if (!targets) { targets = new Set(); pairs.set(from, targets) }
    targets.add(to)
    links.push({ source: edge.source, target: edge.target })
  }
  const simulation = forceSimulation(nodes).stop()
    .alpha(0.6).alphaTarget(0.025).alphaDecay(0.01).velocityDecay(0.4)
    .force('charge', forceManyBody<Particle>().strength(-80).theta(0.9).distanceMax(600))
    .force('spring', forceLink<Particle, Spring>(links).id((node) => node.id)
      .distance((edge) => 36 + (edge.source as Particle).radius + (edge.target as Particle).radius))
    .force('collision', forceCollide<Particle>((node) => node.radius + 2).strength(0.8))
    // Translation alone does not pull documents towards a particular shape.
    .force('center', forceCenter<Particle>(0, 0).strength(0.05))
    .force('speedLimit', () => {
      for (const node of nodes) {
        const speed = Math.hypot(node.vx!, node.vy!)
        if (speed > 8) { node.vx! *= 8 / speed; node.vy! *= 8 / speed }
      }
    })
  let quiet = 0
  const before = new Float64Array(nodes.length * 2)
  return {
    nodes, links,
    step(recycled?: Float32Array) {
      nodes.forEach((node, index) => { before[index * 2] = node.x; before[index * 2 + 1] = node.y })
      simulation.tick()
      let movement = 0
      const positions = recycled?.length === nodes.length * 2 ? recycled : new Float32Array(nodes.length * 2)
      for (let index = 0; index < nodes.length; index++) {
        const node = nodes[index]!
        if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) throw new Error('invalid graph position')
        movement = Math.max(movement, Math.hypot(node.x - before[index * 2]!, node.y - before[index * 2 + 1]!))
        positions[index * 2] = node.x
        positions[index * 2 + 1] = node.y
      }
      quiet = movement < 0.08 ? quiet + 1 : 0
      return { positions, stable: quiet >= 60 }
    },
    resume() { quiet = 0 },
  }
}

export function createPackedGraphPhysics(input: PhysicsSnapshot) {
  const nodes: PhysicsNode[] = []
  for (let index = 0; index < input.nodes.length; index += 3) {
    nodes.push({ id: String(index / 3), x: input.nodes[index]!, y: input.nodes[index + 1]!, radius: input.nodes[index + 2]! })
  }
  const edges: PhysicsInput['edges'] = []
  for (let index = 0; index < input.edges.length; index += 2) {
    edges.push({ source: String(input.edges[index]), target: String(input.edges[index + 1]), kind: 'link' })
  }
  return createGraphPhysics({ nodes, edges, interval: input.interval })
}

// Leave proportional idle time; quiet graphs continue at no more than 2 Hz.
export function graphPhysicsDelay(interval: number, elapsed: number, stable: boolean): number {
  return Math.max(interval, stable ? 500 : 0, elapsed * 5)
}
