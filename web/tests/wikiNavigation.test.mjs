import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import { LatestRequest } from '../src/services/latestRequest.ts'

function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
async function settle() { for (let i = 0; i < 8; i++) await Promise.resolve(); await vue.nextTick() }
class ApiError extends Error { constructor(status) { super(`HTTP ${status}`); this.status = status } }

function evaluate(source, modules) {
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => {
    assert.ok(name in modules, `Unexpected module ${name}`)
    return modules[name]
  }, output, output.exports)
  return output.exports
}

function articleSetup(api, initial = () => null) {
  const modules = {
    vue, '../services/api': { ...api, ApiError },
    '../services/latestRequest': { LatestRequest },
    '../services/initialPage': { consumeInitialArticle: initial },
  }
  const { useArticle } = evaluate(readFileSync(new URL('../src/composables/useArticle.ts', import.meta.url), 'utf8'), modules)
  const scope = vue.effectScope(), locale = vue.ref('ko'), name = vue.ref('첫 문서')
  const view = scope.run(() => useArticle(locale, name))
  return { view, locale, name, stop: () => scope.stop() }
}

function historySetup(api) {
  const scope = vue.effectScope(), locale = vue.ref('ko'), route = vue.reactive({ params: { name: '첫_문서' } })
  const modules = {
    vue: { ...vue, onBeforeUnmount: vue.onScopeDispose },
    'vue-router': { useRoute: () => route },
    '../services/api': api,
    '../services/latestRequest': { LatestRequest },
    '../i18n': { useWikiI18n: () => ({ locale, t: (key) => key }) },
    '../services/auth': { hasPermission: () => false },
    '../composables/useMobileViewer': { useMobileViewer: () => vue.ref(false) },
    '../services/articleHTML': { articleHTMLDirective: {} },
  }
  const { descriptor } = parse(readFileSync(new URL('../src/views/HistoryView.vue', import.meta.url), 'utf8'))
  const component = evaluate(compileScript(descriptor, { id: 'history-navigation' }).content, modules).default
  const view = scope.run(() => component.setup({}, { expose() {} }))
  return { view, locale, route, stop: () => scope.stop() }
}

test('article navigation ignores old success and finally even before the next Vue tick', async () => {
  const old = deferred(), next = deferred(), signals = []
  const screen = articleSetup({ getArticle: (_, name, signal) => { signals.push(signal); return name === '첫 문서' ? old.promise : next.promise } })
  try {
    screen.name.value = '새 문서'
    assert.equal(signals[0].aborted, true)
    old.resolve({ name: '첫 문서' }); await settle()
    assert.equal(screen.view.article.value, null)
    assert.equal(screen.view.loading.value, true)
    next.resolve({ name: '새 문서' }); await settle()
    assert.equal(screen.view.article.value.name, '새 문서')
    assert.equal(screen.view.loading.value, false)
  } finally { screen.stop() }
})

test('old article 404/409 never replaces the current edition; current errors and retry still work', async () => {
  for (const status of [404, 409]) {
    const old = deferred(), next = deferred()
    const screen = articleSetup({ getArticle: (_, name) => name === '첫 문서' ? old.promise : next.promise })
    try {
      screen.name.value = '새 문서'
      old.reject(new ApiError(status)); await settle()
      assert.equal(screen.view.missing.value, false)
      assert.equal(screen.view.inactiveEdition.value, false)
      assert.equal(screen.view.loading.value, true)
      next.reject(new ApiError(status)); await settle()
      assert.equal(screen.view.missing.value, status === 404)
      assert.equal(screen.view.inactiveEdition.value, status === 409)
    } finally { screen.stop() }
  }
  let fail = true
  const screen = articleSetup({ getArticle: async () => { if (fail) throw Error('offline'); return { name: '첫 문서' } } })
  try {
    await settle(); assert.equal(screen.view.error.value, 'offline')
    fail = false; await screen.view.reload()
    assert.equal(screen.view.error.value, null); assert.equal(screen.view.article.value.name, '첫 문서')
  } finally { screen.stop() }
})

test('article SSR bootstrap invalidates pending reads and disposal ignores late errors', async () => {
  const old = deferred(); let signal
  const screen = articleSetup({ getArticle: (_, __, value) => { signal = value; return old.promise } }, (_, name) => name === 'SSR' ? { name } : null)
  screen.name.value = 'SSR'
  assert.equal(signal.aborted, true)
  old.resolve({ name: '첫 문서' }); await settle()
  assert.equal(screen.view.article.value.name, 'SSR'); assert.equal(screen.view.loading.value, false)
  screen.stop()
  const late = deferred(), stopped = articleSetup({ getArticle: () => late.promise })
  stopped.stop(); late.reject(Error('late')); await settle()
  assert.equal(stopped.view.error.value, null)
})

