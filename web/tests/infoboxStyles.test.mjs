import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

test('infobox images fit the box without floating or clearing the outer article', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /wiki-infobox \.wiki-image-infobox \{[^}]*float: none;[^}]*clear: none;[^}]*width: auto;[^}]*margin: 0;/)
  assert.match(css, /wiki-infobox \.wiki-image-infobox figcaption \{[^}]*flex-direction: column;[^}]*overflow-wrap: anywhere;/)
  assert.match(css, /wiki-image img \{[^}]*height: auto;[^}]*object-fit: contain;/)
})
