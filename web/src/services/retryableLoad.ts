// Share concurrent work and retain success, but allow retry after rejection.
export function retryableLoad(work: () => Promise<void>): () => Promise<void> {
  let pending: Promise<void> | null = null
  return () => {
    if (!pending) pending = Promise.resolve().then(work).catch((reason) => { pending = null; throw reason })
    return pending
  }
}