test('history navigation ignores old cancellation/errors and clears stale document state', async () => {
  const old = deferred(), next = deferred(), signals = []
  const screen = historySetup({ getRevisions: (_, name, signal) => { signals.push(signal); return name === '첫 문서' ? old.promise : next.promise } })
  try {
    screen.view.resetConfirmation.value = '첫 문서'
    screen.route.params.name = '새_문서'
    assert.equal(signals[0].aborted, true)
    old.reject(new DOMException('cancelled', 'AbortError')); await settle()
    assert.equal(screen.view.error.value, ''); assert.equal(screen.view.loading.value, true)
    assert.equal(screen.view.resetConfirmation.value, '')
    next.resolve({ name: '새 문서', revisions: [] }); await settle()
    assert.equal(screen.view.history.value.name, '새 문서')
    assert.equal(screen.view.loading.value, false)
  } finally { screen.stop() }
})

test('revision preview latest selection wins and navigation/unmount cancels reads', async () => {
  const first = deferred(), second = deferred(), third = deferred(), fourth = deferred(), pending = [first, second, third, fourth], signals = []
  const screen = historySetup({
    getRevisions: async (_, name) => ({ name, revisions: [] }),
    getRevision: (_, __, ___, signal) => { signals.push(signal); return pending.shift().promise },
  })
  try {
    await settle()
    const a = screen.view.selectRevision('r1'), b = screen.view.selectRevision('r2')
    assert.equal(signals[0].aborted, true)
    second.resolve({ revision: 'r2' }); await b
    first.resolve({ revision: 'r1' }); await a
    assert.equal(screen.view.selected.value.revision, 'r2')
    const c = screen.view.selectRevision('r3')
    screen.route.params.name = '새_문서'
    assert.equal(signals[2].aborted, true)
    third.reject(Error('old document')); await c
    assert.equal(screen.view.selected.value, null); assert.equal(screen.view.error.value, '')
    const d = screen.view.selectRevision('r4'); screen.stop()
    assert.equal(signals[3].aborted, true)
    fourth.resolve({ revision: 'r4' }); await d
    assert.equal(screen.view.selected.value, null)
  } finally { screen.stop() }
})

test('revision compare rejects stale pairs and stale document errors', async () => {
  const first = deferred(), second = deferred(), pending = [first, second], signals = []
  const screen = historySetup({
    getRevisions: async (_, name) => ({ name, revisions: [] }),
    getRevisionDiff: (_, __, from, to, signal) => { signals.push(signal); assert.deepEqual([from, to], ['r1', 'r2']); return pending.shift().promise },
  })
  try {
    await settle(); screen.view.toggleCompare('r2'); screen.view.toggleCompare('r1')
    const a = screen.view.compareRevisions()
    screen.view.toggleCompare('r1')
    assert.equal(signals[0].aborted, true)
    first.resolve({ from: 'r1', to: 'r2' }); await a
    assert.equal(screen.view.diff.value, null); assert.equal(screen.view.comparing.value, false)
    screen.view.toggleCompare('r1'); const b = screen.view.compareRevisions()
    screen.route.params.name = '새_문서'
    second.reject(Error('old diff')); await b
    assert.equal(screen.view.diff.value, null); assert.equal(screen.view.error.value, '')
    assert.deepEqual(screen.view.compareSelection.value, [])
  } finally { screen.stop() }
})

test('history restore completion after navigation never reloads the new page', async () => {
  const pending = deferred(), loads = []
  const screen = historySetup({
    getRevisions: async (_, name) => { loads.push(name); return { name, revisions: [{ revision: 'r2' }] } },
    restoreRevision: () => pending.promise,
  })
  globalThis.window = { confirm: () => true }
  try {
    await settle(); screen.view.selected.value = { revision: 'r1' }
    const restoring = screen.view.restoreSelected()
    screen.route.params.name = '새_문서'; await settle()
    pending.resolve(); await restoring
    assert.deepEqual(loads, ['첫 문서', '새 문서'])
    assert.equal(screen.view.history.value.name, '새 문서')
  } finally { screen.stop(); delete globalThis.window }
})

test('history reset reports completion only on the same page, including its reload', async () => {
  const pending = deferred(), loads = []
  let resets = 0
  const screen = historySetup({
    getRevisions: async (_, name) => { loads.push(name); return { name, revisions: [{ revision: 'r2' }] } },
    resetDocumentHistory: () => { resets++; return resets === 1 ? Promise.resolve({ removed_revisions: 1, removed_changes: 2 }) : pending.promise },
  })
  globalThis.window = { confirm: () => true }
  try {
    await settle(); screen.view.resetConfirmation.value = '첫 문서'
    await screen.view.resetHistory()
    assert.match(screen.view.resetMessage.value, /리비전 1개와 최근 변경 2개/)
    assert.equal(screen.view.resetConfirmation.value, '')
    screen.view.resetConfirmation.value = '첫 문서'
    const resetting = screen.view.resetHistory()
    screen.route.params.name = '새_문서'; await settle()
    pending.resolve({ removed_revisions: 1, removed_changes: 2 }); await resetting
    assert.equal(screen.view.resetMessage.value, '')
    assert.deepEqual(loads, ['첫 문서', '첫 문서', '새 문서'])
  } finally { screen.stop(); delete globalThis.window }
})
