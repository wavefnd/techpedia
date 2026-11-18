import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import { LatestRequest } from '../src/services/latestRequest.ts'
import { boxSyntax } from '../src/services/boxes.ts'

// Run the real SFC setup and Vue watchers without a browser or new test runtime.
// Rendering, keyboard interaction and browser navigation remain separate QA.
function setup(file, overrides = {}) {
  const hooks = { mounted: [], unmounted: [], errors: [] }, timers = new Map()
  let timerID = 0
  globalThis.window = {
    setTimeout: (work) => { const id = ++timerID; timers.set(id, work); return id },
    clearTimeout: (id) => timers.delete(id),
    confirm: () => false,
    prompt: () => '테스트 사유',
    addEventListener() {}, removeEventListener() {},
  }
  globalThis.document = { documentElement: { lang: '', dir: '', dataset: {} } }
  const editions = vue.ref([{ tag: 'ko', status: 'active' }, { tag: 'en', status: 'active' }])
  class MockApiError extends Error { constructor(status, message) { super(message); this.status = status } }
  const api = new Proxy(overrides, { get: (target, key) => target[key] ?? (key === 'ApiError' ? MockApiError : (() => { throw new Error(`Unexpected API call: ${String(key)}`) })) })
  const modules = {
    vue: { ...vue, onMounted: (work) => hooks.mounted.push(work), onBeforeUnmount: (work) => hooks.unmounted.push(work), onErrorCaptured: (work) => hooks.errors.push(work) },
    'vue-router': { useRoute: () => overrides.route ?? ({ query: {} }), useRouter: () => overrides.router ?? ({ push: async () => {} }), onBeforeRouteLeave() {} },
    '@lucide/vue': new Proxy({}, { get: () => ({}) }),
    '../../services/api': api,
    '../../services/auth': { account: vue.ref(overrides.account ?? null) },
    '../../services/support': { syncPublicSupportSettings: overrides.syncPublicSupportSettings ?? (() => {}) },
    '../../services/latestRequest': { LatestRequest },
    '../../services/editions': { languageEditions: editions, activeLanguageEditions: editions, loadLanguageEditions: overrides.loadLanguageEditions ?? (async () => {}) },
    '../../services/boxes': { boxSyntax },
    '../../services/articleHTML': { articleHTMLDirective: {} },
    '../../i18n': { documentURL: () => '/' },
  }
  const path = new URL(file === 'AdminShell' ? '../src/components/admin/AdminShell.vue' : `../src/views/admin/${file}.vue`, import.meta.url)
  const { descriptor } = parse(readFileSync(path, 'utf8'))
  const script = compileScript(descriptor, { id: file }).content
  const code = ts.transpileModule(script, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => {
    if (!(name in modules)) throw new Error(`Unexpected module ${name}`)
    return modules[name]
  }, output, output.exports)
  const view = output.exports.default.setup({}, { expose() {} })
  return { view, captureError: () => hooks.errors.map((hook) => hook(new Error('render failed'))), mount: async () => { for (const hook of hooks.mounted) await hook() }, unmount: () => { for (const hook of hooks.unmounted) hook(); delete globalThis.window; delete globalThis.document }, timers }
}
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }

const boxEntry = (name = 'Common', version = 1, locale = 'ko', source = 'Saved source') => ({ name, version, locales: [locale], fallback_locale: '', updated_at: '', content: { locale, source, version, author: 'admin', updated_at: '' } })
test('AdminBoxes: later selection wins and dirty input needs explicit discard', async () => {
  const pending = deferred()
  const screen = setup('AdminBoxesView', { request: async (path) => path.includes('/First/') ? pending.promise : boxEntry('Second') })
  try {
    const first = screen.view.openBox({ name: 'First', locales: ['ko'] }, 'ko')
    await screen.view.openBox({ name: 'Second', locales: ['ko'] }, 'ko')
    pending.resolve(boxEntry('First')); await first
    assert.equal(screen.view.name.value, 'Second'); assert.equal(screen.view.dirty.value, false)
    screen.view.source.value = 'Unsaved'; assert.equal(screen.view.dirty.value, true)
    await screen.view.openBox({ name: 'First', locales: ['ko'] }, 'ko')
    assert.equal(screen.view.source.value, 'Unsaved'); assert.equal(screen.view.name.value, 'Second')
  } finally { screen.unmount() }
})

test('AdminBoxes: duplicate saves blocked and failed save keeps exact source and baseline', async () => {
  const pending = deferred(); let writes = 0
  const screen = setup('AdminBoxesView', { request: async (_path, init) => {
    assert.equal(init.method, 'PUT'); writes++; assert.equal(JSON.parse(init.body).base_version, 3); return pending.promise
  } })
  try {
    screen.view.applyEntry(boxEntry('Common', 3)); screen.view.source.value = 'My exact new source'
    const save = screen.view.save(); await screen.view.save(); assert.equal(writes, 1)
    pending.reject(new Error('conflict')); await save
    assert.equal(screen.view.source.value, 'My exact new source'); assert.equal(screen.view.baseVersion.value, 3)
    assert.equal(screen.view.dirty.value, true); assert.equal(screen.view.busy.value, false)
  } finally { screen.unmount() }
})

