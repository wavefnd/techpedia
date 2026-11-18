import type { RouteLocationNormalized, Router } from 'vue-router'

type AnalyticsRouteKind = 'portal' | 'article' | 'search' | 'graph' | 'support' | 'history' | 'languages' | 'profile'

const routeKinds: Record<string, AnalyticsRouteKind> = {
  'language-portal': 'portal',
  article: 'article',
  search: 'search',
  'graph-all': 'graph',
  'graph-language': 'graph',
  support: 'support',
  history: 'history',
  languages: 'languages',
  'user-profile': 'profile',
}

export function bindPublicAnalytics(router: Router): () => void {
  let scheduled = 0
  let generation = 0
  let lastPath = ''

  const schedule = (route: RouteLocationNormalized) => {
    const kind = routeKinds[String(route.name || '')]
    if (!kind || privacyOptOut() || route.fullPath === lastPath) return
    lastPath = route.fullPath
    generation++
    const currentGeneration = generation
    window.clearTimeout(scheduled)
    scheduled = window.setTimeout(() => {
      if (currentGeneration === generation) void collect(kind, route)
    }, 1_200)
  }

  schedule(router.currentRoute.value)
  const unbind = router.afterEach((route) => schedule(route))
  return () => {
    generation++
    window.clearTimeout(scheduled)
    unbind()
  }
}

async function collect(kind: AnalyticsRouteKind, route: RouteLocationNormalized): Promise<void> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 8_000)
  const startedAt = performance.now()
  try {
    const ping = await fetch('/api/analytics/ping', {
      cache: 'no-store',
      credentials: 'omit',
      signal: controller.signal,
    })
    if (ping.status !== 204 || ping.headers.get('X-TechPedia-Analytics') !== 'enabled') return
    const measured = Math.round(performance.now() - startedAt)
    const locale = normalizedLocale(route.params.locale)
    await fetch('/api/analytics/visits', {
      method: 'POST',
      cache: 'no-store',
      credentials: 'omit',
      keepalive: true,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        route: kind,
        ...(locale ? { locale } : {}),
        rtt_ms: Math.max(1, Math.min(60_000, measured)),
      }),
    })
  } catch {
    // Analytics is best-effort and must never delay or break a reading route.
  } finally {
    window.clearTimeout(timeout)
  }
}

function normalizedLocale(value: unknown): string {
  const locale = Array.isArray(value) ? String(value[0] || '') : String(value || '')
  return /^[a-z]{2,3}$/i.test(locale) ? locale.toLowerCase() : ''
}

function privacyOptOut(): boolean {
  const value = navigator as Navigator & { globalPrivacyControl?: boolean }
  return value.globalPrivacyControl === true || navigator.doNotTrack === '1'
}
