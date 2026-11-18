import assert from 'node:assert/strict'
import { test } from 'node:test'
import { retryableLoad } from '../src/services/retryableLoad.ts'

test('admin UI loader shares pending work, retries failure and retains success', async () => {
  let calls = 0
  const load = retryableLoad(async () => { if (++calls === 1) throw new Error('chunk unavailable') })
  const first = load(), duplicate = load()
  assert.equal(first, duplicate)
  await assert.rejects(first, /chunk unavailable/)
  assert.equal(calls, 1)
  const retry = load(); await retry
  assert.equal(calls, 2); assert.equal(load(), retry)
})
