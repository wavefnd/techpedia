import type { ArticleResponse, LanguageEdition, Locale } from '../types/api'
import { syncInitialLanguageEdition } from './editions'

let initialArticleHTML = ''
let initialArticleElement: HTMLElement | null = null
let initialArticleHost: HTMLElement | null = null
let initialArticleNodes: DocumentFragment | null = null
let initialArticleAccepted = false

function discardInitialArticle(): void {
  initialArticleHost?.remove()
  initialArticleElement = null
  initialArticleHost = null
  initialArticleNodes = null
  initialArticleHTML = ''
}

export function captureInitialPage(): void {
  initialArticleElement = document.querySelector<HTMLElement>('.seo-initial-content .techmark-content')
  initialArticleHTML = initialArticleElement?.innerHTML || ''
  const element = document.getElementById('techpedia-page-data')
  if (!(element instanceof HTMLScriptElement)) return
  try {
    const value = JSON.parse(element.textContent || '') as ArticleResponse & { edition?: LanguageEdition }
    const edition = value.edition
    if (edition?.tag === value.locale && edition.tag === document.documentElement.lang
      && edition.status === 'active' && edition.interface_messages && Array.isArray(edition.navigation)) {
      syncInitialLanguageEdition(edition)
    }
  } catch {
    // A malformed bootstrap must leave the normal API fallback available.
  }
}

export function preserveInitialArticleNodes(): void {
  if (!initialArticleElement || !initialArticleHTML) return
  const app = document.getElementById('app')
  const host = initialArticleElement.closest<HTMLElement>('.seo-initial-content')
  if (app && host && app.contains(host)) {
    app.before(host)
    initialArticleHost = host
    return
  }
  initialArticleNodes = document.createDocumentFragment()
  while (initialArticleElement.firstChild) initialArticleNodes.append(initialArticleElement.firstChild)
  initialArticleElement = null
}

export function consumeInitialArticle(locale: Locale, name: string): ArticleResponse | null {
  const element = document.getElementById('techpedia-page-data')
  if (!(element instanceof HTMLScriptElement)) return null
  element.remove()
  try {
    const value = JSON.parse(element.textContent || '') as ArticleResponse
    if (value.locale.toLowerCase() !== locale.toLowerCase() || value.name !== name || !initialArticleHTML) {
      discardInitialArticle()
      return null
    }
    value.html = initialArticleHTML
    initialArticleAccepted = true
    return value
  } catch {
    discardInitialArticle()
    return null
  }
}

export function restoreInitialArticleNodes(target: HTMLElement): boolean {
  if (!initialArticleAccepted) return false
  if (initialArticleHost && initialArticleElement) {
    const nodes = document.createDocumentFragment()
    while (initialArticleElement.firstChild) nodes.append(initialArticleElement.firstChild)
    target.append(nodes)
    initialArticleHost.remove()
  } else if (initialArticleNodes) {
    target.append(initialArticleNodes)
  } else {
    return false
  }
  initialArticleElement = null
  initialArticleHost = null
  initialArticleNodes = null
  initialArticleHTML = ''
  initialArticleAccepted = false
  return true
}
