import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import { MultiDirectedGraph } from 'graphology'
import { graphInitialPosition, graphInitialPositions, graphLabel, graphNeighborhoods, graphNodeRadius, graphNodeTextColor, graphPalette } from '../src/services/graphPresentation.ts'
import { GraphLayoutSession } from '../src/services/graphLayout.ts'
import { LatestRequest } from '../src/services/latestRequest.ts'

const node = (id, name = id) => ({ id, name, locale: 'ko', url: `/ko/wiki/${name}`, degree: 3 })
const fixture = { scope: 'language', nodes: [node('a', '운영체제'), node('b', 'Linux'), node('c', '고립 문서')], edges: [{ source: 'a', target: 'b', kind: 'link' }], stats: { visible: 3, links: 1 } }
const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
async function settle() { for (let i = 0; i < 12; i++) await Promise.resolve(); await vue.nextTick() }

function setup(api = async () => fixture, mobile = false, hardware = {}) {
  const mounted = [], unmounted = [], renderers = [], layouts = [], theme = vue.ref('light'), routes = []
  const fontListeners = new Map()
  const documentListeners = new Map()
  const route = vue.reactive({ name: 'graph-language', params: { locale: 'ko' } })
  const previousNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { hardwareConcurrency: 8, ...hardware } })
  globalThis.window = { matchMedia: (query) => ({ matches: mobile && query.includes('max-width') }), setTimeout: () => 1, clearTimeout() {} }
  globalThis.document = { title: '', hidden: false, addEventListener: (key, listener) => documentListeners.set(key, listener), removeEventListener: (key) => documentListeners.delete(key), fonts: { addEventListener: (key, listener) => fontListeners.set(key, listener), removeEventListener: (key) => fontListeners.delete(key) } }
  globalThis.getComputedStyle = () => ({ fontFamily: 'Test Font', getPropertyValue: (key) => key === '--paper' ? (theme.value === 'dark' ? '#202020' : '#ffffff') : '#2366a8' })
  class Sigma {
    constructor(graph, _, settings) { this.graph = graph; this.settings = settings; this.events = {}; this.camera = { ratio: 1, getBoundedRatio: (v) => Math.max(.03, Math.min(8, v)), animate: (v) => { Object.assign(this.camera, v); return Promise.resolve() }, animatedReset: () => { this.camera.ratio = 1 } }; renderers.push(this) }
    getGraph() { return this.graph }
    getCanvases() { return this.canvases ||= { hovers: { style: {} } } }
    setSettings(settings) { Object.assign(this.settings, settings) }
    refresh() {}
    on(event, callback) { this.events[event] = callback }
    getNodeDisplayData() { return { x: .5, y: .5 } }
    getCamera() { return this.camera }
    kill() { this.killed = true }
  }
  class Worker {
    constructor() { layouts.push(this) }
    postMessage(command) {
      structuredClone(command)
      if (command.type === 'init') this.input = command.input
      else if (command.type !== 'ack') { this.running = command.type === 'start'; this.epoch = command.epoch }
    }
    frame(positions, stable = false) { this.onmessage({ data: { positions, stable, epoch: this.epoch } }) }
    terminate() { this.killed = true; this.running = false }
  }
  globalThis.Worker = Worker
  const modules = {
    vue: { ...vue, onMounted: (f) => mounted.push(f), onBeforeUnmount: (f) => unmounted.push(f) },
    'vue-router': { useRoute: () => route, useRouter: () => ({ push: (url) => routes.push(url) }) },
    '@lucide/vue': new Proxy({}, { get: () => ({}) }),
    graphology: { MultiDirectedGraph }, sigma: { default: Sigma },
    '../services/api': { getGraph: api },
    '../services/editions': { activeLanguageEditions: vue.ref([]), loadLanguageEditions: async () => [] },
    '../i18n': { canonicalLanguageTag: (v) => v, useWikiI18n: () => ({ locale: vue.ref('ko'), t: (key) => key }) },
    '../composables/useTheme': { useTheme: () => ({ theme }) },
    '../services/graphPresentation': { graphInitialPosition, graphLabel, graphNeighborhoods, graphNodeRadius, graphPalette },
    '../services/graphLayout': { GraphLayoutSession },
    '../services/latestRequest': { LatestRequest }, '../styles/graph.css': {},
  }
  const { descriptor } = parse(readFileSync(new URL('../src/views/GraphView.vue', import.meta.url), 'utf8'))
  const code = ts.transpileModule(compileScript(descriptor, { id: 'graph-test' }).content, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => { assert.ok(name in modules, name); return modules[name] }, output, output.exports)
  const scope = vue.effectScope()
  const view = scope.run(() => output.exports.default.setup({}, { expose() {} }))
  view.container.value = { style: {} }
  return { view, route, theme, renderers, layouts, routes, fontListeners, documentListeners, mount: async () => { mounted.forEach((f) => f()); await settle() }, stop: () => { unmounted.forEach((f) => f()); scope.stop(); delete globalThis.window; delete globalThis.document; delete globalThis.getComputedStyle; delete globalThis.Worker; if (previousNavigator) Object.defineProperty(globalThis, 'navigator', previousNavigator); else delete globalThis.navigator } }
}

