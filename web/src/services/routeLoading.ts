// Start independent chunks together; rejection still prevents route activation.
export function parallelView<T>(loadView: () => Promise<T>, prepareUI: () => Promise<unknown>): () => Promise<T> {
  return async () => {
    const [view] = await Promise.all([loadView(), prepareUI()])
    return view
  }
}
