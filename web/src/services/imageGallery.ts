// The DOM owns the current slide; no framework or image duplication is needed.
// Geometry-based offsets avoid browser-specific RTL scrollLeft conventions.
export function galleryIndex(centers: number[], center: number): number {
  let best = 0
  for (let i = 1; i < centers.length; i++) if (Math.abs(centers[i]! - center) < Math.abs(centers[best]! - center)) best = i
  return best
}

export function nextGalleryIndex(index: number, step: number, count: number) {
  return count ? ((index + step) % count + count) % count : 0
}

export function bindImageGalleries(container: HTMLElement): () => void {
  const frames = new Map<HTMLElement, number>()
  const galleries = Array.from(container.querySelectorAll<HTMLElement>('.wiki-image-gallery'))
  let disposed = false
  const stages = (gallery: HTMLElement) => Array.from(gallery.querySelectorAll<HTMLButtonElement>(':scope > .wiki-image-stage'))
  function current(gallery: HTMLElement, slides: HTMLElement[]) {
    const bounds = gallery.getBoundingClientRect()
    return galleryIndex(slides.map((slide) => { const rect = slide.getBoundingClientRect(); return rect.left + rect.width / 2 }), bounds.left + bounds.width / 2)
  }
  function update(gallery: HTMLElement) {
    if (disposed || !gallery.isConnected) return
    const slides = stages(gallery), index = current(gallery, slides)
    const output = gallery.parentElement?.querySelector<HTMLOutputElement>('.wiki-image-position')
    const position = `${index + 1} / ${slides.length}`
    if (output && output.textContent !== position) output.textContent = position
    slides.forEach((slide, i) => { slide.tabIndex = i === index ? 0 : -1 })
  }
  function move(gallery: HTMLElement, step: number, absolute?: number) {
    const slides = stages(gallery)
    const index = absolute ?? nextGalleryIndex(current(gallery, slides), step, slides.length)
    const slide = slides[index]
    if (!slide) return
    const left = slide.getBoundingClientRect().left - gallery.getBoundingClientRect().left
    // Instant transitions keep rapid clicks deterministic and respect reduced
    // motion without introducing timers or keeping an animation loop alive.
    gallery.scrollBy({ left, behavior: 'instant' })
    update(gallery)
  }
  function findGallery(event: Event) {
    const target = event.target
    if (!(target instanceof Element)) return null
    const figure = target.closest('wiki-image figure')
    if (!figure || !container.contains(figure)) return null
    return figure.querySelector<HTMLElement>('.wiki-image-gallery')
  }
  function click(event: MouseEvent) {
    if (!(event.target instanceof Element)) return
    const control = event.target.closest('.wiki-image-previous, .wiki-image-next')
    const gallery = control ? findGallery(event) : null
    if (!control || !gallery) return
    event.preventDefault()
    move(gallery, control.classList.contains('wiki-image-next') ? 1 : -1)
  }
  function keyboard(event: KeyboardEvent) {
    if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    const gallery = findGallery(event)
    if (!gallery) return
    event.preventDefault()
    const absolute = event.key === 'Home' ? 0 : event.key === 'End' ? stages(gallery).length - 1 : undefined
    move(gallery, event.key === 'ArrowRight' ? 1 : -1, absolute)
    if (event.target instanceof Element && event.target.classList.contains('wiki-image-stage')) stages(gallery)[current(gallery, stages(gallery))]?.focus({ preventScroll: true })
  }
  function scroll(event: Event) {
    const gallery = event.target
    if (!(gallery instanceof HTMLElement) || !gallery.classList.contains('wiki-image-gallery') || frames.has(gallery)) return
    frames.set(gallery, window.requestAnimationFrame(() => { frames.delete(gallery); update(gallery) }))
  }
  galleries.forEach(update)
  const resize = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver((entries) => {
    for (const entry of entries) update(entry.target as HTMLElement)
  })
  galleries.forEach((gallery) => resize?.observe(gallery))
  container.addEventListener('click', click)
  container.addEventListener('keydown', keyboard)
  container.addEventListener('scroll', scroll, true)
  return () => {
    disposed = true
    resize?.disconnect()
    container.removeEventListener('click', click)
    container.removeEventListener('keydown', keyboard)
    container.removeEventListener('scroll', scroll, true)
    frames.forEach((id) => window.cancelAnimationFrame(id)); frames.clear()
  }
}
