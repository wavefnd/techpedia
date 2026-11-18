import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { test } from 'node:test'
import { auditActionNames, auditActionName, auditResult } from '../src/services/adminAudit.ts'

test('all actual Go audit events have Korean labels', () => {
  const server = new URL('../../internal/server/', import.meta.url)
  const source = readdirSync(server).filter((name) => name.endsWith('.go') && !name.endsWith('_test.go')).map((name) => readFileSync(new URL(name, server), 'utf8')).join('\n')
  const service = readFileSync(new URL('../../internal/oauth/service.go', import.meta.url), 'utf8')
  const actions = [...source.matchAll(/recordAudit\([^\n]*?,\s*"([a-z_]+\.[a-z_]+)"/g)].map((match) => match[1])
  actions.push(...[...service.matchAll(/"(oauth\.[a-z_]+)"/g)].map((match) => match[1]))
  assert.ok(actions.length > 20)
  for (const action of actions) assert.ok(auditActionNames[action], `Missing label: ${action}`)
})
test('future events and non-success outcomes have explicit labels', () => {
  assert.equal(auditActionName('future.change'), '알 수 없는 작업')
  assert.equal(auditResult('blocked').text, '차단')
  assert.equal(auditResult('attempt').text, '요청')
  assert.equal(auditResult('failed').text, '실패')
  assert.equal(auditResult('failure').text, '실패')
  assert.equal(auditResult('future-result').text, '알 수 없음')
})