test('AdminBoxes: draft copy never saves and cannot replace a newer baseline', async () => {
  const calls = []
  const screen = setup('AdminBoxesView', { request: async (path, init) => {
    calls.push([path, init]); return { source: 'English draft', base_version: 4, unresolved: ['Missing'] }
  } })
  try {
    screen.view.applyEntry(boxEntry('Common', 3, 'en')); screen.view.copyFrom.value = 'ko'
    await screen.view.copyLanguage(); assert.equal(screen.view.source.value, 'Saved source')
    screen.view.applyEntry(boxEntry('Common', 4, 'en')); screen.view.copyFrom.value = 'ko'; await screen.view.copyLanguage()
    assert.equal(screen.view.source.value, 'English draft'); assert.equal(screen.view.dirty.value, true)
    assert.deepEqual(screen.view.unresolved.value, ['Missing']); assert.equal(calls.length, 2)
    assert.ok(calls.every(([path, init]) => path.endsWith('/draft/en?from=ko') && !init))
  } finally { screen.unmount() }
})

test('AdminUsers: legacy null permissions do not crash role lists', async () => {
  const screen = setup('AdminUsersView', { getUserManagement: async () => ({ users: [{ id: 'reader', permissions: null }], regular_permissions: [] }) })
  try {
    await screen.mount()
    assert.equal(screen.view.regularUsers.value.length, 1)
    assert.deepEqual(screen.view.regularUsers.value[0].permissions, [])
    assert.equal(screen.view.ownerUsers.value.length, 0)
    assert.equal(screen.view.administratorUsers.value.length, 0)
  } finally { screen.unmount() }
})

test('AdminConnections: preparing consent never approves or redirects automatically', async () => {
  const calls = [], replaced = [], assigned = []
  const screen = setup('AdminConnectionsView', {
    route: { query: { client_id: 'test-client', scope: 'wiki:read' } },
    router: { replace: async (path) => replaced.push(path) },
    request: async (path, init) => {
      calls.push(path)
      if (path === '/api/admin/oauth') return { enabled: true, clients: [], grants: [] }
      if (path === '/api/admin/oauth/requests') return { request: { id: 'request-one' }, client: { name: 'Test' } }
      assert.deepEqual(JSON.parse(init.body), { allow: false })
      return { redirect_url: 'https://chatgpt.example/callback?error=access_denied' }
    },
  })
  globalThis.window.location = { assign: (value) => assigned.push(value) }
  try {
    await screen.mount()
    assert.deepEqual(calls, ['/api/admin/oauth', '/api/admin/oauth/requests'])
    assert.equal(screen.view.approval.value.request.id, 'request-one')
    assert.deepEqual(replaced, ['/admin/connections']); assert.deepEqual(assigned, [])
    await screen.view.decide(false)
    assert.deepEqual(assigned, ['https://chatgpt.example/callback?error=access_denied'])
  } finally { screen.unmount() }
})

test('AdminConnections: registration is single-submit and clears fields after success', async () => {
  const pending = deferred(); let registrations = 0
  const screen = setup('AdminConnectionsView', { request: async (path) => {
    if (path === '/api/admin/oauth') return { enabled: true, clients: [], grants: [] }
    assert.equal(path, '/api/admin/oauth/clients'); registrations++; return pending.promise
  } })
  try {
    await screen.mount()
    screen.view.name.value = 'Test'; screen.view.redirect.value = 'https://chatgpt.example/callback'
    const first = screen.view.register(); await screen.view.register()
    assert.equal(registrations, 1); assert.equal(screen.view.busy.value, true)
    pending.resolve({ id: 'client-one' }); await first
    assert.equal(screen.view.busy.value, false); assert.equal(screen.view.name.value, '')
    assert.equal(screen.view.redirect.value, ''); assert.match(screen.view.notice.value, /등록했습니다/)
  } finally { screen.unmount() }
})

test('AdminConnections: duplicate authorization parameters are rejected before preparation', async () => {
  const calls = []
  const screen = setup('AdminConnectionsView', {
    route: { query: { client_id: ['one', 'two'] } },
    request: async (path) => { calls.push(path); return { enabled: true, clients: [], grants: [] } },
  })
  try {
    await screen.mount()
    assert.deepEqual(calls, ['/api/admin/oauth']); assert.equal(screen.view.approval.value, null)
    assert.match(screen.view.error.value, /중복 연결 요청/)
  } finally { screen.unmount() }
})

