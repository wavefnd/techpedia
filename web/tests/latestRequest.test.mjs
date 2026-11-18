import assert from 'node:assert/strict'
import { test } from 'node:test'
import { LatestRequest } from '../src/services/latestRequest.ts'

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function state() {
  const value = { data: null, error: null, settled: 0 }
  return { value, handlers: { success: (data) => { value.data = data }, error: (error) => { value.error = error }, settled: () => { value.settled++ } } }
}

test('latest search or analytics period wins when responses finish in reverse order', async () => {
  const requests = new LatestRequest(), old = deferred(), next = deferred(), output = state()
  const a = requests.run(() => old.promise, output.handlers)
  const b = requests.run(() => next.promise, output.handlers)
  next.resolve('90 days'); await b
  old.resolve('7 days'); await a
  assert.deepEqual(output.value, { data: '90 days', error: null, settled: 1 })
})
test('input invalidation blocks old response during debounce before next fetch', async () => {
  const requests = new LatestRequest(), old = deferred(), output = state()
  const a = requests.run(() => old.promise, output.handlers)
  requests.invalidate(); old.resolve('old locale'); await a
  assert.deepEqual(output.value, { data: null, error: null, settled: 0 })
})
test('closed and reopened Techdata dialog rejects old search error and finally', async () => {
  const requests = new LatestRequest(), old = deferred(), next = deferred(), output = state()
  const a = requests.run(() => old.promise, output.handlers)
  requests.invalidate()
  const b = requests.run(() => next.promise, output.handlers)
  old.reject(new Error('old page failed')); await a
  assert.deepEqual(output.value, { data: null, error: null, settled: 0 })
  next.resolve('new page candidates'); await b
  assert.equal(output.value.data, 'new page candidates'); assert.equal(output.value.settled, 1)
})
test('duplicate candidate warning is not replaced by an invalidated search', async () => {
  const requests = new LatestRequest(), pending = deferred(), output = state()
  const a = requests.run(() => pending.promise, output.handlers)
  requests.invalidate(); output.value.data = 'duplicates'
  pending.resolve('search results'); await a
  assert.equal(output.value.data, 'duplicates')
})
test('unmount blocks success, errors, finally and future work', async () => {
  const requests = new LatestRequest(), pending = deferred(), output = state()
  const a = requests.run(() => pending.promise, output.handlers)
  requests.dispose(); pending.reject(new Error('late failure')); await a
  let started = false
  await requests.run(async () => { started = true }, output.handlers)
  assert.equal(started, false)
  assert.deepEqual(output.value, { data: null, error: null, settled: 0 })
})
test('current API failure is visible and settles once, then retry succeeds', async () => {
  const requests = new LatestRequest(), output = state(), failure = new Error('API offline')
  await requests.run(async () => { throw failure }, output.handlers)
  assert.equal(output.value.error, failure); assert.equal(output.value.settled, 1)
  output.value.error = null
  await requests.run(async () => 'recovered', output.handlers)
  assert.deepEqual(output.value, { data: 'recovered', error: null, settled: 2 })
})

test('invalidation and unmount abort the actual browser request signal', async () => {
  const requests = new LatestRequest(), output = state()
  let signal
  const pending = deferred()
  const work = requests.run((value) => { signal = value; return pending.promise }, output.handlers)
  assert.equal(signal.aborted, false)
  requests.invalidate(); assert.equal(signal.aborted, true)
  pending.resolve('old'); await work
  let nextSignal
  const next = requests.run(async (value) => { nextSignal = value; return 'new' }, output.handlers)
  requests.dispose(); await next
  assert.equal(nextSignal.aborted, true)
})
