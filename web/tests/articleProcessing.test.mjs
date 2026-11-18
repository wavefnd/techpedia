import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { collectArticleToc } from '../src/services/toc.ts'
import { revealCurrentHash } from '../src/services/sections.ts'

test('direct fragment reopens and scrolls restored article nodes without following stale navigation', () => {
  const frame = [], section = { open: false }, scrolls = []
  const target = { isConnected: true, closest: () => section, scrollIntoView: (options) => scrolls.push(options) }
  globalThis.window = { location: { hash: '#section-160' }, document: { getElementById: () => target }, requestAnimationFrame: (work) => frame.push(work) }
  assert.equal(revealCurrentHash({ contains: () => true }), target)
  assert.equal(section.open, true)
  frame.shift()()
  assert.deepEqual(scrolls, [{ block: 'start', behavior: 'instant' }])
  frame.shift()()
  assert.equal(scrolls.length, 2)
  revealCurrentHash({ contains: () => true })
  window.location.hash = '#elsewhere'
  frame.shift()()
  assert.equal(scrolls.length, 2)
  window.location.hash = '#section-160'
  revealCurrentHash({ contains: () => true })
  target.isConnected = false
  frame.shift()()
  assert.equal(scrolls.length, 2)
  assert.equal(revealCurrentHash({ contains: () => false }), null)
})

