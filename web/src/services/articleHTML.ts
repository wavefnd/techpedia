import type { Directive } from 'vue'
import { restoreInitialArticleNodes } from './initialPage'
import { setTrustedHTML } from './trustedHTML'

const galleryBindings = new WeakMap<HTMLElement, { cancelled: boolean; dispose?: () => void }>()
function clearGalleries(element: HTMLElement) {
  const binding = galleryBindings.get(element)
  if (binding) { binding.cancelled = true; binding.dispose?.(); galleryBindings.delete(element) }
}
function enhanceGalleries(element: HTMLElement) {
  if (!element.querySelector('.wiki-image-gallery')) return
  const binding: { cancelled: boolean; dispose?: () => void } = { cancelled: false }
  galleryBindings.set(element, binding)
  void import('./imageGallery').then(({ bindImageGalleries }) => {
    if (!binding.cancelled && element.isConnected) binding.dispose = bindImageGalleries(element)
  }).catch(() => { /* Native horizontal scrolling still works without enhancement. */ })
}

export const articleHTMLDirective: Directive<HTMLElement, string> = {
  mounted(element, binding) {
    if (!restoreInitialArticleNodes(element)) setTrustedHTML(element, binding.value)
    enhanceGalleries(element)
  },
  updated(element, binding) {
    if (binding.value !== binding.oldValue) {
      clearGalleries(element)
      setTrustedHTML(element, binding.value)
      enhanceGalleries(element)
    }
  },
  beforeUnmount: clearGalleries,
}
