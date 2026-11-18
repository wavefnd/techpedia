// Invalidate at input change, not only after a debounce starts the next fetch.
export class LatestRequest {
  private generation = 0
  private disposed = false
  private controller: AbortController | null = null

  invalidate() { this.generation++; this.controller?.abort(); this.controller = null }
  dispose() { this.disposed = true; this.invalidate() }

  async run<T>(work: (signal: AbortSignal) => Promise<T>, handlers: {
    success: (value: T) => void
    error: (reason: unknown) => void
    settled: () => void
  }) {
    if (this.disposed) return
    this.invalidate()
    const generation = this.generation
    const controller = new AbortController()
    this.controller = controller
    const current = () => !this.disposed && this.generation === generation
    try {
      const value = await work(controller.signal)
      if (current()) handlers.success(value)
    } catch (reason) {
      if (current()) handlers.error(reason)
    } finally {
      if (current()) { this.controller = null; handlers.settled() }
    }
  }
}
