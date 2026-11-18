import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

test('navbox hierarchy stays compact, direction-aware and stacked on mobile', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.match(css, /\.wiki-navbox-table \{[^}]*min-width: 0;/)
  assert.doesNotMatch(css, /\.wiki-navbox-table[^}]*min-width: (?:640|720)px/)
  assert.match(css, /\.wiki-navbox-parent-column \{ width: 84px;/)
  assert.match(css, /\.wiki-navbox-deep col\[class\] \{ width: calc\(48% \/ var\(--navbox-depth\)\);/)
  assert.doesNotMatch(css, /wiki-navbox-(?:nested|level)|--navbox-indent/)
  assert.match(css, /\.wiki-navbox-table colgroup \{ display: none;/)
  assert.match(css, /\.wiki-navbox-descendant \{ padding: 7px 11px !important; border-inline-start:/)
})
