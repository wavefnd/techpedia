import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const source = resolve(import.meta.dirname, '../..')
const gitBinary = spawnSync('which', ['git'], { encoding: 'utf8' }).stdout.trim()
function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'techpedia-deploy-test-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  return dir
}
function run(cwd, command, args, env = {}) {
  return spawnSync(command, args, { cwd, encoding: 'utf8', env: { ...process.env, ...env } })
}
function git(cwd, ...args) {
  const r = run(cwd, gitBinary, args)
  assert.equal(r.status, 0, r.stderr)
  return r.stdout.trim()
}
function init(dir) {
  mkdirSync(dir, { recursive: true })
  git(dir, 'init', '--initial-branch=main')
  git(dir, 'config', 'user.name', 'Deployment test')
  git(dir, 'config', 'user.email', 'test@example.invalid')
}
function put(dir, name, body, executable = false) {
  writeFileSync(join(dir, name), body, { mode: executable ? 0o755 : 0o644 })
}
function commit(dir) { git(dir, 'add', '.'); git(dir, 'commit', '-m', 'fixture') }

function installation(t) {
  const base = fixture(t), upstream = join(base, 'upstream'), install = join(base, 'install')
  init(upstream)
  put(upstream, 'start.sh', '#!/usr/bin/env bash\nprintf "%s" "$1" > started\n', true)
  put(upstream, 'restart.sh', readFileSync(join(source, 'restart.sh')), true)
  put(upstream, '.gitignore', '.env\ndata/\nstarted\n')
  put(upstream, 'marker', 'initial source')
  commit(upstream)
  git(base, 'clone', '--branch', 'main', upstream, install)
  git(install, 'config', 'user.name', 'Deployment test')
  git(install, 'config', 'user.email', 'test@example.invalid')
  git(install, 'remote', 'set-url', 'origin', 'https://github.com/wavefnd/techpedia.git')
  put(install, '.env', 'TEST=private')
  mkdirSync(join(install, 'data')); put(join(install, 'data'), 'sentinel', 'keep')
  put(upstream, 'marker', 'updated source'); commit(upstream)
  const bin = join(base, 'bin'); mkdirSync(bin)
  put(bin, 'git', `#!/usr/bin/env bash\nif [[ "$1" == fetch ]]; then shift 2; exec '${gitBinary}' fetch '${upstream}' "$@"; fi\nexec '${gitBinary}' "$@"\n`, true)
  return { install, upstream, env: { PATH: bin + ':' + process.env.PATH } }
}

test('restart fast-forwards approved main without touching runtime or remote', t => {
  const { install, upstream, env } = installation(t)
  const r = run(install, 'bash', ['restart.sh'], env)
  assert.equal(r.status, 0, r.stderr)
  assert.equal(git(install, 'rev-parse', 'HEAD'), git(upstream, 'rev-parse', 'HEAD'))
  assert.equal(git(install, 'branch', '--show-current'), 'main')
  assert.equal(git(install, 'remote', 'get-url', 'origin'), 'https://github.com/wavefnd/techpedia.git')
  assert.equal(readFileSync(join(install, '.env'), 'utf8'), 'TEST=private')
  assert.equal(readFileSync(join(install, 'data/sentinel'), 'utf8'), 'keep')
  assert.equal(readFileSync(join(install, 'started'), 'utf8'), '--force-recreate')
})

for (const reason of ['dirty source', 'unapproved origin', 'feature branch', 'divergent main']) {
  test(`restart rejects ${reason} without starting or replacing local source`, t => {
    const { install, env } = installation(t)
    if (reason === 'dirty source') put(install, 'marker', 'local uncommitted source')
    if (reason === 'unapproved origin') git(install, 'remote', 'set-url', 'origin', 'https://example.invalid/unapproved.git')
    if (reason === 'feature branch') git(install, 'switch', '-c', 'codex/local-change')
    if (reason === 'divergent main') { put(install, 'marker', 'local committed source'); commit(install) }
    const before = git(install, 'rev-parse', 'HEAD')
    const marker = readFileSync(join(install, 'marker'), 'utf8')
    const origin = git(install, 'remote', 'get-url', 'origin')
    const r = run(install, 'bash', ['restart.sh'], env)
    assert.notEqual(r.status, 0)
    assert.equal(git(install, 'rev-parse', 'HEAD'), before)
    assert.equal(readFileSync(join(install, 'marker'), 'utf8'), marker)
    assert.equal(git(install, 'remote', 'get-url', 'origin'), origin)
    assert.equal(readFileSync(join(install, 'data/sentinel'), 'utf8'), 'keep')
    assert.equal(existsSync(join(install, 'started')), false)
  })
}

test('restart no-pull does not require git or change source', t => {
  const dir = fixture(t)
  put(dir, 'restart.sh', readFileSync(join(source, 'restart.sh')), true)
  put(dir, 'start.sh', '#!/usr/bin/env bash\nprintf "%s" "$1" > started\n', true)
  assert.equal(run(dir, 'bash', ['restart.sh', '--no-pull']).status, 0)
  assert.equal(readFileSync(join(dir, 'started'), 'utf8'), '--force-recreate')
  assert.equal(run(dir, 'bash', ['restart.sh', '--no-pull', 'extra']).status, 2)
})

test('start rejects sample passwords and malformed env without disclosing values', t => {
  const dir = fixture(t), bin = join(dir, 'bin'); mkdirSync(bin)
  put(bin, 'docker', '#!/usr/bin/env bash\nexit 0\n', true)
  put(dir, 'start.sh', readFileSync(join(source, 'start.sh')), true)
  put(dir, '.env', 'TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD=replace-with-secret\n')
  const env = { PATH: bin + ':' + process.env.PATH, TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD: 'replace-with-secret' }
  assert.equal(run(dir, 'bash', ['start.sh'], env).status, 1)
  put(dir, '.env', 'INVALID credential-private-value\n')
  const r = run(dir, 'bash', ['start.sh'], env)
  assert.equal(r.status, 1)
  assert.equal(r.stderr.includes('credential-private-value'), false)
})

test('private transfer retains Git but excludes env variants and local runtime', t => {
  const base = fixture(t), repo = join(base, 'repo'), archive = join(base, 'private.tar.gz')
  init(repo)
  put(repo, 'move.sh', readFileSync(join(source, 'move.sh')), true)
  put(repo, '.env.example', 'SAMPLE=change-me')
  commit(repo)
  put(repo, '.env', 'PRIVATE=not-for-transfer')
  put(repo, '.env.production', 'PRIVATE=not-for-transfer')
  for (const directory of ['docs/node_modules', '.local/test/data']) {
    mkdirSync(join(repo, directory), { recursive: true })
    put(join(repo, directory), 'sentinel', 'not-for-transfer')
  }
  const r = run(repo, 'bash', ['move.sh', '--allow-dirty', archive])
  assert.equal(r.status, 0, r.stderr)
  const entries = run(base, 'tar', ['-tzf', archive]).stdout.split('\n')
  assert.ok(entries.includes('repo/.git/HEAD'))
  assert.ok(entries.includes('repo/.env.example'))
  assert.ok(!entries.includes('repo/.env'))
  assert.ok(!entries.includes('repo/.env.production'))
  assert.ok(!entries.some(x => x.startsWith('repo/docs/node_modules')))
  assert.ok(!entries.some(x => x.startsWith('repo/.local')))
  rmSync(join(repo, '.env.production'))
  const second = run(repo, 'bash', ['move.sh', '--allow-dirty', join(base, 'second.tar.gz')])
  assert.equal(second.status, 0, second.stderr)
})