test('AdminPages: locale search protects data, loading and error from stale responses', async () => {
  const old = deferred(), next = deferred()
  const screen = setup('AdminPagesView', { getAdminPages: (locale) => locale === 'en' ? old.promise : next.promise })
  try {
    screen.view.locale.value = 'en'; const a = screen.view.load()
    screen.view.locale.value = 'ko'; const b = screen.view.load()
    old.reject(new Error('old error')); await a
    assert.equal(screen.view.loading.value, true); assert.equal(screen.view.error.value, '')
    next.resolve([{ name: '새 문서' }]); await b
    assert.equal(screen.view.pages.value[0].name, '새 문서'); assert.equal(screen.view.loading.value, false)
  } finally { screen.unmount() }
})
test('AdminPages: closing and reopening Techdata dialog changes request ownership', async () => {
  const old = deferred(), next = deferred(); let calls = 0
  const screen = setup('AdminPagesView', { getTechdataItems: () => ++calls === 1 ? old.promise : next.promise })
  try {
    const a = screen.view.openTechdata({ name: 'A', locale: 'en' })
    screen.view.closeTechdata()
    const b = screen.view.openTechdata({ name: 'B', locale: 'ko' })
    next.resolve([{ id: 'T2' }]); await b
    old.resolve([{ id: 'T1' }]); await a
    assert.equal(screen.view.techdataPage.value.name, 'B'); assert.equal(screen.view.techdataItems.value[0].id, 'T2')
    assert.equal(screen.view.techdataLoading.value, false)
  } finally { screen.unmount() }
})
test('AdminPages: link report is cleared and in-flight report rejected on locale change', async () => {
  const pending = deferred(), screen = setup('AdminPagesView', { getAdminLinkReport: () => pending.promise })
  try {
    screen.view.linkReport.value = { documents: 1 }
    const work = screen.view.loadLinkReport()
    screen.view.locale.value = 'en'
    assert.equal(screen.view.linkReport.value, null)
    pending.resolve({ documents: 99 }); await work
    assert.equal(screen.view.linkReport.value, null); assert.equal(screen.view.linkLoading.value, false)
  } finally { screen.unmount() }
})
test('AdminPages: initial language failure is visible and prevents creation until retry', async () => {
  let failed = true
  const screen = setup('AdminPagesView', { loadLanguageEditions: async () => { if (failed) throw new Error('언어판 서버 오류') }, getAdminPages: async () => [], getAdminRedirects: async () => [] })
  try {
    await screen.mount(); assert.match(screen.view.error.value, /언어판을 불러오지 못했습니다/); assert.equal(screen.view.newLocale.value, '')
    assert.equal(screen.view.editionsReady.value, false)
    failed = false; await screen.view.bootstrap()
    assert.equal(screen.view.newLocale.value, 'ko'); assert.equal(screen.view.error.value, '')
    assert.equal(screen.view.editionsReady.value, true)
  } finally { screen.unmount() }
})
test('AdminAnalytics: selected period never shows the previous period data', async () => {
  const old = deferred(), next = deferred(), screen = setup('AdminAnalyticsView', { getAdminAnalytics: (days) => days === 30 ? old.promise : next.promise })
  try {
    const a = screen.view.load()
    screen.view.days.value = 90; const b = screen.view.load()
    assert.equal(screen.view.analytics.value, null)
    next.resolve({ days: 90, daily: [], countries: [] }); await b
    old.resolve({ days: 30, daily: [], countries: [] }); await a
    assert.equal(screen.view.days.value, 90); assert.equal(screen.view.analytics.value.days, 90)
  } finally { screen.unmount() }
})

test('AdminPages: repeated redirect, Techdata and delete submissions are guarded', async () => {
  const redirect = deferred(), link = deferred(), deletion = deferred()
  let redirectCalls = 0, linkCalls = 0, deleteCalls = 0
  const screen = setup('AdminPagesView', {
    createAdminRedirect: () => { redirectCalls++; return redirect.promise },
    account: { permissions: ['owner'] },
    linkPageTechdata: () => { linkCalls++; return link.promise },
    deleteAdminPage: () => { deleteCalls++; return deletion.promise },
    getAdminRedirects: async () => [], getAdminPages: async () => [],
  })
  try {
    screen.view.redirectLocale.value = 'en'; screen.view.redirectName.value = 'BSD'; screen.view.redirectTarget.value = 'Berkeley Software Distribution'
    const a = screen.view.createRedirect(); await screen.view.createRedirect()
    assert.equal(redirectCalls, 1); redirect.resolve(); await a
    screen.view.techdataPage.value = { name: 'A', locale: 'en' }
    const b = screen.view.connectTechdata('T1'); await screen.view.connectTechdata('T1')
    assert.equal(linkCalls, 1); link.resolve(); await b
    const c = screen.view.deletePage({ name: 'A', locale: 'en' }); await screen.view.deletePage({ name: 'A', locale: 'en' })
    assert.equal(deleteCalls, 1); deletion.resolve(); await c
    assert.equal(screen.view.deleteBusy.value, false)
  } finally { screen.unmount() }
})

test('AdminPages: duplicate Techdata warning survives an earlier in-flight search', async () => {
  const search = deferred(), link = deferred()
  const screen = setup('AdminPagesView', {
    getTechdataItems: () => search.promise,
    linkPageTechdata: () => link.promise,
    techdataDuplicateResponse: () => ({ candidates: [{ id: 'T9' }] }),
  })
  try {
    const pendingSearch = screen.view.openTechdata({ name: 'A', locale: 'en' })
    const pendingLink = screen.view.connectTechdata()
    link.reject(new Error('duplicate')); await pendingLink
    assert.equal(screen.view.techdataDuplicateBlocked.value, true)
    search.resolve([{ id: 'T1' }]); await pendingSearch
    assert.equal(screen.view.techdataItems.value[0].id, 'T9'); assert.equal(screen.view.techdataDuplicateBlocked.value, true)
  } finally { screen.unmount() }
})

test('AdminConnections: disabled status never registers or prepares authorization', async () => {
  const calls = []
  const screen = setup('AdminConnectionsView', { route: { query: { client_id: 'test' } }, request: async (path) => { calls.push(path); return { enabled: false, clients: [], grants: [] } } })
  try {
    await screen.mount()
    screen.view.name.value = 'Test'; screen.view.redirect.value = 'https://example.com/callback'
    await screen.view.register()
    assert.deepEqual(calls, ['/api/admin/oauth'])
    assert.equal(screen.view.data.value.enabled, false)
    assert.equal(screen.view.approval.value, null)
  } finally { screen.unmount() }
})