for (const { name, hardware, mobile, interval } of [
  { name: 'eight cores', hardware: { hardwareConcurrency: 8 }, interval: 33 },
  { name: 'four cores', hardware: { hardwareConcurrency: 4 }, interval: 66 },
  { name: 'two cores', hardware: { hardwareConcurrency: 2 }, interval: 100 },
  { name: 'low memory', hardware: { hardwareConcurrency: 8, deviceMemory: 2 }, interval: 100 },
  { name: 'medium memory', hardware: { hardwareConcurrency: 8, deviceMemory: 4 }, interval: 66 },
  { name: 'mobile', hardware: { hardwareConcurrency: 8 }, mobile: true, interval: 100 },
  { name: 'missing core count', hardware: { hardwareConcurrency: undefined }, interval: 100 },
]) {
  test(`graph frame budget uses a deterministic ${name} profile`, async () => {
    const previousNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
    const screen = setup(undefined, mobile, hardware)
    try {
      await screen.mount()
      assert.equal(screen.layouts[0].input.interval, interval)
      assert.equal(screen.renderers[0].graph.order, fixture.nodes.length)
    } finally { screen.stop() }
    assert.deepEqual(Object.getOwnPropertyDescriptor(globalThis, 'navigator'), previousNavigator)
  })
}

test('graph neighborhoods deduplicate directions, translations and self/missing edges', () => {
  const neighbors = graphNeighborhoods({ ...fixture, edges: [...fixture.edges, { source: 'b', target: 'a', kind: 'translation' }, { source: 'a', target: 'a' }, { source: 'a', target: 'missing' }] })
  assert.deepEqual([...neighbors.get('a')], ['b'])
  assert.deepEqual([...neighbors.get('b')], ['a'])
  assert.equal(neighbors.get('c').size, 0)
})

test('mobile imports all ten thousand nodes, searches the last one, and only lowers frame frequency', async () => {
  const nodes = Array.from({ length: 10001 }, (_, index) => node(String(index), `Document ${index}`))
  const value = { ...fixture, nodes, edges: [], stats: { documents: nodes.length, visible: nodes.length, links: 0, truncated: false } }
  const calls = []
  const screen = setup(async (...args) => { calls.push(args); return value }, true)
  try {
    await screen.mount()
    screen.view.query.value = 'Document 10000'
    // Search typed during the cooperative import must refresh when its index arrives.
    assert.equal(screen.view.matches.value.length, 0)
    for (let attempt = 0; attempt < 200 && !screen.layouts.length; attempt++) await new Promise((resolve) => setTimeout(resolve, 1))
    assert.equal(calls[0].length, 2, 'hardware must not select an API document limit')
    assert.equal(screen.renderers[0].graph.order, nodes.length)
    assert.equal(screen.layouts[0].input.nodes.length, nodes.length * 3)
    assert.ok(screen.layouts[0].input.interval >= 100)
    assert.equal(vue.isReactive(screen.view.payload.value.nodes), false)
    assert.equal(screen.view.matches.value[0].id, '10000')
    screen.view.focusNode(screen.view.matches.value[0])
    assert.equal(screen.view.selected.value.id, '10000')
  } finally { screen.stop() }
})

