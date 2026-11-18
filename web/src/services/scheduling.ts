type IdleWindow = Window & typeof globalThis & {
  requestIdleCallback?: (callback: () => void, options?: { timeout: number }) => number
  cancelIdleCallback?: (handle: number) => void
}

export function scheduleWhenIdle(task: () => void, timeout = 1500): () => void {
  const browser = window as IdleWindow
  if (browser.requestIdleCallback) {
    const handle = browser.requestIdleCallback(task, { timeout })
    return () => browser.cancelIdleCallback?.(handle)
  }
  const handle = window.setTimeout(task, Math.min(timeout, 250))
  return () => window.clearTimeout(handle)
}

export function waitForIdle(timeout = 1500): Promise<void> {
  return new Promise((resolve) => scheduleWhenIdle(resolve, timeout))
}