function loadLinks(resolveWikiLinks, waitForIdle = async () => {}) {
  const code = ts.transpileModule(readFileSync(new URL('../src/services/wikiLinks.ts', import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const modules = { './api': { resolveWikiLinks }, './scheduling': { waitForIdle } }, output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => {
    assert.ok(name in modules, `Unexpected module: ${name}`)
    return modules[name]
  }, output, output.exports)
  return output.exports
}
function link(target, locale = 'ko') {
  const classes = new Set()
  return {
    pathname: `/${locale}/wiki/${encodeURIComponent(target)}`, isConnected: true, href: 'unchanged', title: '',
    classes, classList: { toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name) },
  }
}
function container(links) {
  return { isConnected: true, queries: 0, querySelectorAll() { this.queries++; return links } }
}
function resolved(targets) { return targets.map((target) => ({ target, exists: false, url: `/ko/edit/${target}`, name: target })) }
function deferred() { let resolve; const promise = new Promise((yes) => { resolve = yes }); return { promise, resolve } }

test('mobile TOC starts collapsed before rendering and never constructs a hidden tree', () => {
  globalThis.window = { matchMedia: () => ({ matches: true }) }
  const mounted = [], emitted = []
  const { descriptor } = parse(readFileSync(new URL('../src/components/ArticleToc.vue', import.meta.url), 'utf8'))
  const code = ts.transpileModule(compileScript(descriptor, { id: 'toc-test' }).content, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const modules = {
    vue: { ...vue, onMounted: (work) => mounted.push(work) },
    '@lucide/vue': {}, './TocTreeItem.vue': {},
    '../i18n': { useWikiI18n: () => ({ t: (key) => key, uiLocale: vue.ref('ko') }) },
    '../services/sections': { revealCollapsibleSection: () => null },
  }, output = { exports: {} }, scope = vue.effectScope()
  new Function('require', 'module', 'exports', code)((name) => modules[name], output, output.exports)
  try {
    const view = scope.run(() => output.exports.default.setup(vue.reactive({ sections: [] }), { expose() {}, emit: (...args) => emitted.push(args) }))
    assert.equal(view.collapsed.value, true)
    for (const work of mounted) work()
    assert.deepEqual(emitted, [['collapse', true]])
    view.toggle(); assert.equal(view.collapsed.value, false)
    assert.match(descriptor.template.content, /<ul v-if="!collapsed" class="toc-tree">/)
  } finally { scope.stop() }
})

test('TOC reads displayed headings once and assigns real fallback anchors', () => {
  const headings = [
    { tagName: 'H2', id: 'existing', textContent: '  첫 절  ' },
    { tagName: 'H3', id: '', textContent: '하위 절' },
    { tagName: 'H4', id: '', textContent: '' },
    { tagName: 'H2', id: 'next', textContent: '다음 절' },
    { tagName: 'H4', id: 'deep', textContent: '건너뛴 단계' },
  ]
  let calls = 0
  const nodes = collectArticleToc({ querySelectorAll(selector) { calls++; assert.equal(selector, 'h2, h3, h4'); return headings } }, '절')
  assert.equal(calls, 1); assert.equal(nodes.length, 2)
  assert.equal(nodes[0].id, 'existing'); assert.equal(nodes[0].label, '첫 절')
  assert.equal(nodes[0].children[0].id, 'section-2')
  assert.equal(nodes[0].children[0].children[0].label, '절 3')
  assert.equal(headings[2].id, 'section-3')
  assert.equal(nodes[1].children[0].id, 'deep')
  assert.deepEqual(collectArticleToc(null, 'Section'), [])
})

test('offscreen section containment keeps the markup and is disabled for printing', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /@supports \(content-visibility: auto\) \{\s*\.techmark-section-viewport:has\(> \.techmark-section\[open\]\) \{[^}]*content-visibility: auto;[^}]*contain-intrinsic-size: auto 680px;/)
  assert.match(css, /\.techmark-section-content \{[^}]*content-visibility: auto;[^}]*contain-intrinsic-size: auto 600px;/)
  assert.match(css, /@media print \{[\s\S]*\.techmark-section-viewport \{[^}]*content-visibility: visible !important;/)
  assert.match(css, /@media print \{[\s\S]*\.techmark-section > \.techmark-section-content \{[^}]*content-visibility: visible;/)
  const toc = readFileSync(new URL('../src/components/ArticleToc.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(toc, /DOMParser|parseFromString/)
})

test('link resolver retains 80-target network batches and decorates each result', async () => {
  globalThis.window = { location: { pathname: '/ko/wiki/Test' } }
  const links = Array.from({ length: 171 }, (_, i) => link(`대상 ${i}`)), calls = []
  const api = loadLinks(async (locale, targets, signal) => {
    calls.push(targets.length); assert.equal(locale, 'ko'); assert.equal(signal.aborted, false)
    return resolved(targets)
  })
  await api.markMissingWikiLinks(container(links), 'ko')
  assert.deepEqual(calls, [80, 80, 11])
  assert.ok(links.every((value) => value.classes.has('missing') && !value.classes.has('exists') && value.href.startsWith('/ko/edit/')))
  assert.equal(links[0].title, '대상 0 문서는 아직 없습니다.')
})

test('DOM work yields for repeated anchors, not only unique targets', async () => {
  globalThis.window = { location: { pathname: '/ko/wiki/Test' } }
  const links = Array.from({ length: 650 }, () => link('One Target')), calls = [], idle = []
  const api = loadLinks(async (_, targets) => { calls.push(targets); return resolved(targets) }, async (timeout) => { idle.push(timeout) })
  await api.markMissingWikiLinks(container(links), 'ko')
  assert.deepEqual(calls, [['One Target']])
  assert.ok(idle.filter((timeout) => timeout === 50).length >= 6)
  assert.ok(links.every((value) => value.classes.has('missing')))
})

test('navigation before idle prevents the whole DOM query', async () => {
  globalThis.window = { location: { pathname: '/ko/wiki/Old' } }
  const idle = deferred(), root = container([link('Target')])
  const api = loadLinks(() => { throw new Error('Should not fetch') }, () => idle.promise)
  const task = api.markMissingWikiLinks(root, 'ko')
  window.location.pathname = '/admin'
  idle.resolve(); await task
  assert.equal(root.queries, 0)
})

test('same-container replacement aborts HTTP and ignores an old late response', async () => {
  globalThis.window = { location: { pathname: '/ko/wiki/Test' } }
  const response = deferred(), started = deferred(), links = [link('Old')], root = container(links)
  let signal
  const api = loadLinks((_, __, value) => { signal = value; started.resolve(); return response.promise })
  const task = api.markMissingWikiLinks(root, 'ko')
  await started.promise
  api.cancelMissingWikiLinks(root)
  assert.equal(signal.aborted, true)
  response.resolve(resolved(['Old'])); await task
  assert.equal(links[0].href, 'unchanged'); assert.equal(links[0].classes.size, 0)
})

test('detached article and unavailable resolver do not decorate unknown links', async () => {
  globalThis.window = { location: { pathname: '/ko/wiki/Test' } }
  const links = [link('Target')], root = container(links)
  const api = loadLinks(async () => { throw new Error('offline') })
  root.isConnected = false
  await api.markMissingWikiLinks(root, 'ko')
  assert.equal(root.queries, 0)
  root.isConnected = true
  await assert.doesNotReject(api.markMissingWikiLinks(root, 'ko'))
  assert.equal(links[0].href, 'unchanged'); assert.equal(links[0].classes.size, 0)
})
