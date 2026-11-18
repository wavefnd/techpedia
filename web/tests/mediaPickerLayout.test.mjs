import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { parse } from '@vue/compiler-sfc'
import { parse as parseDOM } from '@vue/compiler-dom'

const source = readFileSync(new URL('../src/components/MediaPicker.vue', import.meta.url), 'utf8')
const root = parseDOM(parse(source).descriptor.template.content)
const elements = (node) => (node.children || []).filter((child) => child.type === 1)
const attribute = (node, name) => node.props.find((prop) => prop.type === 6 && prop.name === name)?.value?.content
const hasClass = (node, name) => (attribute(node, 'class') || '').split(' ').includes(name)
function find(node, className) {
  for (const child of elements(node)) {
    if (hasClass(child, className)) return child
    const match = find(child, className)
    if (match) return match
  }
}

test('media picker fixes header, tabs and actions while only the bounded body can grow and scroll', () => {
  const dialog = find(root, 'media-picker')
  assert.deepEqual(elements(dialog).map((child) => child.tag), ['header', 'nav', 'div', 'footer'])
  assert.ok(hasClass(elements(dialog)[2], 'media-picker-body'))
  assert.ok(find(find(root, 'media-picker-body'), 'media-picker-error'))
  const footer = find(root, 'media-picker-footer')
  assert.deepEqual(elements(footer).map((child) => attribute(child, 'class')), ['media-picker-options', 'media-picker-actions'])
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /\.media-picker \{[^}]*100dvh[^}]*display: flex;[^}]*flex-direction: column;/)
  assert.match(css, /\.media-picker-header, \.media-picker-tabs, \.media-picker-footer \{ flex: 0 0 auto;/)
  assert.match(css, /\.media-picker-body \{[^}]*flex: 1 1 0;[^}]*min-height: 0;[^}]*overflow: auto;/)
  assert.match(css, /\.media-library-list \{[^}]*min-height: 0;/)
  assert.doesNotMatch(css, /\.media-(?:library|upload-form) \{[^}]*min-height: 350px;/)
  assert.match(css, /\.media-picker-options \{[^}]*max-height: min\(180px, 30dvh\);[^}]*overflow: auto;/)
  assert.doesNotMatch(css, /\.media-picker-footer \{[^}]*overflow: auto;/)
  assert.match(css, /\.media-library \{[^}]*grid-template-rows: minmax\(0, 1fr\);/)
  assert.match(css, /\.media-results \{[^}]*overflow-y: scroll;[^}]*scrollbar-gutter: stable;/)
  assert.match(source, /<Teleport to="body">/)
  assert.match(source, /document\.body\.style\.overflow = 'hidden'/)
  assert.match(source, /document\.body\.style\.overflow = previousOverflow/)
})

test('fixed footer submit buttons remain associated with native validation forms', () => {
  const body = find(root, 'media-picker-body'), actions = find(root, 'media-picker-actions')
  const forms = elements(body).filter((node) => node.tag === 'form')
  const submitButtons = elements(actions).filter((node) => attribute(node, 'type') === 'submit')
  assert.equal(forms.length, 2)
  assert.equal(submitButtons.length, 2)
  for (const form of forms) {
    assert.ok(submitButtons.some((button) => attribute(button, 'form') === attribute(form, 'id')))
    assert.ok(form.props.some((prop) => prop.type === 7 && prop.name === 'on' && prop.arg?.content === 'submit' && prop.modifiers.some((modifier) => modifier.content === 'prevent')))
  }
})
