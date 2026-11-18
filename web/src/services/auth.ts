import { computed, ref } from 'vue'
import type { Account, Permission } from '../types/api'

export const account = ref<Account | null>(null)
const sessionLoaded = ref(false)
let sessionRequest: Promise<Account | null> | null = null

export const isLoggedIn = computed(() => account.value !== null)

export function hasPermission(permission: Permission): boolean {
  const permissions = account.value?.permissions
  if (!permissions) return false
  if (permission === 'owner') return permissions.includes('owner')
  return permissions.includes('owner') || permissions.includes('admin') || permissions.includes(permission)
}

async function authRequest(path: string, init: RequestInit = {}): Promise<Account> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...init.headers,
    },
  })
  if (!response.ok) {
    const message = (await response.text()).trim()
    throw new Error(message || `Request failed with status ${response.status}`)
  }
  return response.json() as Promise<Account>
}

export async function loadSession(force = false): Promise<Account | null> {
  if (sessionLoaded.value && !force) return account.value
  if (sessionRequest && !force) return sessionRequest
  sessionRequest = (async () => {
    const response = await fetch('/api/auth/session', { headers: { Accept: 'application/json' } })
    sessionLoaded.value = true
    if (!response.ok) {
      account.value = null
      return null
    }
    account.value = await response.json() as Account
    return account.value
  })()
  try {
    return await sessionRequest
  } finally {
    sessionRequest = null
  }
}

export async function login(username: string, password: string): Promise<Account> {
  const user = await authRequest('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  account.value = user
  sessionLoaded.value = true
  return user
}

export async function signup(username: string, displayName: string, password: string): Promise<Account> {
  const user = await authRequest('/api/auth/signup', {
    method: 'POST',
    body: JSON.stringify({ username, display_name: displayName, password }),
  })
  account.value = user
  sessionLoaded.value = true
  return user
}

export async function updateProfile(displayName: string, bio: string): Promise<Account> {
  const user = await authRequest('/api/account/profile', {
    method: 'PATCH',
    body: JSON.stringify({ display_name: displayName, bio }),
  })
  account.value = user
  return user
}

export async function logout(): Promise<void> {
  await fetch('/api/auth/logout', { method: 'POST', headers: { Accept: 'application/json' } })
  account.value = null
  sessionLoaded.value = true
}
