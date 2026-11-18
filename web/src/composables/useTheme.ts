import { readonly, ref } from 'vue'

type Theme = 'light' | 'dark'

const storageKey = 'techpedia.theme'
const theme = ref<Theme>('light')
let initialized = false

function preferredTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(storageKey)
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // Storage may be unavailable in privacy-restricted browser contexts.
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(value: Theme) {
  theme.value = value
  document.documentElement.dataset.theme = value
  document.documentElement.style.colorScheme = value
}

export function initializeTheme() {
  if (initialized) return
  initialized = true
  applyTheme(preferredTheme())
}

export function useTheme() {
  function toggleTheme() {
    const next: Theme = theme.value === 'dark' ? 'light' : 'dark'
    applyTheme(next)
    try { window.localStorage.setItem(storageKey, next) } catch { /* Keep the in-memory theme. */ }
  }

  return { theme: readonly(theme), toggleTheme }
}
