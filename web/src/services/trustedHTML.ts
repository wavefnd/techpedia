interface TrustedHTMLPolicy {
  createHTML(value: string): unknown
}

interface TrustedTypesFactory {
  createPolicy(name: string, rules: { createHTML(value: string): string }): TrustedHTMLPolicy
}

declare global {
  interface Window {
    trustedTypes?: TrustedTypesFactory
  }
}

let policy: TrustedHTMLPolicy | null | undefined

function techpediaHTMLPolicy(): TrustedHTMLPolicy | null {
  if (policy !== undefined) return policy
  if (!window.trustedTypes) {
    policy = null
    return policy
  }
  try {
    // Server-rendered TechMark and sanitized bundled repository documents may
    // pass through this policy. Their parsers remain the sanitization boundary.
    policy = window.trustedTypes.createPolicy('techpedia', { createHTML: (value) => value })
  } catch {
    policy = null
  }
  return policy
}

export function trustedTechMarkHTML(value: string): string {
  return (techpediaHTMLPolicy()?.createHTML(value) ?? value) as string
}

export function setTrustedHTML(element: HTMLElement, value: string): void {
  element.innerHTML = trustedTechMarkHTML(value)
}