test('navigation cancels an in-progress large graph import without attaching a stale renderer', async () => {
  const nodes = Array.from({ length: 10001 }, (_, index) => node(String(index)))
  const screen = setup(async (locale) => locale === 'ko' ? { ...fixture, nodes, edges: [] } : fixture)
  try {
    await screen.mount()
    assert.equal(screen.renderers.length, 0)
    screen.route.params.locale = 'en'
    for (let attempt = 0; attempt < 100 && !screen.layouts.length; attempt++) await new Promise((resolve) => setTimeout(resolve, 1))
    assert.equal(screen.renderers.length, 1)
    assert.equal(screen.renderers[0].graph.order, fixture.nodes.length)
    assert.equal(screen.layouts.length, 1)
  } finally { screen.stop() }
})

test('graph labels use the edition font, an opaque theme halo and Unicode source text', () => {
  const calls = [], context = { save() {}, restore() {}, strokeText: (...args) => calls.push(['stroke', ...args]), fillText: (...args) => calls.push(['fill', ...args]) }
  graphLabel('#eeeeee', '#202020')(context, { x: 10, y: 20, size: 5, label: '操作系统 · نظام التشغيل · 운영체제' }, { labelWeight: '600', labelSize: 14, labelFont: 'Test Font' })
  assert.equal(context.font, '600 14px Test Font')
  assert.equal(context.strokeStyle, '#202020'); assert.equal(context.fillStyle, '#eeeeee')
  assert.equal(context.lineWidth, 4)
  assert.deepEqual(calls, [['stroke', '操作系统 · نظام التشغيل · 운영체제', 21, 20], ['fill', '操作系统 · نظام التشغيل · 운영체제', 21, 20]])
  graphLabel('#eee', '#202020', 'rtl')(context, { x: 10, y: 20, size: 5, label: 'نظام التشغيل' }, { labelWeight: '600', labelSize: 14, labelFont: 'Test Font' })
  assert.equal(context.direction, 'rtl'); assert.equal(context.textAlign, 'right')
  assert.equal(calls.at(-1)[2], -1)
})

test('graph circles grow monotonically with degree, remain bounded, and do not plateau at ordinary hub sizes', () => {
  const degrees = [0, 1, 5, 20, 50, 100, 200, 1000, 10000]
  const radii = degrees.map(graphNodeRadius)
  assert.equal(radii[0], 3)
  for (let index = 1; index < radii.length; index++) assert.ok(radii[index] > radii[index - 1])
  assert.ok(graphNodeRadius(200) > graphNodeRadius(20) * 1.5)
  assert.ok(graphNodeRadius(10000) <= 23)
  assert.equal(graphNodeRadius(-10), 3)
  assert.equal(graphNodeRadius(NaN), 3)
  assert.equal(graphNodeRadius(Infinity), 3)
})

test('inside graph text chooses a high-contrast foreground for each node color', () => {
  assert.equal(graphNodeTextColor('#2366a8', 'fallback'), '#f8fafc')
  assert.equal(graphNodeTextColor('#78afe5', 'fallback'), '#111827')
  assert.equal(graphNodeTextColor('#ffffff', 'fallback'), '#111827')
  assert.equal(graphNodeTextColor('#000000', 'fallback'), '#f8fafc')
  assert.equal(graphNodeTextColor('unknown', 'fallback'), 'fallback')
})

