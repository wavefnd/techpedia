import { computed, ref } from 'vue'
import type { LanguageEdition } from '../types/api'

export const languageEditions = ref<LanguageEdition[]>([])
const loaded = ref(false)
let request: Promise<LanguageEdition[]> | null = null

export const activeLanguageEditions = computed(() => languageEditions.value.filter((value) => value.status === 'active'))

export function syncLanguageEditions(values: LanguageEdition[]): void {
  languageEditions.value = values
  loaded.value = true
}

export function syncPortalLanguageEditions(values: LanguageEdition[]): void {
  languageEditions.value = values
  loaded.value = false
}

export function syncInitialLanguageEdition(value: LanguageEdition): void {
  languageEditions.value = [...languageEditions.value.filter((edition) => edition.tag !== value.tag), value]
}

export async function ensureLanguageEdition(tag: string): Promise<LanguageEdition | undefined> {
  const current = findLanguageEdition(tag)
  if (current?.interface_messages.searchLabel && current.navigation.length) return current
  await loadLanguageEditions()
  return findLanguageEdition(tag)
}

export async function loadLanguageEditions(force = false): Promise<LanguageEdition[]> {
  if (loaded.value && !force) return languageEditions.value
  if (request && !force) return request
  request = import('./api').then(({ getLanguageEditions }) => getLanguageEditions()).then((values) => {
    syncLanguageEditions(values)
    return values
  })
  try {
    return await request
  } finally {
    request = null
  }
}

export function findLanguageEdition(tag: string): LanguageEdition | undefined {
  return languageEditions.value.find((value) => value.tag.toLowerCase() === tag.toLowerCase())
}
