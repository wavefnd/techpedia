type BundledWritingSystem = 'Kore' | 'Jpan' | 'Hans' | 'Hant' | 'Arab' | 'Deva' | 'Beng' | 'Grek' | 'Hebr' | 'Ethi'

const loaders: Record<BundledWritingSystem, () => Promise<unknown>> = {
  Kore: () => import('../styles/fonts-kore.css'),
  Jpan: () => import('../styles/fonts-jpan.css'),
  Hans: () => import('../styles/fonts-hans.css'),
  Hant: () => import('../styles/fonts-hant.css'),
  Arab: () => import('../styles/fonts-arab.css'),
  Deva: () => import('../styles/fonts-deva.css'),
  Beng: () => import('../styles/fonts-beng.css'),
  Grek: () => import('../styles/fonts-grek.css'),
  Hebr: () => import('../styles/fonts-hebr.css'),
  Ethi: () => import('../styles/fonts-ethi.css'),
}

const loading = new Map<BundledWritingSystem, Promise<unknown>>()
let cjkCodeLoading: Promise<unknown> | undefined
const cjkPattern = /[\p{Script=Han}\p{Script=Hangul}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Bopomofo}]/u

function isBundledWritingSystem(value: string | undefined): value is BundledWritingSystem {
  return Boolean(value && value in loaders)
}

export function ensureWritingSystemFont(value: string | undefined): Promise<unknown> {
  if (!isBundledWritingSystem(value)) return Promise.resolve()
  const existing = loading.get(value)
  if (existing) return existing
  const request = loaders[value]().catch(() => {
    loading.delete(value)
  })
  loading.set(value, request)
  return request
}

export function ensureCJKCodeFont(text: string): void {
  if (!cjkPattern.test(text) || cjkCodeLoading) return
  cjkCodeLoading = import('../styles/fonts-code-cjk.css').catch(() => {
    cjkCodeLoading = undefined
  })
}

function detectedWritingSystems(text: string): Set<BundledWritingSystem> {
  const detected = new Set<BundledWritingSystem>()
  if (/\p{Script=Hangul}/u.test(text)) detected.add('Kore')
  if (/[\p{Script=Hiragana}\p{Script=Katakana}]/u.test(text)) detected.add('Jpan')
  if (/\p{Script=Arabic}/u.test(text)) detected.add('Arab')
  if (/\p{Script=Devanagari}/u.test(text)) detected.add('Deva')
  if (/\p{Script=Bengali}/u.test(text)) detected.add('Beng')
  if (/\p{Script=Greek}/u.test(text)) detected.add('Grek')
  if (/\p{Script=Hebrew}/u.test(text)) detected.add('Hebr')
  if (/\p{Script=Ethiopic}/u.test(text)) detected.add('Ethi')
  return detected
}

export function ensurePageFonts(): void {
  const primary = document.documentElement.dataset.writingSystem
  void ensureWritingSystemFont(primary)

  const text = document.body.textContent || ''
  const detected = detectedWritingSystems(text)
  if (/\p{Script=Han}/u.test(text)) {
    detected.add(primary === 'Kore' || primary === 'Jpan' || primary === 'Hant' ? primary : 'Hans')
  }
  for (const writingSystem of detected) void ensureWritingSystemFont(writingSystem)

  for (const code of document.querySelectorAll('code, pre')) {
    const value = code.textContent || ''
    if (!cjkPattern.test(value)) continue
    ensureCJKCodeFont(value)
    break
  }
}