test('both theme palettes retain readable inside labels for base, faded and focused circles', () => {
  const luminance = (color) => {
    const rgb = color.slice(1).match(/../g).map((part) => parseInt(part, 16) / 255)
      .map((value) => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4)
    return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722
  }
  for (const dark of [false, true]) {
    const palette = graphPalette(dark)
    assert.equal(graphNodeTextColor(palette.node, 'fallback'), dark ? '#111827' : '#f8fafc')
    for (const color of [palette.node, palette.faded, palette.focus]) {
      const background = luminance(color), foreground = luminance(graphNodeTextColor(color, 'fallback'))
      const contrast = (Math.max(background, foreground) + .05) / (Math.min(background, foreground) + .05)
      assert.ok(contrast >= 4.5, `${color}: ${contrast}`)
    }
  }
})

function labelContext() {
  const calls = []
  return { calls, save() {}, restore() {}, measureText(text) { return { width: Array.from(text).length * parseFloat(this.font.split(' ')[1]) * .6 } }, strokeText(text, x, y) { calls.push({ type: 'stroke', text, x, y }) }, fillText(text, x, y) { calls.push({ type: 'fill', text, x, y, font: this.font, color: this.fillStyle, align: this.textAlign }) } }
}
const labelSettings = { labelWeight: '600', labelSize: 14, labelFont: 'Test Font' }

test('large graph circles contain centered labels while small circles retain outside halos', () => {
  const draw = graphLabel('#202122', '#ffffff'), context = labelContext()
  draw(context, { x: 50, y: 40, size: 30, label: 'Linux', color: '#2366a8' }, labelSettings)
  assert.deepEqual(context.calls, [{ type: 'fill', text: 'Linux', x: 50, y: 40, font: '600 14px Test Font', color: '#f8fafc', align: 'center' }])
  context.calls.length = 0
  draw(context, { x: 50, y: 40, size: 8, label: 'Linux', color: '#2366a8' }, labelSettings)
  assert.equal(context.calls[0].type, 'stroke'); assert.equal(context.calls[1].align, 'left')
  assert.equal(context.calls[1].x, 64)
})

test('multiline graph labels fit circle chords without shrinking below twelve pixels or dropping text', () => {
  const context = labelContext(), draw = graphLabel('#eee', '#202020')
  draw(context, { x: 100, y: 100, size: 54, label: 'Berkeley Software Distribution', color: '#78afe5' }, labelSettings)
  assert.ok(context.calls.length >= 2 && context.calls.length <= 3)
  assert.equal(context.calls.map((call) => call.text).join(' '), 'Berkeley Software Distribution')
  for (const call of context.calls) {
    assert.equal(call.type, 'fill'); assert.equal(call.x, 100); assert.equal(call.color, '#111827')
    const fontSize = parseFloat(call.font.split(' ')[1])
    assert.ok(fontSize >= 12)
    const extent = Math.abs(call.y - 100) + fontSize * 1.2 / 2
    assert.ok((context.measureText(call.text).width / 2) ** 2 + extent ** 2 <= 50 ** 2)
  }
  context.calls.length = 0
  const title = '아주 긴 문서 제목은 작게 뭉개거나 생략하지 않고 바깥에서 읽을 수 있어야 한다'
  draw(context, { x: 100, y: 100, size: 24, label: title, color: '#2366a8' }, labelSettings)
  assert.equal(context.calls[0].type, 'stroke'); assert.equal(context.calls[1].text, title)
})

test('inside labels preserve grapheme clusters and reuse bounded text-fit measurements', () => {
  const draw = graphLabel('#eee', '#202020', 'rtl'), context = labelContext()
  let measurements = 0
  context.measureText = (text) => { measurements++; return { width: [...new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(text)].length * 8 } }
  const title = '👩🏽‍💻 e\u0301 🇰🇷 نظام التشغيل', data = { x: 100, y: 100, size: 40, label: title, color: '#78afe5' }
  draw(context, data, labelSettings)
  const first = measurements
  assert.ok(first > 0)
  assert.ok(context.calls.every((call) => call.type === 'fill' && call.align === 'center'))
  assert.equal(context.calls.map((call) => call.text).join(' '), title)
  draw(context, { ...data, x: 50 }, labelSettings)
  assert.equal(measurements, first)
})

