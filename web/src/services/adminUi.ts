import type { App } from 'vue'
import { retryableLoad } from './retryableLoad'

let app: App | null = null
const install = retryableLoad(async () => {
  const { installAdminUI } = await import('../admin-vuestic')
  installAdminUI(app!)
})

export function bindAdminUI(target: App) {
  app = target
}

export function ensureAdminUI(): Promise<void> {
  if (!app) return Promise.reject(new Error('Vue application is not ready'))
  return install()
}
