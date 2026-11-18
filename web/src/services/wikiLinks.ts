import { resolveWikiLinks } from './api'
import { waitForIdle } from './scheduling'

const resolveBatchSize = 80
const domBatchSize = 200
const pending = new WeakMap<HTMLElement, AbortController>()

export function cancelMissingWikiLinks(container: HTMLElement | null): void {
  if (!container) return
  pending.get(container)?.abort()
  pending.delete(container)
}

export async function markMissingWikiLinks(container: HTMLElement | null, locale: string): Promise<void> {
  if (!container) return
  cancelMissingWikiLinks(container)
  const controller = new AbortController()
  pending.set(container, controller)
  const pagePath = window.location.pathname
  const current = () => !controller.signal.aborted && container.isConnected
    && pending.get(container) === controller && window.location.pathname === pagePath
  try {
    await waitForIdle(1800)
    if (!current()) return
    const links = container.querySelectorAll<HTMLAnchorElement>('a.wiki-link')
    const grouped = new Map<string, HTMLAnchorElement[]>()
    let processed = 0
    for (const link of links) {
      if (++processed % domBatchSize === 0) {
        await waitForIdle(50)
        if (!current()) return
      }
      const target = wikiTarget(link, locale)
      if (!target) continue
      const group = grouped.get(target) ?? []
      group.push(link)
      grouped.set(target, group)
    }
    const targets = Array.from(grouped.keys())
    if (!targets.length) return

    for (let index = 0; index < targets.length; index += resolveBatchSize) {
      if (!current()) return
      const resolved = await resolveWikiLinks(locale, targets.slice(index, index + resolveBatchSize), controller.signal)
      if (!current()) return
      for (const result of resolved) {
        for (const link of grouped.get(result.target) ?? []) {
          if (++processed % domBatchSize === 0) {
            await waitForIdle(50)
            if (!current()) return
          }
          if (!link.isConnected) continue
          link.classList.toggle('missing', !result.exists)
          link.classList.toggle('exists', result.exists)
          link.href = result.url
          link.title = result.exists
            ? result.name
            : (locale.toLowerCase().startsWith('ko') ? `${result.name} 문서는 아직 없습니다.` : `${result.name} does not exist yet.`)
        }
        grouped.delete(result.target)
      }
    }
  } catch {
    // Link decoration is optional: cancellation or an unavailable resolver
    // must not reject an unawaited task or mark an unknown page as missing.
  } finally {
    if (pending.get(container) === controller) pending.delete(container)
  }
}

function wikiTarget(link: HTMLAnchorElement, locale: string): string {
  const prefix = `/${locale}/wiki/`
  if (!link.pathname.startsWith(prefix)) return ''
  try {
    return decodeURIComponent(link.pathname.slice(prefix.length)).replaceAll('_', ' ')
  } catch {
    return link.pathname.slice(prefix.length).replaceAll('_', ' ')
  }
}
