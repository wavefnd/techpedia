<script setup lang="ts">
import { ChevronDown, ChevronUp } from '@lucide/vue'
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useWikiI18n } from '../i18n'

const route = useRoute()
const { t, uiLocale } = useWikiI18n()
const canScrollUp = ref(false)
const canScrollDown = ref(false)
let resizeObserver: ResizeObserver | undefined

function updateState() {
  const root = document.documentElement
  canScrollUp.value = window.scrollY > 240
  canScrollDown.value = window.scrollY + window.innerHeight < root.scrollHeight - 240
}

function label(key: 'scrollTop' | 'scrollBottom'): string {
  const translated = t(key)
  if (translated !== key) return translated
  if (uiLocale.value === 'ko') return key === 'scrollTop' ? '페이지 맨 위로' : '페이지 맨 아래로'
  return key === 'scrollTop' ? 'Go to top' : 'Go to bottom'
}

function scrollTo(top: number) {
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  window.scrollTo({ top, behavior: reducedMotion ? 'auto' : 'smooth' })
}

function scrollToBottom() {
  scrollTo(document.documentElement.scrollHeight)
}

watch(() => route.fullPath, async () => {
  await nextTick()
  updateState()
})

onMounted(() => {
  window.addEventListener('scroll', updateState, { passive: true })
  window.addEventListener('resize', updateState)
  resizeObserver = new ResizeObserver(updateState)
  resizeObserver.observe(document.documentElement)
  updateState()
})

onUnmounted(() => {
  window.removeEventListener('scroll', updateState)
  window.removeEventListener('resize', updateState)
  resizeObserver?.disconnect()
})
</script>

<template>
  <nav v-if="canScrollUp || canScrollDown" class="scroll-navigator" :aria-label="uiLocale === 'ko' ? '페이지 이동' : 'Page navigation'">
    <button v-if="canScrollUp" type="button" :aria-label="label('scrollTop')" :title="label('scrollTop')" @click="scrollTo(0)">
      <ChevronUp :size="20" aria-hidden="true" />
    </button>
    <button v-if="canScrollDown" type="button" :aria-label="label('scrollBottom')" :title="label('scrollBottom')" @click="scrollToBottom">
      <ChevronDown :size="20" aria-hidden="true" />
    </button>
  </nav>
</template>
