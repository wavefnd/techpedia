import type { LanguageEdition, SupportSettings } from '../types/api'
import { syncPortalLanguageEditions } from './editions'
import { syncPublicSupportSettings } from './support'

interface InitialPortalData {
  editions: Array<Omit<LanguageEdition, 'interface_messages' | 'navigation'>>
  support: SupportSettings
}

export function captureInitialPortal(): void {
  const element = document.getElementById('techpedia-portal-data')
  if (!(element instanceof HTMLScriptElement)) return
  element.remove()
  try {
    const value = JSON.parse(element.textContent || '') as InitialPortalData
    if (!Array.isArray(value.editions) || !value.support) return
    syncPortalLanguageEditions(value.editions.map((edition) => ({
      ...edition,
      interface_messages: {},
      navigation: [],
    })))
    syncPublicSupportSettings(value.support)
  } catch {
    // The normal API loaders remain available when initial data is malformed.
  }
}
