import { onBeforeUnmount, onMounted, readonly, ref } from 'vue'

export const mobileViewerQuery = '(max-width: 680px)'

export function isMobileViewerViewport(): boolean {
  return typeof window !== 'undefined' && window.matchMedia(mobileViewerQuery).matches
}

export function useMobileViewer() {
  const mobile = ref(isMobileViewerViewport())
  let media: MediaQueryList | null = null

  function update(event?: MediaQueryListEvent) {
    mobile.value = event ? event.matches : Boolean(media?.matches)
  }

  onMounted(() => {
    media = window.matchMedia(mobileViewerQuery)
    update()
    media.addEventListener('change', update)
  })
  onBeforeUnmount(() => media?.removeEventListener('change', update))

  return readonly(mobile)
}
