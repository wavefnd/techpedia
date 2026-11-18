import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
import { createRouter, createMemoryHistory } from 'vue-router'
import { ref } from 'vue'
import { parallelView } from '../src/services/routeLoading.ts'

function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }

test('view and UI start together but route activation waits for both', async () => {
  const view = deferred(), ui = deferred(), started = []
  const load = parallelView(() => { started.push('view'); return view.promise }, () => { started.push('ui'); return ui.promise })
  let finished = false
  const result = load().then((value) => { finished = true; return value })
  assert.deepEqual(started, ['view', 'ui'])
  view.resolve('screen'); await Promise.resolve(); assert.equal(finished, false)
  ui.resolve(); assert.equal(await result, 'screen')
})

test('parallel UI failure never activates the view and a later attempt can retry', async () => {
  let calls = 0
  const load = parallelView(async () => 'screen', async () => { if (++calls === 1) throw Error('CSS unavailable') })
  await assert.rejects(load(), /CSS unavailable/)
  assert.equal(await load(), 'screen')
})

function actualRouter(account, calls, ui = Promise.resolve()) {
  const code = ts.transpileModule(readFileSync(new URL('../src/app/router.ts', import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const component = { __esModule: true, default: { render() {} } }
  const modules = {
    'vue-router': { createRouter, createWebHistory: createMemoryHistory },
    '../composables/useMobileViewer': { isMobileViewerViewport: () => false },
    '../services/adminUi': { ensureAdminUI: () => { calls.push('ui'); return ui } },
    '../services/routeLoading': { parallelView },
    '../services/navigationFailure': { navigationFailed: ref(false), reportNavigationFailure() {} },
    '../services/auth': { loadSession: async () => { calls.push('auth'); return account }, hasPermission: () => Boolean(account?.permissions.includes('admin')) },
    '../components/admin/AdminShell.vue': component,
    '../views/LoginView.vue': component,
    '../views/LanguagePortalView.vue': component,
    '../views/admin/AdminDashboardView.vue': component,
  }, output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => {
    assert.ok(name in modules, `Unexpected module ${name}`)
    if (name.includes('AdminDashboard')) calls.push('view')
    return modules[name]
  }, output, output.exports)
  return output.exports.router
}

test('actual admin route checks permission before starting parallel UI/view imports', async () => {
  const calls = [], ui = deferred(), router = actualRouter({ permissions: ['admin'] }, calls, ui.promise)
  const navigation = router.push('/admin')
  for (let i = 0; i < 40 && !calls.includes('view'); i++) await Promise.resolve()
  assert.equal(calls[0], 'auth')
  assert.ok(calls.includes('ui') && calls.includes('view'))
  assert.notEqual(router.currentRoute.value.path, '/admin')
  ui.resolve(); await navigation
  assert.equal(router.currentRoute.value.path, '/admin')
})

test('actual admin route denial never loads admin UI or view', async () => {
  const calls = [], router = actualRouter(null, calls)
  await router.push('/admin')
  assert.equal(router.currentRoute.value.name, 'login')
  assert.deepEqual(calls, ['auth'])
})

test('initial wiki shell is loaded alongside the route; discussion retains article DOM', () => {
  const main = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8')
  assert.match(main, /Promise\.all\(\[router\.isReady\(\)\.catch\(reportNavigationFailure\), initialShell\]\)/)
  const article = readFileSync(new URL('../src/views/ArticleView.vue', import.meta.url), 'utf8')
  assert.match(article, /<div v-show="!discussionMode" class="article-grid">/)
})
