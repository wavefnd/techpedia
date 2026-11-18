import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, symlinkSync, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const source = resolve(import.meta.dirname, '../..')
function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'techpedia-runtime-test-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  mkdirSync(join(dir, 'tests'))
  mkdirSync(join(dir, 'tests/lib'))
  writeFileSync(join(dir, 'tests/server.sh'), readFileSync(join(source, 'tests/server.sh')))
  writeFileSync(join(dir, 'tests/lib/build-lock.sh'), readFileSync(join(source, 'tests/lib/build-lock.sh')))
  return dir
}
function run(dir, args, extra = {}) {
  const env = { ...process.env }
  for (const key of Object.keys(env)) if (key.startsWith('TECHPEDIA_')) delete env[key]
  return spawnSync('bash', ['tests/server.sh', ...args], { cwd: dir, encoding: 'utf8', env: { ...env, ...extra } })
}

test('test server reports only loopback and isolated paths without reading root env', t => {
  const dir = fixture(t)
  writeFileSync(join(dir, '.env'), 'invalid production configuration\n')
  const r = run(dir, ['--check'], { TECHPEDIA_ADDR: '0.0.0.0:8080', TECHPEDIA_DATA_DIR: '/production/data', TECHPEDIA_PUBLIC_URL: 'https://example.invalid' })
  assert.equal(r.status, 0, r.stderr)
  assert.match(r.stdout, /127\.0\.0\.1:18090/)
  assert.ok(r.stdout.includes(join(dir, '.local/test/data')))
  assert.ok(!r.stdout.includes('/production/data'))
  assert.equal(existsSync(join(dir, 'data')), false)
})

test('test executable cannot inherit production credentials, integration flags or arbitrary env', t => {
  const dir = fixture(t), binary = join(dir, '.local/test/bin')
  mkdirSync(binary, { recursive: true })
  writeFileSync(join(binary, 'techpedia'), '#!/usr/bin/env bash\nprintf "%s\\n" "$TECHPEDIA_ADDR" "$TECHPEDIA_DATA_DIR" "$TECHPEDIA_PUBLIC_URL" "$TECHPEDIA_MCP_ENABLED" "$TECHPEDIA_INTERFACE_AUTO_TRANSLATION" "$TECHPEDIA_ANALYTICS_ENABLED" "$TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME" "${PRODUCTION_SENTINEL-unset}" "${TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD-unset}"\n', { mode: 0o755 })
  const r = run(dir, ['--no-build'], {
    TECHPEDIA_TEST_PORT: '18123', TECHPEDIA_ADDR: '0.0.0.0:8080',
    TECHPEDIA_DATA_DIR: '/production/data', TECHPEDIA_PUBLIC_URL: 'https://example.invalid',
    TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME: 'production-owner',
    TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD: 'production-only-placeholder',
    TECHPEDIA_MCP_ENABLED: 'true', TECHPEDIA_INTERFACE_AUTO_TRANSLATION: 'true',
    PRODUCTION_SENTINEL: 'must-not-inherit',
  })
  assert.equal(r.status, 0, r.stderr)
  const lines = readFileSync(join(dir, '.local/test/server.log'), 'utf8').split('\n')
  assert.deepEqual(lines.slice(0, 9), ['127.0.0.1:18123', join(dir, '.local/test/data'), 'http://127.0.0.1:18123', 'false', 'false', 'false', '', 'unset', ''])
  assert.equal(existsSync(join(dir, 'data')), false)
})

for (const port of ['0', '80', '65536', '18090x', '1;touch marker']) {
  test(`test server rejects invalid port ${JSON.stringify(port)}`, t => {
    const dir = fixture(t)
    const r = run(dir, ['--check'], { TECHPEDIA_TEST_PORT: port })
    assert.equal(r.status, 2)
    assert.equal(existsSync(join(dir, 'marker')), false)
  })
}

test('test-owner credentials must be explicitly paired and never printed', t => {
  const dir = fixture(t)
  assert.equal(run(dir, ['--check'], { TECHPEDIA_TEST_OWNER_USERNAME: 'test-owner' }).status, 2)
  const r = run(dir, ['--check'], { TECHPEDIA_TEST_OWNER_USERNAME: 'test-owner', TECHPEDIA_TEST_OWNER_PASSWORD: 'local-only-placeholder' })
  assert.equal(r.status, 0, r.stderr)
  assert.ok(!r.stdout.includes('local-only-placeholder'))
  assert.ok(!r.stderr.includes('local-only-placeholder'))
})

test('test server rejects symlinked runtime and nested data instead of touching other data', t => {
  const dir = fixture(t), elsewhere = join(dir, 'sentinel')
  mkdirSync(elsewhere)
  writeFileSync(join(elsewhere, 'keep'), 'unchanged')
  symlinkSync(elsewhere, join(dir, '.local'))
  assert.equal(run(dir, ['--check']).status, 1)
  rmSync(join(dir, '.local'))
  mkdirSync(join(dir, '.local/test/data'), { recursive: true })
  symlinkSync(elsewhere, join(dir, '.local/test/data/badger'))
  assert.equal(run(dir, ['--check']).status, 1)
  assert.equal(readFileSync(join(elsewhere, 'keep'), 'utf8'), 'unchanged')
})

test('test server refuses a missing binary and unknown options without starting a service', t => {
  const dir = fixture(t)
  assert.equal(run(dir, ['--no-build']).status, 1)
  assert.equal(run(dir, ['--unknown']).status, 2)
  assert.equal(run(dir, ['--check', 'extra']).status, 2)
})

test('test builds refuse an existing lock without deleting it or invoking npm', t => {
  const dir = fixture(t), bin = join(dir, 'bin')
  mkdirSync(bin)
  mkdirSync(join(dir, '.local/test/build.lock'), { recursive: true })
  writeFileSync(join(bin, 'npm'), '#!/usr/bin/env sh\ntouch unexpected-build\n', { mode: 0o755 })
  const r = run(dir, ['--build-only'], { PATH: bin + ':' + process.env.PATH })
  assert.equal(r.status, 1)
  assert.match(r.stderr, /Another test build/)
  assert.equal(existsSync(join(dir, 'unexpected-build')), false)
  assert.equal(existsSync(join(dir, '.local/test/build.lock')), true)
})

test('failed test builds release only their own empty lock', t => {
  const dir = fixture(t), bin = join(dir, 'bin')
  mkdirSync(bin)
  writeFileSync(join(bin, 'npm'), '#!/usr/bin/env sh\nexit 7\n', { mode: 0o755 })
  const r = run(dir, ['--build-only'], { PATH: bin + ':' + process.env.PATH })
  assert.equal(r.status, 7)
  assert.equal(existsSync(join(dir, '.local/test/build.lock')), false)
  assert.equal(existsSync(join(dir, '.local/test/data')), true)
})
