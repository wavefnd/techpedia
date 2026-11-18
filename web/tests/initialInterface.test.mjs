import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'

function evaluate(path, modules) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const output = { exports: {} }
  new Function('require', 'module', 'exports', code)((name) => {
    assert.ok(name in modules, `Unexpected module ${name}`)
    return modules[name]
  }, output, output.exports)
  return output.exports
}

function edition(tag = 'ko') {
  return {
    tag, status: 'active', name: tag, native_name: tag, direction: tag === 'ar' ? 'rtl' : 'ltr',
    interface_messages: { searchLabel: `${tag} search`, history: `${tag} history`, updated: `${tag} updated` },
    navigation: [{ title: `${tag} navigation`, items: [{ label: 'Main', target: 'Main' }] }],
    main_page_name: 'Main', main_page_url: `/${tag}/wiki/Main`, document_count: 0,
  }
}

function setup(initial, getLanguageEditions = async () => { throw new Error('API blocked') }) {
  const editions = evaluate('../src/services/editions.ts', { vue, './api': { getLanguageEditions } })
  const page = evaluate('../src/services/initialPage.ts', { './editions': editions })
  class Script { constructor(value) { this.textContent = value; this.removed = false } remove() { this.removed = true } }
  const script = new Script(typeof initial === 'string' ? initial : JSON.stringify(initial))
  globalThis.HTMLScriptElement = Script
  globalThis.document = {
    documentElement: { lang: initial?.locale || 'ko' },
    getElementById: () => script.removed ? null : script,
    querySelector: () => ({ innerHTML: '<p>Server-rendered article body</p>' }),
  }
  return { page, editions, script }
}

test('article language messages, navigation and SSR body do not require an API call', async () => {
  for (const locale of ['ko', 'en', 'ar']) {
    let calls = 0
    const screen = setup({ locale, name: 'Test', html: '', edition: edition(locale) }, async () => { calls++; throw new Error('blocked') })
    screen.page.captureInitialPage()
    const value = await screen.editions.ensureLanguageEdition(locale)
    assert.equal(calls, 0)
    assert.equal(value.interface_messages.history, `${locale} history`)
    assert.equal(value.navigation[0].title, `${locale} navigation`)
    assert.equal(value.direction, locale === 'ar' ? 'rtl' : 'ltr')
    assert.equal(screen.page.consumeInitialArticle(locale, 'Test').html, '<p>Server-rendered article body</p>')
  }
})

test('one bootstrapped edition never marks the entire language catalog loaded', async () => {
  let calls = 0
  const screen = setup({ locale: 'ko', name: 'Test', edition: edition() }, async () => { calls++; return [edition(), edition('en')] })
  screen.page.captureInitialPage()
  await screen.editions.ensureLanguageEdition('ko')
  assert.equal(calls, 0)
  assert.equal((await screen.editions.ensureLanguageEdition('en')).tag, 'en')
  assert.equal(calls, 1)
  assert.equal(screen.editions.languageEditions.value.length, 2)
  await screen.editions.loadLanguageEditions()
  assert.equal(calls, 1)
})

test('an unavailable language API preserves the server messages and remains retryable', async () => {
  let failed = true
  const screen = setup({ locale: 'ko', name: 'Test', edition: edition() }, async () => {
    if (failed) throw new Error('robots or network blocked')
    return [edition(), edition('en')]
  })
  screen.page.captureInitialPage()
  await assert.rejects(screen.editions.ensureLanguageEdition('en'))
  assert.equal(screen.editions.findLanguageEdition('ko').interface_messages.searchLabel, 'ko search')
  failed = false
  assert.equal((await screen.editions.ensureLanguageEdition('en')).tag, 'en')
})

test('malformed, mismatched or inactive bootstrap data uses the normal API fallback', async () => {
  for (const initial of ['{', { locale: 'ko', edition: edition('en') }, { locale: 'ko', edition: { ...edition(), status: 'pending' } }]) {
    let calls = 0
    const screen = setup(initial, async () => { calls++; return [edition()] })
    screen.page.captureInitialPage()
    assert.equal(screen.editions.languageEditions.value.length, 0)
    await screen.editions.ensureLanguageEdition('ko')
    assert.equal(calls, 1)
  }
})

test('client navigation preserves the longer SEO description without changing the short reader excerpt', () => {
  const view = readFileSync(new URL('../src/views/ArticleView.vue', import.meta.url), 'utf8')
  assert.match(view, /setMeta\('description', article\.value\.meta_description \|\| article\.value\.description\)/)
})

test('the article language switch uses existing page links without eager catalog requests', () => {
  const source = readFileSync(new URL('../src/components/LanguageSwitch.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(source, /onMounted|void loadLanguageEditions/)
  assert.match(source, /async function openTranslation\(\)[\s\S]*await loadLanguageEditions\(\)/)
})

test('compact code tokens have no color override that would be lost by unwrapping', () => {
  const css = readFileSync(new URL('../src/styles/wiki.css', import.meta.url), 'utf8')
  assert.doesNotMatch(css, /\.syntax-(?:n|p|w)(?![a-zA-Z0-9_-])/)
})
