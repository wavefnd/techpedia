import { computed, ref } from 'vue'
import type { SupportSettings } from '../types/api'

export const publicSupportSettings = ref<SupportSettings | null>(null)
export const supportAvailable = computed(() => Boolean(publicSupportSettings.value?.enabled))

let loaded = false
let request: Promise<SupportSettings | null> | null = null

export function syncPublicSupportSettings(settings: SupportSettings): void {
  publicSupportSettings.value = settings.enabled ? settings : null
  loaded = true
}

export async function loadPublicSupportSettings(force = false): Promise<SupportSettings | null> {
  if (loaded && !force) return publicSupportSettings.value
  if (request && !force) return request

  request = import('./api').then(({ getSupportSettings }) => getSupportSettings())
    .then((settings) => {
      syncPublicSupportSettings(settings)
      return settings
    })
    .catch(() => {
      publicSupportSettings.value = null
      return null
    })

  try {
    return await request
  } finally {
    loaded = true
    request = null
  }
}
