import assert from 'node:assert/strict'
import { test } from 'node:test'
import { GraphLayoutSession } from '../src/services/graphLayout.ts'
import { reactive } from 'vue'

function setup() {
  const states = [], updates = [], commands = []
  let worker
  globalThis.Worker = class {
    constructor(url, options) { worker = this; assert.match(url.pathname, /graphPhysics.ts$/); assert.equal(options.type, 'module') }
    postMessage(command) { commands.push(structuredClone(command)) }
    terminate() { this.terminated = true }
    send(epoch, positions = Float32Array.from([10, 20]), stable = false) { this.onmessage({ data: { epoch, positions, stable } }) }
  }
  const session = new GraphLayoutSession(reactive({ nodes: [{ id: 'a', x: 0, y: 0, radius: 3 }], edges: [], interval: 33 }), (p) => updates.push(p), (s) => states.push(s))
  return { session, worker, states, updates, commands, stop() { session.dispose(); delete globalThis.Worker } }
}

test('only active epoch frames reach the UI, including quiet continuous frames', () => {
  const screen = setup()
  try {
    screen.session.start()
    screen.worker.send(1); assert.equal(screen.updates.length, 1)
    screen.session.pause(); screen.worker.send(1)
    assert.equal(screen.updates.length, 1)
    screen.session.start(); screen.worker.send(1)
    assert.equal(screen.updates.length, 1)
    screen.worker.send(3, Float32Array.from([30, 40]), true)
    assert.equal(screen.states.at(-1), 'stable'); assert.equal(screen.updates.length, 2)
    screen.worker.send(3, Float32Array.from([31, 41]), true)
    assert.equal(screen.updates.length, 3)
  } finally { screen.stop() }
})

test('initial Worker snapshot uses transferable numeric arrays, not document identifiers', () => {
  const screen = setup()
  try {
    const snapshot = screen.commands[0].input
    assert.ok(snapshot.nodes instanceof Float32Array)
    assert.ok(snapshot.edges instanceof Uint32Array)
    assert.deepEqual([...snapshot.nodes], [0, 0, 3])
    assert.equal(JSON.stringify(snapshot).includes('"a"'), false)
    screen.session.start()
    screen.worker.send(1)
    const ack = screen.commands.at(-1)
    assert.equal(ack.type, 'ack')
    assert.deepEqual([...ack.positions], [10, 20])
    assert.ok(ack.elapsed >= 0)
  } finally { screen.stop() }
})

test('pause, hidden, resume and disposal invalidate old frames without resetting coordinates', () => {
  const screen = setup()
  try {
    screen.session.start(); screen.session.start()
    assert.equal(screen.commands.length, 2)
    screen.session.pause('hidden'); assert.equal(screen.states.at(-1), 'hidden')
    screen.session.start()
    assert.equal(screen.commands.filter((c) => c.type === 'init').length, 1)
    screen.session.dispose(); screen.session.start(); screen.worker.send(3)
    assert.equal(screen.worker.terminated, true); assert.equal(screen.updates.length, 0)
    assert.equal(screen.commands.at(-1).type, 'start')
  } finally { screen.stop() }
})

test('invalid output or worker errors terminate calculation and leave a failure state', () => {
  for (const failure of ['invalid', 'error', 'messageerror']) {
    const screen = setup()
    try {
      screen.session.start()
      if (failure === 'invalid') screen.worker.send(1, Float32Array.from([NaN, 0]))
      else screen.worker[failure === 'error' ? 'onerror' : 'onmessageerror']()
      assert.equal(screen.states.at(-1), 'failed'); assert.equal(screen.worker.terminated, true)
      assert.equal(screen.updates.length, 0)
    } finally { screen.stop() }
  }
})
