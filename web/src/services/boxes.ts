export function boxSyntax(name: string): string {
  const escaped = name.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  return `<wiki-box name="${escaped}" />`
}

export function boxAvailability(locales: string[], fallback: string, locale: string) {
  if (locales.includes(locale)) return { locale, available: true, fallback: false }
  if (fallback && locales.includes(fallback)) return { locale: fallback, available: true, fallback: true }
  return { locale, available: false, fallback: false }
}
