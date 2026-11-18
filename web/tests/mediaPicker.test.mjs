import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

function setup(api = {}, owner = true) {
  const hooks = { mounted: [], unmounted: [] }
  globalThis.window = { clearTimeout() {}, setTimeout: () => 1, addEventListener() {}, removeEventListener() {}, prompt: () => null }
  const body = { style: { overflow: 'scroll' } }
  globalThis.document = { body }
  const { descriptor } = parse(readFileSync(new URL('../src/components/MediaPicker.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'MediaPicker' }).content
  const code = ts.transpileModule(script, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const modules = {
    vue: { ...vue, onMounted: (f) => hooks.mounted.push(f), onBeforeUnmount: (f) => hooks.unmounted.push(f) },
    '@lucide/vue': new Proxy({}, { get: () => ({}) }),
    '../services/auth': { hasPermission: (permission) => owner && permission === 'owner' },
    '../services/api': { searchMedia: async () => ({ files: [], usage_bytes: 0, limit_bytes: 1000 }), ...api },
  }
  const out = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => modules[name], out, out.exports)
  const view = out.exports.default.setup({}, { expose() {}, emit() {} })
  return { view, body, mount: async () => { for (const f of hooks.mounted) await f(); await vue.nextTick() }, unmount: () => { for (const f of hooks.unmounted) f(); delete globalThis.window; delete globalThis.document } }
}
const file = { name: 'Photo.webp', preview_url: '/photo', thumbnail_url: '/photo' }
const deferred = () => { let resolve; const promise = new Promise((r) => { resolve = r }); return { promise, resolve } }

test('media picker locks only background scrolling and restores it when closed', async () => {
  const screen = setup()
  try { await screen.mount(); assert.equal(document.body.style.overflow, 'hidden') }
  finally { screen.unmount() }
  assert.equal(screen.body.style.overflow, 'scroll')
})

test('media deletion requires owner, exact filename and a single in-flight write', async () => {
  let writes = 0
  const blocked = setup({ deleteMedia: async () => { writes++ } }, false)
  blocked.view.selected.value = file; window.prompt = () => { throw Error('non-owner got a prompt') }
  try { await blocked.view.removeUploaded(); assert.equal(writes, 0) } finally { blocked.unmount() }
  const pending = deferred(), screen = setup({ deleteMedia: async (name, confirmation) => { assert.equal(name, file.name); assert.equal(confirmation, file.name); writes++; return pending.promise } })
  try {
    screen.view.selected.value = file; window.prompt = () => 'wrong'
    await screen.view.removeUploaded(); assert.equal(writes, 0)
    window.prompt = () => file.name
    const work = screen.view.removeUploaded(); await screen.view.removeUploaded()
    assert.equal(writes, 1); assert.equal(screen.view.deleting.value, true)
    pending.resolve({ cleanup_pending: false }); await work
    assert.equal(screen.view.selected.value, null); assert.equal(screen.view.deleting.value, false)
    assert.match(screen.view.notice.value, /삭제했습니다/)
  } finally { screen.unmount() }
})

test('media deletion failure preserves selection, surfaces server reason and never retries automatically', async () => {
  let writes = 0
  const screen = setup({ deleteMedia: async () => { writes++; throw Error('현재 문서에서 사용하는 이미지입니다') } })
  try {
    screen.view.selected.value = file; window.prompt = () => file.name
    await screen.view.removeUploaded()
    assert.equal(writes, 1); assert.deepEqual(screen.view.selected.value, file)
    assert.match(screen.view.error.value, /사용하는 이미지/); assert.equal(screen.view.deleting.value, false)
  } finally { screen.unmount() }
})

test('a search started before deletion cannot put a deleted file back into the list', async () => {
  const old = deferred(); let reads = 0
  const screen = setup({ searchMedia: () => ++reads === 1 ? old.promise : Promise.resolve({ files: [], usage_bytes: 0, limit_bytes: 1000 }), deleteMedia: async () => ({ cleanup_pending: false }) })
  try {
    const search = screen.view.loadMedia(); screen.view.selected.value = file; window.prompt = () => file.name
    await screen.view.removeUploaded()
    old.resolve({ files: [file], usage_bytes: 42, limit_bytes: 1000 }); await search
    assert.deepEqual(screen.view.files.value, []); assert.equal(screen.view.selected.value, null)
  } finally { screen.unmount() }
})