test('an English word is not arbitrarily chopped into tiny lines just to fit a circle', () => {
  const context = labelContext()
  graphLabel('#202122', '#fff')(context, { x: 50, y: 40, size: 22, label: 'Fedora', color: '#2366a8' }, labelSettings)
  assert.deepEqual(context.calls.map((call) => call.text), ['Fedora', 'Fedora'])
  assert.equal(context.calls[0].type, 'stroke')
})

test('font completion refreshes label measurement without restarting layout, and its listener is removed', async () => {
  const screen = setup()
  try {
    await screen.mount()
    const original = screen.renderers[0].settings.defaultDrawNodeLabel
    screen.fontListeners.get('loadingdone')()
    assert.notEqual(screen.renderers[0].settings.defaultDrawNodeLabel, original)
    assert.equal(screen.renderers.length, 1); assert.equal(screen.layouts.length, 1)
  } finally { screen.stop() }
  assert.equal(screen.fontListeners.size, 0)
})

test('selecting a graph document retains neighbors, hides unrelated labels, and never navigates automatically', async () => {
  const screen = setup()
  try {
    await screen.mount()
    const renderer = screen.renderers[0], settings = renderer.settings
    assert.equal(renderer.graph.getNodeAttribute('a', 'size'), graphNodeRadius(3))
    assert.equal(renderer.getCanvases().hovers.style.zIndex, '1')
    assert.equal(renderer.getCanvases().hovers.style.pointerEvents, 'none')
    renderer.events.enterNode({ node: 'a' })
    assert.equal(settings.nodeReducer('b', { label: 'Linux' }).label, 'Linux')
    assert.equal(settings.nodeReducer('c', { label: '고립' }).label, '')
    assert.equal(settings.nodeReducer('c', { size: 10 }).size, 2)
    renderer.events.leaveNode()
    assert.equal(settings.nodeReducer('c', { label: '고립' }).label, '고립')
    renderer.events.clickNode({ node: 'a' })
    assert.equal(screen.view.selected.value.name, '운영체제')
    assert.deepEqual(screen.view.neighbors.value.map((n) => n.name), ['Linux'])
    assert.deepEqual(screen.routes, []); assert.equal(screen.layouts[0].running, false)
    assert.equal(settings.edgeReducer('link:0', { kind: 'link' }).color, graphPalette(false).focus)
    assert.equal(settings.nodeReducer('a', {}).forceLabel, true)
    assert.notEqual(settings.nodeReducer('b', {}).forceLabel, true)
    renderer.events.clickStage()
    assert.equal(screen.view.selected.value, undefined)
  } finally { screen.stop() }
})

test('search selection closes results, supports neighbors and reset; theme does not restart layout/camera', async () => {
  const screen = setup()
  try {
    await screen.mount()
    screen.view.query.value = 'Linux'; screen.view.searchChanged()
    assert.equal(screen.view.searchOpen.value, true)
    screen.view.focusNode(screen.view.matches.value[0])
    assert.equal(screen.view.searchOpen.value, false)
    assert.equal(screen.view.selected.value.name, 'Linux')
    screen.theme.value = 'dark'; await settle()
    assert.equal(screen.renderers.length, 1); assert.equal(screen.layouts.length, 1)
    assert.equal(screen.renderers[0].camera.ratio, .65)
    assert.equal(screen.view.selected.value.name, 'Linux')
    screen.view.zoom(true); assert.ok(screen.renderers[0].camera.ratio < .65)
    screen.view.resetView()
    assert.equal(screen.view.query.value, ''); assert.equal(screen.view.selected.value, undefined)
    assert.equal(screen.renderers[0].camera.ratio, 1)
  } finally { screen.stop() }
  assert.equal(screen.renderers[0].killed, true)
  assert.equal(screen.layouts[0].killed, true)
})

