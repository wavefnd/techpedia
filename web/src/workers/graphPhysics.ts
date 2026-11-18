import { createPackedGraphPhysics, graphPhysicsDelay } from '../services/graphPhysics'
import type { PhysicsCommand, PhysicsFrame } from '../services/graphLayout'

let physics: ReturnType<typeof createPackedGraphPhysics> | undefined
let timer: ReturnType<typeof setTimeout> | undefined
let epoch = 0
let running = false
let interval = 33
let pending = false
let delay = 33
let recycled: Float32Array | undefined

function tick() {
  if (!running || !physics) return
  const started = performance.now()
  try {
    const result = physics.step(recycled)
    recycled = undefined
    delay = graphPhysicsDelay(interval, performance.now() - started, result.stable)
    pending = true
    self.postMessage({ ...result, epoch } satisfies PhysicsFrame, { transfer: [result.positions.buffer] })
  } catch {
    running = false
    self.postMessage({ epoch, positions: new Float32Array(), stable: false, failed: true } satisfies PhysicsFrame)
  }
}

self.onmessage = ({ data }: MessageEvent<PhysicsCommand>) => {
  if (data.type === 'ack') {
    if (running && pending && data.epoch === epoch) {
      recycled = data.positions
      pending = false
      // Also back off when applying a frame is expensive on the main thread.
      timer = setTimeout(tick, Math.max(delay, Math.min(5000, Math.max(0, data.elapsed || 0)) * 5))
    }
    return
  }
  clearTimeout(timer)
  pending = false
  if (data.type === 'init') {
    running = false
    recycled = undefined
    try { physics = createPackedGraphPhysics(data.input); interval = Math.max(33, data.input.interval) }
    catch { physics = undefined }
    return
  }
  epoch = data.epoch
  running = data.type === 'start'
  if (running) {
    if (!physics) { self.postMessage({ epoch, positions: new Float32Array(), stable: false, failed: true } satisfies PhysicsFrame); return }
    physics.resume()
    timer = setTimeout(tick, 0)
  }
}
