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
function init(dir, branch) {
  mkdirSync(dir, { recursive: true })
  git(dir, 'init', '--initial-branch=' + branch)
  git(dir, 'config', 'user.name', 'Deployment test')
  git(dir, 'config', 'user.email', 'test@example.invalid')
}
function put(dir, name, body, executable = false) {
  writeFileSync(join(dir, name), body, { mode: executable ? 0o755 : 0o644 })
}
function commit(dir) { git(dir, 'add', '.'); git(dir, 'commit', '-m', 'fixture') }

test('restart changes known GitLab source to GitHub main, preserving runtime and legacy branch', t => {
  const base = fixture(t), upstream = join(base, 'upstream'), install = join(base, 'install')
  init(upstream, 'main')
  const starter = '#!/usr/bin/env bash\nprintf "%s" "$1" > started\n'
  put(upstream, 'start.sh', starter, true)
  put(upstream, '.gitignore', '.env\ndata/\nstarted\n')
  put(upstream, 'marker', 'public source')
  commit(upstream)
  init(install, 'new')
  put(install, 'start.sh', starter, true)
  put(install, 'restart.sh', readFileSync(join(source, 'restart.sh')), true)
  put(install, '.gitignore', '.env\ndata/\nstarted\n')
  put(install, 'marker', 'private legacy source')
  commit(install)
  git(install, 'remote', 'add', 'origin', 'https://gitlab.com/wave-inc/wave-web/techpedia.git')
  put(install, '.env', 'TEST=private')
  mkdirSync(join(install, 'data')); put(join(install, 'data'), 'sentinel', 'keep')
  const bin = join(base, 'bin'); mkdirSync(bin)
  put(bin, 'git', `#!/usr/bin/env bash\nif [[ "$1" == fetch ]]; then shift 2; exec '${gitBinary}' fetch '${upstream}' "$@"; fi\nexec '${gitBinary}' "$@"\n`, true)
  const r = run(install, 'bash', ['restart.sh'], { PATH: bin + ':' + process.env.PATH })
  assert.equal(r.status, 0, r.stderr)
  assert.equal(git(install, 'branch', '--show-current'), 'main')
  assert.equal(git(install, 'remote', 'get-url', 'origin'), 'https://github.com/wavefnd/techpedia.git')
  assert.equal(readFileSync(join(install, 'marker'), 'utf8'), 'public source')
  assert.equal(readFileSync(join(install, '.env'), 'utf8'), 'TEST=private')
  assert.equal(readFileSync(join(install, 'data/sentinel'), 'utf8'), 'keep')
  assert.equal(git(install, 'show', 'new:marker'), 'private legacy source')
  assert.equal(readFileSync(join(install, 'started'), 'utf8'), '--force-recreate')
})

test('restart no-pull does not require git or change source', t => {
  const dir = fixture(t)
  put(dir, 'restart.sh', readFileSync(join(source, 'restart.sh')), true)
  put(dir, 'start.sh', '#!/usr/bin/env bash\nprintf "%s" "$1" > started\n', true)
  assert.equal(run(dir, 'bash', ['restart.sh', '--no-pull']).status, 0)
  assert.equal(readFileSync(join(dir, 'started'), 'utf8'), '--force-recreate')
  assert.equal(run(dir, 'bash', ['restart.sh', '--no-pull', 'extra']).status, 2)
})

test('public export has one main root commit and no private history/runtime', t => {
  const base = fixture(t), repo = join(base, 'repo'), out = join(base, 'public')
  init(repo, 'new'); mkdirSync(join(repo, 'scripts'))
  put(join(repo, 'scripts'), 'export-public.sh', readFileSync(join(source, 'scripts/export-public.sh')), true)
  put(repo, '.gitignore', '.env\ndata/\n')
  put(repo, 'README.md', 'Old private title'); commit(repo)
  put(repo, 'README.md', 'Reviewed public snapshot'); commit(repo)
  put(repo, '.env', 'do-not-copy'); mkdirSync(join(repo, 'data')); put(join(repo, 'data'), 'sentinel', 'private')
  const r = run(repo, 'bash', ['scripts/export-public.sh', out], {
    GIT_AUTHOR_NAME: 'Export test', GIT_AUTHOR_EMAIL: 'test@example.invalid',
    GIT_COMMITTER_NAME: 'Export test', GIT_COMMITTER_EMAIL: 'test@example.invalid',
  })
  assert.equal(r.status, 0, r.stderr)
  assert.equal(git(out, 'branch', '--show-current'), 'main')
  assert.equal(git(out, 'rev-list', '--count', 'HEAD'), '1')
  assert.equal(git(out, 'remote'), '')
  assert.equal(existsSync(join(out, '.env')), false)
  assert.equal(existsSync(join(out, 'data')), false)
  assert.equal(git(repo, 'rev-list', '--count', 'HEAD'), '2')
  assert.equal(run(repo, 'bash', ['scripts/export-public.sh', out]).status, 2)
  put(repo, 'untracked', 'dirty')
  assert.equal(run(repo, 'bash', ['scripts/export-public.sh', join(base, 'blocked')]).status, 1)
  assert.equal(existsSync(join(base, 'blocked')), false)
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

test('private transfer retains Git but excludes env variants and caches', t => {
  const base = fixture(t), repo = join(base, 'repo'), archive = join(base, 'private.tar.gz')
  init(repo, 'new')
  put(repo, 'move.sh', readFileSync(join(source, 'move.sh')), true)
  put(repo, '.env.example', 'SAMPLE=change-me')
  commit(repo)
  put(repo, '.env', 'PRIVATE=not-for-transfer')
  put(repo, '.env.production', 'PRIVATE=not-for-transfer')
  mkdirSync(join(repo, 'docs/node_modules'), { recursive: true })
  put(join(repo, 'docs/node_modules'), 'sentinel', 'cache')
  const r = run(repo, 'bash', ['move.sh', '--allow-dirty', archive])
  assert.equal(r.status, 0, r.stderr)
  const entries = run(base, 'tar', ['-tzf', archive]).stdout.split('\n')
  assert.ok(entries.includes('repo/.git/HEAD'))
  assert.ok(entries.includes('repo/.env.example'))
  assert.ok(!entries.includes('repo/.env'))
  assert.ok(!entries.includes('repo/.env.production'))
  assert.ok(!entries.some(x => x.startsWith('repo/docs/node_modules')))
  // An empty optional exclusion list must also work on macOS Bash 3.
  rmSync(join(repo, '.env.production'))
  const second = run(repo, 'bash', ['move.sh', '--allow-dirty', join(base, 'second.tar.gz')])
  assert.equal(second.status, 0, second.stderr)
})

test('public export refuses tracked private paths before creating destination', t => {
  const base = fixture(t), repo = join(base, 'repo'), out = join(base, 'blocked')
  init(repo, 'new'); mkdirSync(join(repo, 'scripts'))
  put(join(repo, 'scripts'), 'export-public.sh', readFileSync(join(source, 'scripts/export-public.sh')), true)
  put(repo, '.env.production', 'TEST=placeholder')
  commit(repo)
  const r = run(repo, 'bash', ['scripts/export-public.sh', out])
  assert.equal(r.status, 1)
  assert.match(r.stderr, /Forbidden public-export path/)
  assert.equal(existsSync(out), false)
})