test('graph edition changes and unmount reject stale requests and their loading/error state', async () => {
  const first = deferred(), second = deferred(), third = deferred(), pending = [first, second, third], signals = []
  const screen = setup((_, signal) => { signals.push(signal); return pending.shift().promise })
  try {
    await screen.mount()
    screen.route.params.locale = 'en'
    assert.equal(signals[0].aborted, true)
    first.reject(Error('old failure')); await settle()
    assert.equal(screen.view.error.value, ''); assert.equal(screen.view.loading.value, true)
    second.resolve(fixture); await settle()
    assert.equal(screen.renderers.length, 1); assert.equal(screen.view.loading.value, false)
    screen.route.params.locale = 'fr'; screen.stop()
    assert.equal(signals[2].aborted, true)
    third.resolve(fixture); await settle()
    assert.equal(screen.renderers.length, 1)
    assert.equal(screen.view.payload.value, null)
  } catch (error) { screen.stop(); throw error }
})

test('large graph neighborhoods initially render only sixty entries and reset when exploring', async () => {
  const nodes = [node('center'), ...Array.from({ length: 150 }, (_, index) => node(`n${index}`))]
  const value = { ...fixture, nodes, edges: nodes.slice(1).map((n) => ({ source: 'center', target: n.id, kind: 'link' })) }
  const screen = setup(async () => value)
  try {
    await screen.mount(); screen.view.focusNode(nodes[0])
    assert.equal(screen.view.neighbors.value.length, 150)
    assert.equal(screen.view.visibleNeighbors.value.length, 60)
    screen.view.neighborLimit.value += 60
    assert.equal(screen.view.visibleNeighbors.value.length, 120)
    screen.view.focusNode(nodes[1])
    assert.equal(screen.view.neighborLimit.value, 60)
    assert.equal(screen.view.visibleNeighbors.value.length, 1)
  } finally { screen.stop() }
})

test('an exact graph title precedes highly connected partial matches', async () => {
  const value = { ...fixture, nodes: [{ ...node('dist', 'Linux 배포판'), degree: 999 }, node('kernel', 'Linux')] }
  const screen = setup(async () => value)
  try {
    await screen.mount(); screen.view.query.value = 'linux'
    assert.equal(screen.view.matches.value[0].name, 'Linux')
  } finally { screen.stop() }
})

test('natural physics has no community colors, seeded groups or weakened bridge links', async () => {
  const nodes = ['a', 'b', 'c', 'd', 'e', 'f'].map((id) => node(id)), edges = []
  for (const group of [['a', 'b', 'c'], ['d', 'e', 'f']]) for (let i = 0; i < 3; i++) for (let j = i + 1; j < 3; j++) edges.push({ source: group[i], target: group[j], kind: 'link' })
  edges.push({ source: 'c', target: 'd', kind: 'link' })
  const screen = setup(async () => ({ ...fixture, nodes, edges }))
  try {
    await screen.mount()
    const graph = screen.renderers[0].graph
    assert.deepEqual(new Set(graph.nodes().map((id) => graph.getNodeAttribute(id, 'color'))), new Set([graphPalette(false).node]))
    assert.ok(graph.edges().every((id) => graph.getEdgeAttribute(id, 'weight') === 1))
    assert.equal(screen.layouts[0].input.interval, 33)
    assert.equal(screen.renderers[0].settings.itemSizesReference, 'positions')
    assert.equal(screen.layouts[0].running, true)
    screen.theme.value = 'dark'; await settle()
    assert.deepEqual(new Set(graph.nodes().map((id) => graph.getNodeAttribute(id, 'color'))), new Set([graphPalette(true).node]))
  } finally { screen.stop() }
})

test('initial positions do not depend on connectivity, degree, input order or language grouping', () => {
  const initial = graphInitialPositions(fixture)
  const changed = { ...fixture, nodes: [...fixture.nodes].reverse().map((n) => ({ ...n, degree: 500, locale: 'en' })), edges: [] }
  assert.deepEqual(graphInitialPositions(changed), initial)
  assert.equal(new Set([...initial.values()].map((p) => `${p.x},${p.y}`)).size, fixture.nodes.length)
  assert.ok([...initial.values()].every((p) => Number.isFinite(p.x) && Number.isFinite(p.y)))
})

