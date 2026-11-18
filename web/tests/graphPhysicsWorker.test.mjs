import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
import { graphPhysicsDelay } from '../src/services/graphPhysics.ts'

function setup() {
  const timers = new Map(), frames = [], self = {}
  let id = 0, clock = 0, stable = false, failed = false
  const source = readFileSync(new URL('../src/workers/graphPhysics.ts', import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const physics = { resume() {}, step() { clock += 4; if (failed) throw Error('test'); return { positions: Float32Array.from([1, 2]), stable } } }
  self.postMessage = (data) => frames.push(data)
  new Function('require', 'exports', 'self', 'setTimeout', 'clearTimeout', 'performance', code)(
    () => ({ createPackedGraphPhysics: () => physics, graphPhysicsDelay }), {}, self,
    (callback, delay) => { const key = ++id; timers.set(key, { callback, delay }); return key },
    (key) => timers.delete(key), { now: () => clock },
  )
  const send = (data) => self.onmessage({ data })
  send({ type: 'init', input: { nodes: [], edges: [], interval: 66 } })
  return { send, timers, frames, quiet: () => { stable = true }, fail: () => { failed = true }, tick() { const [key, timer] = timers.entries().next().value; timers.delete(key); timer.callback() } }
}

test('worker waits for frame acknowledgement and paces quiet motion instead of freezing', () => {
  const screen = setup()
  screen.send({ type: 'start', epoch: 1 }); screen.tick()
  assert.equal(screen.frames.length, 1); assert.equal(screen.timers.size, 0)
  screen.send({ type: 'ack', epoch: 0 }); assert.equal(screen.timers.size, 0)
  screen.send({ type: 'ack', epoch: 1 }); assert.equal([...screen.timers.values()][0].delay, 66)
  screen.quiet(); screen.tick()
  assert.equal(screen.frames[1].stable, true)
  screen.send({ type: 'ack', epoch: 1 }); assert.equal([...screen.timers.values()][0].delay, 500)
  screen.send({ type: 'ack', epoch: 1 }); assert.equal(screen.timers.size, 1)
})

test('pause cancels scheduled work and stale acknowledgements cannot restart it', () => {
  const screen = setup()
  screen.send({ type: 'start', epoch: 1 }); screen.tick()
  screen.send({ type: 'pause', epoch: 2 }); screen.send({ type: 'ack', epoch: 1 })
  assert.equal(screen.timers.size, 0)
  screen.send({ type: 'start', epoch: 3 }); screen.tick()
  screen.send({ type: 'ack', epoch: 1 }); assert.equal(screen.timers.size, 0)
  screen.send({ type: 'ack', epoch: 3 }); assert.equal(screen.timers.size, 1)
  screen.send({ type: 'pause', epoch: 4 }); assert.equal(screen.timers.size, 0)
})

test('expensive UI application backs off without omitting coordinates', () => {
  const screen = setup()
  screen.send({ type: 'start', epoch: 1 }); screen.tick()
  screen.send({ type: 'ack', epoch: 1, positions: screen.frames[0].positions, elapsed: 80 })
  assert.equal([...screen.timers.values()][0].delay, 400)
  screen.tick()
  assert.equal(screen.frames.length, 2)
})

test('numerical failure reports only a generic failure and schedules no more work', () => {
  const screen = setup()
  screen.fail(); screen.send({ type: 'start', epoch: 1 }); screen.tick()
  assert.equal(screen.frames[0].failed, true); assert.equal(screen.timers.size, 0)
  screen.send({ type: 'ack', epoch: 1 }); assert.equal(screen.timers.size, 0)
})
