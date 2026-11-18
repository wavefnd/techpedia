import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { boxAvailability, boxSyntax } from '../src/services/boxes.ts'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import { LatestRequest } from '../src/services/latestRequest.ts'

test('box syntax escapes resource names and never translates them', () => {
  assert.equal(boxSyntax('A & B'), '<wiki-box name="A &amp; B" />')
  assert.equal(boxSyntax('"<img>'), '<wiki-box name="&quot;&lt;img&gt;" />')
})

test('box language selection uses exact edition before explicit fallback only', () => {
  assert.deepEqual(boxAvailability(['ko', 'en'], 'ko', 'en'), { locale: 'en', available: true, fallback: false })
  assert.deepEqual(boxAvailability(['ko', 'en'], 'ko', 'ar'), { locale: 'ko', available: true, fallback: true })
  assert.equal(boxAvailability(['en'], '', 'ko').available, false)
  assert.equal(boxAvailability(['en'], 'fr', 'ko').available, false)
})

test('box UI stays behind lazy routes and picker layout fixes chrome outside scrolling content', () => {
  const router = readFileSync(new URL('../src/app/router.ts', import.meta.url), 'utf8')
  assert.match(router, /adminView\(\(\) => import\('\.\.\/views\/admin\/AdminBoxesView.vue'\)\)/)
  const editor = readFileSync(new URL('../src/views/EditorView.vue', import.meta.url), 'utf8')
  assert.match(editor, /defineAsyncComponent\(\(\) => import\('\.\.\/components\/BoxPicker.vue'\)\)/)
  const picker = readFileSync(new URL('../src/components/BoxPicker.vue', import.meta.url), 'utf8')
  assert.match(picker, /<Teleport to="body">/)
  assert.match(picker, /role="dialog" aria-modal="true"/)
  assert.match(picker, /previews\.invalidate\(\)/)
  const css = readFileSync(new URL('../src/components/box-picker.css', import.meta.url), 'utf8')
  assert.match(css, /height: min\(740px, calc\(100dvh - 40px\)\)/)
  assert.match(css, /\.box-picker-body \{[^}]*min-height: 0;[^}]*overflow: hidden/s)
  assert.match(css, /\.box-resource-list \{[^}]*overflow-y: auto/s)
  assert.match(css, /\.box-picker > footer \{[^}]*flex: 0 0 auto/s)
})

test('box picker ignores stale previews, inserts exact resource reference and restores scrolling', async () => {
  let resolveOld
  const oldPreview = new Promise((resolve) => { resolveOld = resolve }), events = [], hooks = { mounted: [], unmounted: [] }
  const oldWindow = globalThis.window, oldDocument = globalThis.document
  let focused = false
  const body = { style: { overflow: 'auto' } }
  globalThis.document = { body, activeElement: { focus: () => { focused = true } } }
  globalThis.window = { clearTimeout() {}, setTimeout: () => 1, addEventListener() {}, removeEventListener() {} }
  const modules = {
    vue: { ...vue, onMounted: (fn) => hooks.mounted.push(fn), onBeforeUnmount: (fn) => hooks.unmounted.push(fn) },
    '../services/api': { request: async (path) => path.includes('First/preview') ? oldPreview : path.includes('/preview/') ? { html: '<p>Second preview</p>' } : { boxes: [], next: '' } },
    '../services/latestRequest': { LatestRequest }, '../services/articleHTML': { articleHTMLDirective: {} },
    '../services/boxes': { boxAvailability, boxSyntax }, './box-picker.css': {},
  }
  const { descriptor } = parse(readFileSync(new URL('../src/components/BoxPicker.vue', import.meta.url), 'utf8'))
  const code = ts.transpileModule(compileScript(descriptor, { id: 'box-picker' }).content, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const output = { exports: {} }, scope = vue.effectScope()
  new Function('require', 'module', 'exports', code)((name) => { assert.ok(name in modules, name); return modules[name] }, output, output.exports)
  const screen = scope.run(() => output.exports.default.setup(vue.reactive({ locale: 'ko' }), { expose() {}, emit: (...args) => events.push(args) }))
  try {
    for (const mount of hooks.mounted) await mount()
    assert.equal(body.style.overflow, 'hidden')
    const first = screen.choose({ name: 'First', locales: ['ko'], fallback_locale: '' })
    await screen.choose({ name: 'A & B', locales: ['ko'], fallback_locale: '' })
    resolveOld({ html: '<p>Old preview</p>' }); await first
    assert.equal(screen.preview.value, '<p>Second preview</p>')
    screen.insert(); assert.deepEqual(events, [['insert', '<wiki-box name="A &amp; B" />']])
    screen.preview.value = ''; screen.insert(); assert.equal(events.length, 1)
  } finally {
    for (const unmount of hooks.unmounted) unmount()
    scope.stop(); assert.equal(body.style.overflow, 'auto'); assert.equal(focused, true)
    globalThis.window = oldWindow; globalThis.document = oldDocument
  }
})