test('AdminUsers: failed initial load blocks writes and retry recovers', async () => {
  let failed = true, writes = 0
  const screen = setup('AdminUsersView', {
    getUserManagement: async () => { if (failed) throw new Error('server unavailable'); return { users: [], regular_permissions: [] } },
    setRegularUserPermissions: async () => { writes++; return { permissions: ['edit'], updated_users: 0 } },
  })
  try {
    await screen.mount(); await screen.view.togglePolicy('edit', true)
    assert.equal(writes, 0); assert.equal(screen.view.policyLocked.value, true)
    assert.match(screen.view.error.value, /server unavailable/)
    failed = false; await screen.view.load(); await screen.view.togglePolicy('edit', true)
    assert.equal(writes, 1); assert.equal(screen.view.ready.value, true)
  } finally { screen.unmount() }
})

test('AdminUsers: role operations lock each target and global policy until settled', async () => {
  const a = deferred(), b = deferred(); let roles = 0, policies = 0
  const screen = setup('AdminUsersView', {
    getUserManagement: async () => ({ users: [], regular_permissions: [] }),
    setUserPermissions: (id) => { roles++; return id === 'a' ? a.promise : b.promise },
    setRegularUserPermissions: async () => { policies++ },
  })
  globalThis.window.confirm = () => true
  try {
    await screen.mount()
    const first = screen.view.promote({ id: 'a', display_name: 'A' })
    await screen.view.promote({ id: 'a', display_name: 'A' })
    const second = screen.view.promote({ id: 'b', display_name: 'B' })
    await screen.view.togglePolicy('edit', true)
    assert.equal(roles, 2); assert.equal(policies, 0); assert.equal(screen.view.saving.value.size, 2)
    a.resolve({ id: 'a', permissions: ['admin'] }); await first
    assert.equal(screen.view.policyLocked.value, true)
    b.resolve({ id: 'b', permissions: ['admin'] }); await second
    assert.equal(screen.view.policyLocked.value, false)
  } finally { screen.unmount() }
})

test('AdminUsers: policy is single-submit, blocks roles, and rolls back on failure', async () => {
  const pending = deferred(); let policies = 0, roles = 0
  const screen = setup('AdminUsersView', {
    getUserManagement: async () => ({ users: [], regular_permissions: [] }),
    setRegularUserPermissions: () => { policies++; return pending.promise },
    setUserPermissions: async () => { roles++ },
  })
  globalThis.window.confirm = () => true
  try {
    await screen.mount()
    const first = screen.view.togglePolicy('edit', true)
    await screen.view.togglePolicy('techdata', true); await screen.view.promote({ id: 'a' })
    assert.equal(policies, 1); assert.equal(roles, 0)
    pending.reject(new Error('save failed')); await first
    assert.deepEqual(screen.view.regularPermissions.value, []); assert.equal(screen.view.savingPolicy.value, false)
  } finally { screen.unmount() }
})

test('AdminUsers: stale and unmounted loads cannot replace current data', async () => {
  const old = deferred(), current = deferred(), closed = deferred(); let calls = 0
  const screen = setup('AdminUsersView', { getUserManagement: () => [old, current, closed][calls++].promise })
  const first = screen.view.load(), second = screen.view.load()
  current.resolve({ users: [{ id: 'new', permissions: [] }], regular_permissions: [] }); await second
  old.reject(new Error('old')); await first
  assert.equal(screen.view.users.value[0].id, 'new'); assert.equal(screen.view.error.value, '')
  const third = screen.view.load(); screen.unmount()
  closed.resolve({ users: [], regular_permissions: [] }); await third
  assert.equal(screen.view.users.value[0].id, 'new')
})

test('AdminSupport: failed read cannot overwrite saved settings and duplicate save is blocked', async () => {
  let failed = true, saves = 0; const pending = deferred()
  const settings = { enabled: false, title: 'Existing', description: '', wallets: [] }
  const screen = setup('AdminSupportView', {
    getAdminSupportSettings: async () => { if (failed) throw new Error('read failed'); return settings },
    saveAdminSupportSettings: () => { saves++; return pending.promise },
  })
  try {
    await screen.mount(); await screen.view.save(); assert.equal(saves, 0); assert.equal(screen.view.ready.value, false)
    failed = false; await screen.view.load()
    const first = screen.view.save(); await screen.view.save(); assert.equal(saves, 1)
    pending.resolve({ ...settings, wallets: [] }); await first
    assert.equal(screen.view.settings.value.wallets.length, 1); assert.equal(screen.view.saving.value, false)
  } finally { screen.unmount() }
})

test('AdminTrash: duplicate restoration is blocked per document, not globally', async () => {
  const a = deferred(), b = deferred(); let calls = 0
  const screen = setup('AdminTrashView', { getAdminTrash: async () => [], restoreAdminTrash: (id) => { calls++; return id === 'W1' ? a.promise : b.promise } })
  globalThis.window.confirm = () => true
  try {
    const first = screen.view.restore({ wid: 'W1', name: 'A' }); await screen.view.restore({ wid: 'W1', name: 'A' })
    const second = screen.view.restore({ wid: 'W2', name: 'B' })
    assert.equal(calls, 2); assert.equal(screen.view.busy.value.size, 2)
    a.resolve(); await first; assert.equal(screen.view.busy.value.has('W2'), true)
    b.resolve(); await second; assert.equal(screen.view.busy.value.size, 0)
  } finally { screen.unmount() }
})