test('layout pauses while hidden, resumes only if previously running, and user pause survives visibility changes', async () => {
  const screen = setup()
  try {
    await screen.mount()
    assert.equal(screen.layouts[0].running, true)
    document.hidden = true; screen.documentListeners.get('visibilitychange')()
    assert.equal(screen.layouts[0].running, false)
    assert.equal(screen.view.layoutState.value, 'hidden')
    document.hidden = false; screen.documentListeners.get('visibilitychange')()
    assert.equal(screen.layouts[0].running, true)
    screen.view.toggleLayout(); assert.equal(screen.layouts[0].running, false)
    document.hidden = true; screen.documentListeners.get('visibilitychange')()
    document.hidden = false; screen.documentListeners.get('visibilitychange')()
    assert.equal(screen.layouts[0].running, false)
    screen.view.toggleLayout(); assert.equal(screen.layouts[0].running, true)
  } finally { screen.stop() }
  assert.equal(screen.documentListeners.size, 0)
})

test('hidden requests are aborted, late results are ignored and a visible screen retries the current graph', async () => {
  const old = deferred(), current = deferred(), signals = []
  const screen = setup((_, signal) => { signals.push(signal); return signals.length === 1 ? old.promise : current.promise })
  try {
    await screen.mount()
    document.hidden = true; screen.documentListeners.get('visibilitychange')()
    assert.equal(signals[0].aborted, true)
    document.hidden = false; screen.documentListeners.get('visibilitychange')()
    old.resolve(fixture); await settle()
    assert.equal(screen.renderers.length, 0)
    current.resolve(fixture); await settle()
    assert.equal(screen.renderers.length, 1)
  } finally { screen.stop() }
})

test('quiet simulation remains pausable, hidden frames stop, and frames update positions without rebuilding the camera', async () => {
  const screen = setup()
  try {
    await screen.mount()
    const coordinates = Float32Array.from([10, 20, 30, 40, 50, 60])
    screen.layouts[0].frame(coordinates, true)
    assert.equal(screen.view.layoutState.value, 'stable')
    assert.equal(screen.view.layoutActive.value, true)
    assert.equal(screen.renderers[0].graph.getNodeAttribute('b', 'x'), 30)
    assert.equal(screen.renderers.length, 1)
    document.hidden = true; screen.documentListeners.get('visibilitychange')()
    assert.equal(screen.layouts[0].running, false)
    document.hidden = false; screen.documentListeners.get('visibilitychange')()
    screen.view.toggleLayout()
    assert.equal(screen.layouts[0].running, false)
    assert.equal(screen.view.layoutState.value, 'paused')
  } finally { screen.stop() }
})

test('physics worker failure keeps document search and graph exploration available', async () => {
  const screen = setup()
  try {
    await screen.mount()
    screen.layouts[0].onerror()
    assert.equal(screen.view.layoutState.value, 'failed')
    assert.equal(screen.view.error.value, '')
    screen.view.query.value = 'Linux'
    screen.view.focusNode(screen.view.matches.value[0])
    assert.equal(screen.view.selected.value.name, 'Linux')
    assert.equal(screen.renderers[0].killed, undefined)
  } finally { screen.stop() }
})

test('graph styles and visualization stay lazy, with bounded mobile neighbor scroll', () => {
  const view = readFileSync(new URL('../src/views/GraphView.vue', import.meta.url), 'utf8')
  const css = readFileSync(new URL('../src/styles/graph.css', import.meta.url), 'utf8')
  const wiki = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  const router = readFileSync(new URL('../src/app/router.ts', import.meta.url), 'utf8')
  assert.match(view, /import '\.\.\/styles\/graph.css'/)
  assert.match(router, /import\('\.\.\/views\/GraphView.vue'\)/)
  assert.doesNotMatch(wiki, /\.graph-view\s*\{/)
  assert.match(css, /\.graph-neighbors\s*\{[^}]*min-height: 0[^}]*overflow: auto/)
  assert.match(css, /grid-template-rows: minmax\(150px, 1fr\) minmax\(0, 42%\)/)
  assert.match(view, /searchOpen && query.trim\(\)/)
  assert.match(view, /@keydown.enter.prevent/)
})
