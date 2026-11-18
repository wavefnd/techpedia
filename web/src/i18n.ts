import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { findLanguageEdition } from './services/editions'
import type { WikiMessageKey } from './types/api'

export const wikiMessageKeys = [
  'menu', 'close', 'tagline', 'searchLabel', 'searchPlaceholder', 'search', 'noResults',
  'userActions', 'account', 'adminPanel', 'logout', 'login', 'signup', 'navigation',
  'languages', 'addLanguage', 'sidebarNote', 'toc', 'section', 'document', 'read',
  'discussion', 'edit', 'history', 'contentLicense', 'updated', 'historyTitle',
  'currentArticle', 'noSummary', 'noRevisions', 'selectRevision', 'loadingHistory',
  'historyError', 'revisionError', 'editor', 'cancel', 'save', 'saving', 'loadingSource',
  'documentName', 'editSummary', 'summaryPlaceholder', 'source', 'autocomplete',
  'livePreview', 'serverRendering', 'saved', 'sourceError', 'previewError', 'saveError',
  'graph', 'graphAll', 'graphLanguage', 'graphSearch', 'graphLoading', 'graphEmpty',
  'graphReset', 'graphDocuments', 'graphLinks', 'graphTruncated',
] as const satisfies readonly WikiMessageKey[]

export const optionalWikiMessageKeys = [
  'theme', 'lines', 'characters', 'scrollTop', 'scrollBottom', 'backlinks', 'noBacklinks',
  'graphLayoutStart', 'graphLayoutPause', 'graphLayoutRunning', 'graphLayoutStable', 'graphLayoutPaused', 'graphLayoutFailed',
] as const satisfies readonly WikiMessageKey[]
export const allWikiMessageKeys = [...wikiMessageKeys, ...optionalWikiMessageKeys] as const

export function canonicalLanguageTag(value: unknown): string {
  if (typeof value !== 'string' || value === '') return 'ko'
  try { return Intl.getCanonicalLocales(value)[0] || 'ko' } catch { return 'ko' }
}

export function useWikiI18n() {
  const route = useRoute()
  const locale = computed(() => canonicalLanguageTag(route.params.locale || route.query.lang))
  const uiLocale = computed<'ko' | 'en'>(() => locale.value.toLowerCase().split('-')[0] === 'ko' ? 'ko' : 'en')
  const edition = computed(() => findLanguageEdition(locale.value))
  const t = (key: WikiMessageKey): string => edition.value?.interface_messages?.[key] || key
  const mainPageName = computed(() => edition.value?.main_page_name || '')
  const wikiURL = (name: string) => documentURL('wiki', locale.value, name)
  return { locale, uiLocale, edition, mainPageName, t, wikiURL }
}

export function encodeDocumentName(name: string): string {
  return [...name.trim().replaceAll(' ', '_')]
    .map((character) => '/#?\\'.includes(character) ? encodeURIComponent(character) : encodeURI(character))
    .join('')
}

export function documentURL(surface: 'wiki' | 'edit' | 'history', locale: string, name: string): string {
  const prefix = surface === 'wiki' ? `/${locale}/wiki` : `/${surface}/${locale}`
  return `${prefix}/${encodeDocumentName(name)}`
}

export function languageDisplayName(tag: string, displayLocale: string): string {
  try { return new Intl.DisplayNames([displayLocale], { type: 'language' }).of(tag) || tag } catch { return tag }
}