test('AdminDiscussions: latest submitted search wins and conflicting topic writes are blocked', async () => {
  const old = deferred(), next = deferred(), write = deferred(); let calls = 0, deletes = 0
  const screen = setup('AdminDiscussionsView', {
    getAdminDiscussionTopics: () => ++calls === 1 ? old.promise : calls === 2 ? next.promise : Promise.resolve({ topics: [], count: 0 }),
    archiveDiscussionTopic: () => write.promise, deleteAdminDiscussionTopic: async () => { deletes++ },
  })
  globalThis.window.confirm = () => true
  try {
    screen.view.query.value = 'old'; const a = screen.view.loadTopics()
    screen.view.query.value = 'new'; const b = screen.view.loadTopics()
    next.resolve({ topics: [{ id: 'new' }], count: 1 }); await b
    old.reject(new Error('old')); await a
    assert.equal(screen.view.topics.value[0].id, 'new'); assert.equal(screen.view.error.value, '')
    const archive = screen.view.archiveTopic({ id: 'new', subject: 'New' }); await screen.view.removeTopic({ id: 'new', subject: 'New' })
    assert.equal(deletes, 0); write.resolve(); await archive; assert.equal(screen.view.busy.value.size, 0)
  } finally { screen.unmount() }
})

test('AdminTechdataAudit: edition failure is visible without hiding audit, and duplicate linking is guarded', async () => {
  const pending = deferred(); let links = 0
  const screen = setup('AdminTechdataAuditView', {
    loadLanguageEditions: async () => { throw new Error('language failed') },
    getAdminTechdataAudit: async () => ({ unlinked_documents: [], metadata_candidates: [], name_candidates: [] }),
    linkPageTechdata: () => { links++; return pending.promise },
  })
  try {
    await screen.mount(); assert.match(screen.view.editionError.value, /언어판/); assert.ok(screen.view.audit.value)
    const candidate = { page: { wid: 'W1', name: 'A', locale: 'ko' }, tid: 'T1' }
    const a = screen.view.connectCandidate(candidate); await screen.view.connectCandidate({ ...candidate, tid: 'T2' })
    assert.equal(links, 1); pending.resolve(); await a; assert.equal(screen.view.busyCandidate.value, '')
  } finally { screen.unmount() }
})

test('AdminBots: edition failure preserves suggestions and prevents scan until retry', async () => {
  let failed = true, scans = 0
  const screen = setup('AdminBotsView', {
    loadLanguageEditions: async () => { if (failed) throw new Error('failed') },
    getNamingSuggestions: async () => [{ id: 'one', status: 'open' }],
    scanNamingIndex: async () => { scans++; return { documents_scanned: 1, created: 0 } },
  })
  try {
    await screen.mount(); await screen.view.scan()
    assert.equal(screen.view.suggestions.value.length, 1); assert.equal(scans, 0); assert.match(screen.view.editionError.value, /언어판/)
    failed = false; await screen.view.loadEditions(); await screen.view.scan(); assert.equal(scans, 1)
  } finally { screen.unmount() }
})

test('AdminBots: stale status reads and accept/reject conflicts are guarded', async () => {
  const old = deferred(), next = deferred(), write = deferred(); let calls = 0, rejects = 0
  const screen = setup('AdminBotsView', {
    getNamingSuggestions: () => ++calls === 1 ? old.promise : calls === 2 ? next.promise : Promise.resolve([]),
    acceptNamingSuggestion: () => write.promise, rejectNamingSuggestion: async () => { rejects++ },
  })
  globalThis.window.confirm = () => true
  try {
    const a = screen.view.load(); screen.view.status.value = 'accepted'; await vue.nextTick()
    next.resolve([{ id: 'new' }]); await vue.nextTick(); await vue.nextTick()
    old.resolve([{ id: 'old' }]); await a
    assert.equal(screen.view.suggestions.value[0].id, 'new')
    const accept = screen.view.accept({ id: 'new' }); await screen.view.reject({ id: 'new' })
    assert.equal(rejects, 0); write.resolve(); await accept
  } finally { screen.unmount() }
})

test('AdminShell: child rendering failures are isolated and navigation resets boundary', async () => {
  const route = vue.reactive({ path: '/admin/users', fullPath: '/admin/users' })
  const screen = setup('AdminShell', { route })
  try {
    assert.deepEqual(screen.captureError(), [false])
    assert.equal(screen.view.viewFailed.value, true)
    assert.equal(screen.view.currentTitle.value, '사용자')
    screen.view.open.value = true
    route.path = '/admin/connections'; route.fullPath = '/admin/connections'; await vue.nextTick()
    assert.equal(screen.view.viewFailed.value, false); assert.equal(screen.view.open.value, false)
    assert.equal(screen.view.currentTitle.value, 'ChatGPT 연결')
    assert.equal(screen.view.navigation.flatMap((group) => group.items).some((item) => item.to === '/admin/connections'), true)
  } finally { screen.unmount() }
})

const proposal = (id) => ({ id, kind: 'edit', page: { locale: 'ko', name: '문서', revision: 'r1' }, status: 'pending', expires_at: new Date(Date.now() + 60000).toISOString() })
const proposalDetail = (id) => ({ proposal: proposal(id), before: 'old', after: 'new', lines: [], review_digest: 'a'.repeat(64), diff_offset: 0, diff_limit: 500, diff_total: 1200, changes_only: true, added_lines: 1, removed_lines: 1 })

