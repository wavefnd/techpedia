import type { GraphResponse } from '../types/api'

export type GraphLayoutState = 'running' | 'stable' | 'paused' | 'hidden' | 'failed'
export type PhysicsNode = { id: string; x: number; y: number; radius: number }
export type PhysicsInput = { nodes: PhysicsNode[]; edges: GraphResponse['edges']; interval: number }
export type PhysicsSnapshot = { nodes: Float32Array; edges: Uint32Array; interval: number }
export type PhysicsCommand = { type: 'init'; input: PhysicsSnapshot } | { type: 'start' | 'pause'; epoch: number } | { type: 'ack'; epoch: number; positions?: Float32Array; elapsed?: number }
export type PhysicsFrame = { epoch: number; positions: Float32Array; stable: boolean; failed?: boolean }

export class GraphLayoutSession {
  private worker: Worker
  private epoch = 0
  private running = false
  private disposed = false
  private update: (positions: Float32Array) => void
  private onState: (state: GraphLayoutState) => void
  constructor(input: PhysicsInput, update: (positions: Float32Array) => void, onState: (state: GraphLayoutState) => void) {
    const coordinateCount = input.nodes.length * 2
    this.update = update
    this.onState = onState
    this.worker = new Worker(new URL('../workers/graphPhysics.ts', import.meta.url), { type: 'module' })
    this.worker.onmessage = ({ data }: MessageEvent<PhysicsFrame>) => {
      if (this.disposed || !this.running || data.epoch !== this.epoch) return
      if (data.failed || data.positions?.length !== coordinateCount || !data.positions.every(Number.isFinite)) { this.fail(); return }
      try {
        const started = performance.now()
        this.update(data.positions)
        this.onState(data.stable ? 'stable' : 'running')
        // Return ownership for reuse instead of allocating another N*8 bytes/frame.
        this.worker.postMessage({ type: 'ack', epoch: this.epoch, positions: data.positions, elapsed: performance.now() - started } satisfies PhysicsCommand, [data.positions.buffer])
      } catch { this.fail() }
    }
    this.worker.onerror = () => this.fail()
    this.worker.onmessageerror = () => this.fail()
    // Only numeric coordinates/radii and endpoint indexes cross the boundary.
    // Titles/URLs and per-edge strings stay out of the Worker snapshot.
    const nodes = new Float32Array(input.nodes.length * 3)
    const indexes = new Map<string, number>()
    input.nodes.forEach(({ id, x, y, radius }, index) => {
      indexes.set(id, index)
      nodes[index * 3] = x
      nodes[index * 3 + 1] = y
      nodes[index * 3 + 2] = radius
    })
    const edges = new Uint32Array(input.edges.length * 2)
    let count = 0
    for (const { source, target } of input.edges) {
      const from = indexes.get(source), to = indexes.get(target)
      if (from === undefined || to === undefined || from === to) continue
      edges[count++] = from
      edges[count++] = to
    }
    const snapshot: PhysicsSnapshot = { nodes, edges: edges.subarray(0, count), interval: input.interval }
    try { this.worker.postMessage({ type: 'init', input: snapshot } satisfies PhysicsCommand, [nodes.buffer, edges.buffer]) }
    catch { this.fail() }
  }

  start() {
    if (this.disposed || this.running) return
    this.running = true
    try { this.worker.postMessage({ type: 'start', epoch: ++this.epoch } satisfies PhysicsCommand) }
    catch { this.fail(); return }
    this.onState('running')
  }

  pause(state: 'paused' | 'hidden' = 'paused') {
    if (this.disposed) return
    this.running = false
    try { this.worker.postMessage({ type: 'pause', epoch: ++this.epoch } satisfies PhysicsCommand) }
    catch { this.fail(); return }
    this.onState(state)
  }

  dispose() {
    if (this.disposed) return
    this.running = false
    this.disposed = true
    this.worker.terminate()
  }

  private fail() {
    if (this.disposed) return
    this.dispose()
    this.onState('failed')
  }
}
