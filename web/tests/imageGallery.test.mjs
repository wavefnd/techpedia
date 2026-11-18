import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import { bindImageGalleries, galleryIndex, nextGalleryIndex } from '../src/services/imageGallery.ts'

test('image picker escapes commas only in uploaded file names', () => {
  const { descriptor } = parse(readFileSync(new URL('../src/components/MediaPicker.vue', import.meta.url), 'utf8'))
  const code = ts.transpileModule(compileScript(descriptor, { id: 'media-picker-gallery' }).content, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const modules = {
    vue: { ...vue, onMounted() {}, onBeforeUnmount() {} },
    '@lucide/vue': {},
    '../services/api': {},
    '../services/auth': { hasPermission: () => false },
  }, output = { exports: {} }, scope = vue.effectScope()
  new Function('require', 'module', 'exports', code)((name) => {
    assert.ok(name in modules, `Unexpected module ${name}`)
    return modules[name]
  }, output, output.exports)
  const picker = scope.run(() => output.exports.default.setup({}, { expose() {}, emit() {} }))
  try {
    picker.alt.value = 'Desktop'; picker.caption.value = 'caption'
    assert.equal(picker.imageSyntax('file', 'Linux,Mint.webp'), '<wiki-image file="Linux\\,Mint.webp" layout="frame" alt="Desktop">caption</wiki-image>')
    assert.equal(picker.imageSyntax('file', 'Mint.webp'), '<wiki-image file="Mint.webp" layout="frame" alt="Desktop">caption</wiki-image>')
    assert.equal(picker.imageSyntax('src', 'https://example.com/a,b.webp'), '<wiki-image src="https://example.com/a,b.webp" layout="frame" alt="Desktop">caption</wiki-image>')
  } finally { scope.stop() }
})

test('multiple pictures use a single horizontal viewport and conditional gallery runtime', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /\.wiki-image-gallery \{[^}]*display: flex;[^}]*scroll-snap-type: x mandatory/s)
  assert.match(css, /\.wiki-image-gallery > \.wiki-image-stage \{[^}]*flex: 0 0 100%/s)
  assert.match(css, /\.wiki-image-side \{[^}]*width: min\(44%, 440px\)/s)
  const directive = readFileSync(new URL('../src/services/articleHTML.ts', import.meta.url), 'utf8')
  assert.match(directive, /if \(!element\.querySelector\('\.wiki-image-gallery'\)\) return/)
  assert.match(directive, /import\('\.\/imageGallery'\)/)
  assert.match(directive, /beforeUnmount: clearGalleries/)
})

test('gallery selection is geometry based and cycles across duplicates and boundaries', () => {
  assert.equal(galleryIndex([50, 150, 250], 149), 1)
  assert.equal(galleryIndex([-250, -150, -50], -51), 2)
  assert.equal(nextGalleryIndex(0, -1, 3), 2)
  assert.equal(nextGalleryIndex(2, 1, 3), 0)
  assert.equal(nextGalleryIndex(0, 1, 0), 0)
})

test('section titles clear floated infoboxes and images without manually authored br tags', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /\.techmark-content :is\(h1, h2, h3, h4, h5, h6\), \.techmark-section-viewport \{ clear: both; \}/)
  assert.match(css, /\.techmark-section-content \{ display: flow-root;/)
})

test('gallery arrows, swipe position, keyboard focus and cleanup operate on the current DOM', () => {
  const originals = { Element: globalThis.Element, HTMLElement: globalThis.HTMLElement, window: globalThis.window }
  const listeners = new Map(), frames = new Map(); let frameID = 0, offset = 0, focused = null
  class Node {
    constructor(kind) { this.kind = kind; this.isConnected = true; this.classList = { contains: (value) => kind === value }; this.tabIndex = 0 }
    closest(selector) { return selector === 'wiki-image figure' ? figure : this.kind === 'wiki-image-next' || this.kind === 'wiki-image-previous' ? this : null }
    getBoundingClientRect() { return { left: this.kind.startsWith('slide') ? Number(this.kind.slice(5)) * 100 - offset : 0, width: 100 } }
    querySelectorAll() { return slides }
    querySelector(selector) { return selector === '.wiki-image-gallery' ? gallery : output }
    scrollBy({ left }) { offset += left }
    focus() { focused = this }
  }
  const figure = new Node('figure'), gallery = new Node('wiki-image-gallery'), output = new Node('output')
  gallery.parentElement = figure
  const slides = [new Node('slide0'), new Node('slide1'), new Node('slide2')]
  slides.forEach((slide) => { slide.classList = { contains: (v) => v === 'wiki-image-stage' } })
  const root = {
    querySelectorAll: () => [gallery], contains: () => true,
    addEventListener: (name, handler) => listeners.set(name, handler), removeEventListener: (name) => listeners.delete(name),
  }
  globalThis.Element = Node; globalThis.HTMLElement = Node
  globalThis.window = { requestAnimationFrame: (fn) => { const id = ++frameID; frames.set(id, fn); return id }, cancelAnimationFrame: (id) => frames.delete(id) }
  try {
    const dispose = bindImageGalleries(root)
    assert.equal(output.textContent, '1 / 3'); assert.deepEqual(slides.map((s) => s.tabIndex), [0, -1, -1])
    listeners.get('click')({ target: new Node('wiki-image-next'), preventDefault() {} })
    assert.equal(output.textContent, '2 / 3'); assert.equal(offset, 100)
    listeners.get('keydown')({ target: slides[1], key: 'End', preventDefault() {} })
    assert.equal(output.textContent, '3 / 3'); assert.equal(focused, slides[2])
    listeners.get('click')({ target: new Node('wiki-image-next'), preventDefault() {} })
    assert.equal(output.textContent, '1 / 3')
    offset = 200; listeners.get('scroll')({ target: gallery })
    frames.values().next().value(); frames.clear(); assert.equal(output.textContent, '3 / 3')
    listeners.get('scroll')({ target: gallery }); assert.equal(frames.size, 1)
    dispose(); assert.equal(listeners.size, 0); assert.equal(frames.size, 0)
  } finally { Object.assign(globalThis, originals) }
})