test('AdminProposals: a superseded review cannot approve and links to a separately reviewed replacement', async () => {
  const routes = [], writes = []
  const old = { ...proposal('old'), status: 'superseded', superseded_by: 'new' }
  const next = { ...proposal('new'), supersedes: 'old' }
  const screen = setup('AdminProposalsView', {
    router: { replace: async (route) => routes.push(route) },
    request: async (path, init) => { if (init?.method) writes.push(init); return [old, next] },
  })
  window.confirm = () => true
  try {
    screen.view.detail.value = proposalDetail('old'); screen.view.checked.value = true
    screen.view.proposals.value = [proposal('old'), next]; screen.view.toggleSelected(proposal('old'))
    screen.view.batchChecked.value = true
    await screen.view.load()
    assert.equal(screen.view.detail.value.proposal.status, 'superseded')
    assert.equal(screen.view.checked.value, false); assert.equal(screen.view.batchChecked.value, false)
    assert.deepEqual(screen.view.selectedIDs.value, [])
    assert.equal(screen.view.statusName('superseded'), '수정본으로 대체됨')
    screen.view.checked.value = true; await screen.view.decide(true)
    assert.equal(writes.length, 0)
    await screen.view.select('new')
    assert.deepEqual(routes[0], { path: '/admin/proposals', query: { id: 'new' } })
    assert.equal(screen.view.detail.value.after, 'new')
  } finally { screen.unmount() }
})

test('AdminProposals: review never auto-approves and requires checkbox with exact digest', async () => {
  const writes = [], pending = deferred()
  const screen = setup('AdminProposalsView', {
    route: { query: { id: 'one' } },
    request: async (path, init) => {
      if (path.endsWith('/decide')) { writes.push(JSON.parse(init.body)); return pending.promise }
      if (path.endsWith('/one')) return proposalDetail('one')
      return [proposal('one')]
    },
  })
  globalThis.window.confirm = () => true
  try {
    await screen.mount(); assert.equal(writes.length, 0)
    await screen.view.decide(true); assert.equal(writes.length, 0)
    screen.view.checked.value = true
    const approval = screen.view.decide(true); await screen.view.decide(false); await screen.view.decide(true)
    assert.equal(writes.length, 1)
    assert.deepEqual(writes[0], { allow: true, confirmation: 'one', review_digest: 'a'.repeat(64) })
    pending.resolve({ ...proposal('one'), status: 'applied' }); await approval
    assert.equal(screen.view.detail.value.proposal.status, 'applied'); assert.equal(screen.view.checked.value, false)
    await screen.view.decide(true); assert.equal(writes.length, 1)
  } finally { screen.unmount() }
})
test('AdminProposals: stale detail cannot replace another review or its confirmation', async () => {
  const first = deferred(), second = deferred()
  const screen = setup('AdminProposalsView', { request: (path) => path.endsWith('/one') ? first.promise : second.promise })
  try {
    const a = screen.view.review('one'); screen.view.checked.value = true; const b = screen.view.review('two')
    assert.equal(screen.view.checked.value, false); assert.equal(screen.view.detail.value, null)
    second.resolve(proposalDetail('two')); await b
    first.resolve(proposalDetail('one')); await a
    assert.equal(screen.view.detail.value.proposal.id, 'two'); assert.equal(screen.view.checked.value, false)
  } finally { screen.unmount() }
})
test('AdminProposals: conflict is visible, resets confirmation and never retries approval', async () => {
  let writes = 0
  const screen = setup('AdminProposalsView', { request: async () => { writes++; throw new Error('문서가 변경되었습니다') } })
  globalThis.window.confirm = () => true
  try {
    screen.view.detail.value = proposalDetail('one'); screen.view.checked.value = true
    await screen.view.decide(true); assert.equal(writes, 1); assert.equal(screen.view.checked.value, false)
    assert.match(screen.view.detailError.value, /문서가 변경/)
    await screen.view.decide(true); assert.equal(writes, 1)
  } finally { screen.unmount() }
})
test('AdminPages: ordinary admins cannot invoke deletion even programmatically', async () => {
  let deletes = 0
  const screen = setup('AdminPagesView', { account: { permissions: ['admin'] }, deleteAdminPage: async () => { deletes++ } })
  try { await screen.view.deletePage({ locale: 'ko', name: '문서' }); assert.equal(deletes, 0) } finally { screen.unmount() }
})

test('AdminProposals: paged diff never changes the approval snapshot or source', async () => {
  const calls = []
  const screen = setup('AdminProposalsView', { request: async (path) => {
    calls.push(path)
    if (path.includes('/diff?')) return { ...proposalDetail('one'), diff_offset: 500, lines: [{ kind: 'added', text: 'last change', new_line: 5001 }] }
    return proposalDetail('one')
  } })
  try {
    await screen.view.review('one'); screen.view.checked.value = true
    await screen.view.loadDiff(500)
    assert.match(calls[1], /\/diff\?offset=500&limit=500&changes_only=true$/)
    assert.equal(screen.view.diff.value.diff_offset, 500)
    assert.equal(screen.view.detail.value.before, 'old'); assert.equal(screen.view.detail.value.after, 'new')
    assert.equal(screen.view.detail.value.review_digest, 'a'.repeat(64)); assert.equal(screen.view.checked.value, true)
  } finally { screen.unmount() }
})

test('AdminProposals: late diff page cannot replace another proposal', async () => {
  const first = deferred()
  const screen = setup('AdminProposalsView', { request: async (path) => path.includes('/diff?') ? first.promise : proposalDetail(path.endsWith('/two') ? 'two' : 'one') })
  try {
    await screen.view.review('one')
    const page = screen.view.loadDiff(500)
    await screen.view.review('two')
    first.resolve({ ...proposalDetail('one'), diff_offset: 500 }); await page
    assert.equal(screen.view.detail.value.proposal.id, 'two'); assert.equal(screen.view.diff.value.diff_offset, 0)
    assert.equal(screen.view.diffLoading.value, false)
  } finally { screen.unmount() }
})

