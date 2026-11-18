<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Minus, Plus, X } from '@lucide/vue'

interface LightboxImage {
  src: string
  alt: string
  caption: string
  sourceHref: string
  sourceHost: string
}

const image = ref<LightboxImage | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)
const imageViewport = ref<HTMLDivElement | null>(null)
const zoom = ref(1)
let returnFocus: HTMLElement | null = null
let previousOverflow = ''
const isKorean = computed(() => document.documentElement.lang.toLowerCase().startsWith('ko'))
const closeLabel = computed(() => isKorean.value ? '닫기' : 'Close')
const zoomInLabel = computed(() => isKorean.value ? '확대' : 'Zoom in')
const zoomOutLabel = computed(() => isKorean.value ? '축소' : 'Zoom out')
const resetZoomLabel = computed(() => isKorean.value ? '원래 배율' : 'Reset zoom')
const zoomControlsLabel = computed(() => isKorean.value ? '이미지 배율 조절' : 'Image zoom controls')
const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)
const canvasStyle = computed(() => ({
  width: `${zoom.value * 100}%`,
  height: `${zoom.value * 100}%`,
  '--image-fit': `${100 / zoom.value}%`,
  '--image-zoom': String(zoom.value),
}))

function openImage(button: HTMLElement) {
  const element = button.querySelector<HTMLImageElement>('img')
  if (!element) return
  const figure = button.closest('wiki-image')
  const source = figure?.querySelector<HTMLAnchorElement>('.wiki-image-source a')
  const src = element.dataset.largeSrc || element.currentSrc || element.src
  if (!src) return
  returnFocus = button
  zoom.value = 1
  image.value = {
    src,
    alt: element.alt,
    caption: figure?.querySelector<HTMLElement>('.wiki-image-caption')?.textContent?.trim() || '',
    sourceHref: source?.href || '',
    sourceHost: source?.textContent?.trim() || '',
  }
}

function closeImage() {
  image.value = null
  nextTick(() => returnFocus?.focus())
}

async function setZoom(value: number, clientX?: number, clientY?: number) {
  const viewport = imageViewport.value
  const nextZoom = Math.min(4, Math.max(1, Math.round(value * 2) / 2))
  if (!viewport || nextZoom === zoom.value) return

  const bounds = viewport.getBoundingClientRect()
  const anchorX = clientX === undefined ? viewport.clientWidth / 2 : clientX - bounds.left
  const anchorY = clientY === undefined ? viewport.clientHeight / 2 : clientY - bounds.top
  const ratioX = (viewport.scrollLeft + anchorX) / Math.max(1, viewport.scrollWidth)
  const ratioY = (viewport.scrollTop + anchorY) / Math.max(1, viewport.scrollHeight)

  zoom.value = nextZoom
  await nextTick()
  viewport.scrollLeft = ratioX * viewport.scrollWidth - anchorX
  viewport.scrollTop = ratioY * viewport.scrollHeight - anchorY
}

function toggleImageZoom(event: MouseEvent) {
  setZoom(zoom.value === 1 ? 2 : 1, event.clientX, event.clientY)
}

function handleDocumentClick(event: MouseEvent) {
  const target = event.target
  if (!(target instanceof Element)) return
  const button = target.closest<HTMLElement>('.techmark-content .wiki-image-open')
  if (!button) return
  event.preventDefault()
  openImage(button)
}

function handleKeydown(event: KeyboardEvent) {
  if (!image.value) return
  if (event.key === 'Escape') {
    closeImage()
    return
  }
  if (event.ctrlKey || event.metaKey || event.altKey) return
  if (event.key === '+' || event.key === '=') {
    event.preventDefault()
    setZoom(zoom.value + 0.5)
  }
  if (event.key === '-') {
    event.preventDefault()
    setZoom(zoom.value - 0.5)
  }
  if (event.key === '0') {
    event.preventDefault()
    setZoom(1)
  }
}

watch(image, async (value) => {
  if (value) {
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    closeButton.value?.focus()
    return
  }
  document.body.style.overflow = previousOverflow
})

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  document.addEventListener('keydown', handleKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = previousOverflow
})
</script>

<template>
  <Teleport to="body">
    <div v-if="image" class="image-lightbox-backdrop" @mousedown.self="closeImage">
      <section class="image-lightbox" role="dialog" aria-modal="true" :aria-label="image.alt || 'Image preview'" @mousedown.self="closeImage">
        <header @mousedown.self="closeImage">
          <div class="image-lightbox-tools" role="group" :aria-label="zoomControlsLabel">
            <button type="button" :aria-label="zoomOutLabel" :title="zoomOutLabel" :disabled="zoom === 1" @click="setZoom(zoom - 0.5)"><Minus :size="19" /></button>
            <button class="image-lightbox-zoom" type="button" :aria-label="resetZoomLabel" :title="resetZoomLabel" :disabled="zoom === 1" @click="setZoom(1)">{{ zoomPercent }}</button>
            <button type="button" :aria-label="zoomInLabel" :title="zoomInLabel" :disabled="zoom === 4" @click="setZoom(zoom + 0.5)"><Plus :size="19" /></button>
          </div>
          <button ref="closeButton" type="button" :aria-label="closeLabel" :title="closeLabel" @click="closeImage"><X :size="21" /></button>
        </header>
        <figure @mousedown.self="closeImage">
          <div ref="imageViewport" class="image-lightbox-viewport">
            <div class="image-lightbox-canvas" :class="{ 'is-zoomed': zoom > 1 }" :style="canvasStyle">
              <img :src="image.src" :alt="image.alt" decoding="async" draggable="false" @click="toggleImageZoom">
            </div>
          </div>
          <figcaption v-if="image.caption || image.sourceHref">
            <span v-if="image.caption">{{ image.caption }}</span>
            <a v-if="image.sourceHref" :href="image.sourceHref" target="_blank" rel="nofollow noopener noreferrer" referrerpolicy="no-referrer">from {{ image.sourceHost }}</a>
          </figcaption>
        </figure>
      </section>
    </div>
  </Teleport>
</template>