test('AdminProposals: failed diff disables approval until a successful manual retry', async () => {
  let fails = true, writes = 0
  const screen = setup('AdminProposalsView', { request: async (path) => {
    if (path.endsWith('/decide')) { writes++; return { ...proposal('one'), status: 'applied' } }
    if (path.includes('/diff?') && fails) throw new Error('비교 로드 실패')
    if (path.endsWith('/proposals')) return [proposal('one')]
    return proposalDetail('one')
  } })
  globalThis.window.confirm = () => true
  try {
    await screen.view.review('one'); screen.view.checked.value = true
    await screen.view.loadDiff(500)
    assert.equal(screen.view.checked.value, false); assert.match(screen.view.diffError.value, /비교 로드 실패/)
    screen.view.checked.value = true; await screen.view.decide(true); assert.equal(writes, 0)
    fails = false; await screen.view.loadDiff(0, false)
    screen.view.checked.value = true; await screen.view.decide(true); assert.equal(writes, 1)
  } finally { screen.unmount() }
})

test('AdminProposals: review CSS never depends on the public light palette', () => {
  const css = readFileSync(new URL('../src/styles/admin.css', import.meta.url), 'utf8')
  const review = css.slice(css.indexOf('.admin-proposals-view'))
  assert.doesNotMatch(review, /var\(--(?:paper|surface|line|ink|accent|muted)\)/)
  assert.match(review, /\.proposal-diff-scroll \.added \{ background: #142b23; color: #b8eed0;/)
  assert.match(review, /\.proposal-diff-scroll \.removed \{ background: #321e29; color: #f5b8c6;/)
  assert.match(review, /\.proposal-queue-row\.selected \{[^}]*background: #28213d;/)
  const component = readFileSync(new URL('../src/views/admin/AdminProposalsView.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(component, /class="revision-diff"|v-html/)
  assert.match(component, /readonly spellcheck="false"/)
})

test('AdminProposals: select many first, review together, confirm once, and approve once without per-item steps', async () => {
  const write = deferred(), writes = []
  const screen = setup('AdminProposalsView', { request: async (path, init) => {
    if (path.endsWith('/decide-batch')) { writes.push(JSON.parse(init.body)); return write.promise }
    if (path.endsWith('/proposals')) return [{ ...proposal('one'), status: 'applied' }, proposal('two')]
    return { ...proposalDetail(path.includes('/two?') ? 'two' : 'one'), review_digest: (path.includes('/two?') ? 'b' : 'a').repeat(64) }
  } })
  try {
    screen.view.proposals.value = [proposal('one'), proposal('two')]
    screen.view.togglePage()
    assert.deepEqual(screen.view.selectedIDs.value, ['one', 'two'])
    assert.equal(screen.view.detail.value, null)
    await screen.view.reviewSelected()
    assert.equal(screen.view.batchReviews.value.length, 2); assert.equal(screen.view.batchReady.value, true)
    assert.equal(screen.view.batchChecked.value, false)
    await screen.view.decideBatch(true); assert.equal(writes.length, 0)
    screen.view.batchChecked.value = true
    const work = screen.view.decideBatch(true)
    await screen.view.decideBatch(false); await screen.view.decide(true)
    assert.equal(writes.length, 1)
    assert.deepEqual(writes[0], { allow: true, confirmation: 'approve:2', proposals: [
      { id: 'one', review_digest: 'a'.repeat(64) }, { id: 'two', review_digest: 'b'.repeat(64) },
    ] })
    write.resolve({ results: [{ id: 'one', proposal: { ...proposal('one'), status: 'applied' } }, { id: 'two', error: '문서가 변경되었습니다' }] })
    await work
    assert.match(screen.view.notice.value, /1건 승인·적용, 1건 처리 실패/)
    assert.equal(screen.view.selectedIDs.value.length, 0); assert.equal(screen.view.batchChecked.value, false)
    assert.equal(screen.view.batchResults.value[1].error, '문서가 변경되었습니다')
    await screen.view.decideBatch(true); assert.equal(writes.length, 1)
  } finally { screen.unmount() }
})

test('AdminProposals: ambiguous batch response preserves warning after reload and never auto-retries', async () => {
  let writes = 0
  const screen = setup('AdminProposalsView', { request: async (path) => {
    if (path.endsWith('/decide-batch')) { writes++; throw new Error('connection lost') }
    if (path.includes('?preview=true')) return proposalDetail('one')
    return [proposal('one')]
  } })
  globalThis.window.confirm = () => true
  try {
    screen.view.proposals.value = [proposal('one')]; screen.view.togglePage()
    await screen.view.reviewSelected(); screen.view.batchChecked.value = true
    await screen.view.decideBatch(true)
    assert.match(screen.view.batchError.value, /일부 항목이 처리되었을 수 있습니다/)
    assert.equal(screen.view.selectedIDs.value.length, 0); assert.equal(screen.view.batchChecked.value, false)
    await screen.view.decideBatch(true); assert.equal(writes, 1)
  } finally { screen.unmount() }
})

test('AdminProposals: retention update requires confirmation and invalidates old reviewed snapshots', async () => {
  let writes = 0
  const screen = setup('AdminProposalsView', { request: async (path, init) => {
    if (path.endsWith('/proposal-policy')) {
      if (init?.method === 'PUT') { writes++; assert.deepEqual(JSON.parse(init.body), { retention_days: 1 }); return { retention_days: 1 } }
      return { retention_days: 2 }
    }
    return [proposal('one')]
  } })
  try {
    await screen.mount()
    assert.equal(screen.view.retentionDays.value, 2)
    screen.view.detail.value = proposalDetail('one'); screen.view.toggleSelected(proposal('one'))
    screen.view.retentionDays.value = 1
    await screen.view.savePolicy(); assert.equal(writes, 0)
    globalThis.window.confirm = () => true
    await screen.view.savePolicy(); assert.equal(writes, 1)
    assert.equal(screen.view.savedRetentionDays.value, 1); assert.equal(screen.view.detail.value, null)
    assert.equal(screen.view.selectedIDs.value.length, 0); assert.equal(screen.view.busy.value, false)
  } finally { screen.unmount() }
})

test('AdminProposals: large queues render only fifty summaries per page', async () => {
  const screen = setup('AdminProposalsView', { request: async () => Array.from({ length: 150 }, (_, i) => proposal(String(i))) })
  try {
    await screen.view.load()
    assert.equal(screen.view.pendingCount.value, 150); assert.equal(screen.view.queuePages.value, 3)
    assert.equal(screen.view.visibleProposals.value.length, 50)
    screen.view.queuePage.value = 2; assert.equal(screen.view.visibleProposals.value[0].id, '100')
    screen.view.onlyPending.value = false; await vue.nextTick(); assert.equal(screen.view.queuePage.value, 0)
  } finally { screen.unmount() }
})

test('AdminProposals: changing selection aborts stale batch reads and resets the common confirmation', async () => {
  const first = deferred(); let signal
  const screen = setup('AdminProposalsView', { request: async (path, init) => {
    if (path.includes('/one?')) { signal = init.signal; return first.promise }
    return proposalDetail('two')
  } })
  try {
    screen.view.proposals.value = [proposal('one'), proposal('two')]
    screen.view.toggleSelected(proposal('one')); const old = screen.view.reviewSelected()
    screen.view.batchChecked.value = true
    screen.view.toggleSelected(proposal('two'))
    assert.equal(signal.aborted, true); assert.equal(screen.view.batchChecked.value, false)
    assert.equal(screen.view.batchReady.value, false); assert.equal(screen.view.batchOpen.value, false)
    first.resolve(proposalDetail('one')); await old
    assert.equal(screen.view.batchReviews.value.length, 0)
  } finally { screen.unmount() }
})

test('AdminProposals: an unread or failed preview cannot be hidden behind a bulk approval', async () => {
  let fails = true, writes = 0
  const screen = setup('AdminProposalsView', { request: async (path) => {
    if (path.endsWith('/decide-batch')) { writes++; return { results: [] } }
    if (path.includes('/two?') && fails) throw new Error('검토 읽기 실패')
    return proposalDetail(path.includes('/two?') ? 'two' : 'one')
  } })
  try {
    screen.view.proposals.value = [proposal('one'), proposal('two')]; screen.view.togglePage()
    await screen.view.decideBatch(true); assert.equal(writes, 0)
    await screen.view.reviewSelected()
    assert.match(screen.view.batchReviews.value[1].error, /검토 읽기 실패/)
    screen.view.batchChecked.value = true; await screen.view.decideBatch(true); assert.equal(writes, 0)
    fails = false; await screen.view.reviewSelected()
    assert.equal(screen.view.batchReady.value, true); assert.equal(screen.view.batchChecked.value, false)
  } finally { screen.unmount() }
})

test('AdminProposals: page selection skips decided entries, persists across pages and needs no individual review', async () => {
  const screen = setup('AdminProposalsView')
  try {
    screen.view.proposals.value = Array.from({ length: 101 }, (_, i) => i === 0 ? { ...proposal('0'), status: 'applied' } : proposal(String(i)))
    screen.view.onlyPending.value = false
    screen.view.togglePage(); assert.equal(screen.view.selectedIDs.value.length, 49)
    assert.equal(screen.view.selectedIDs.value.includes('0'), false)
    screen.view.queuePage.value = 1; screen.view.togglePage(); assert.equal(screen.view.selectedIDs.value.length, 99)
    screen.view.queuePage.value = 0; screen.view.togglePage(); assert.equal(screen.view.selectedIDs.value.length, 50)
    assert.equal(screen.view.batchReviews.value.length, 0)
    const component = readFileSync(new URL('../src/views/admin/AdminProposalsView.vue', import.meta.url), 'utf8')
    assert.doesNotMatch(component, /addReviewed|검토 완료 · 일괄 선택|proposal-basket/)
    assert.match(component, /v-model="batchChecked"/)
  } finally { screen.unmount() }
})

test('AdminProposals: combined diff paging keeps immutable digests and invalidates one shared confirmation on error', async () => {
  let fails = true
  const screen = setup('AdminProposalsView', { request: async (path) => {
    if (path.includes('/diff?')) {
      if (fails) throw new Error('비교 읽기 실패')
      return { ...proposalDetail('one'), diff_offset: 50, diff_limit: 50 }
    }
    return proposalDetail('one')
  } })
  try {
    screen.view.proposals.value = [proposal('one')]; screen.view.togglePage(); await screen.view.reviewSelected()
    const entry = screen.view.batchReviews.value[0]
    screen.view.batchChecked.value = true
    await screen.view.loadBatchDiff(entry, 50)
    assert.equal(screen.view.batchChecked.value, false); assert.equal(screen.view.batchReady.value, false)
    assert.match(entry.error, /비교 읽기 실패/)
    fails = false; await screen.view.loadBatchDiff(entry, 50)
    assert.equal(screen.view.batchReady.value, true); assert.equal(screen.view.batchChecked.value, false)
    assert.equal(entry.review_digest, 'a'.repeat(64)); assert.equal(entry.diff.diff_offset, 50)
  } finally { screen.unmount() }
})
