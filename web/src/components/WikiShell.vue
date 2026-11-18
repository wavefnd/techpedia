<script setup lang="ts">
import { computed, onMounted, ref, watch, watchEffect } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import ArticleImageLightbox from './ArticleImageLightbox.vue'
import ScrollNavigator from './ScrollNavigator.vue'
import WikiHeader from './WikiHeader.vue'
import WikiSidebar from './WikiSidebar.vue'
import { useMobileViewer } from '../composables/useMobileViewer'
import { provideSidebarToc } from '../composables/useSidebarToc'
import { canonicalLanguageTag, useWikiI18n } from '../i18n'
import { ensureLanguageEdition, findLanguageEdition } from '../services/editions'
import { scheduleWhenIdle } from '../services/scheduling'
import { loadPublicSupportSettings, publicSupportSettings } from '../services/support'

const navigationOpen = ref(false)
const route = useRoute()
const router = useRouter()
const { locale: currentLocale, t } = useWikiI18n()
const mobileViewer = useMobileViewer()
const supportLabel = computed(() => publicSupportSettings.value?.title || '')
provideSidebarToc()

onMounted(() => {
  scheduleWhenIdle(() => { void loadPublicSupportSettings() })
})
watch(currentLocale, (locale) => {
  // Server-provided messages keep reading usable when optional API requests
  // are blocked by robots.txt, an extension, or a temporary network failure.
  void ensureLanguageEdition(locale).catch(() => {})
}, { immediate: true })
watch(() => route.fullPath, () => { navigationOpen.value = false })
watch(mobileViewer, (enabled) => {
  if (!enabled) return
  if (route.name === 'article' && route.query.tab === 'discussion') {
    const query = { ...route.query }
    delete query.tab
    void router.replace({ name: 'article', params: route.params, query, hash: route.hash })
    return
  }
  if (!route.meta.mobileBlocked) return
  if (route.name === 'editor' && route.params.locale && route.params.name) {
    void router.replace({ name: 'article', params: { locale: route.params.locale, name: route.params.name } })
    return
  }
  void router.replace({ name: 'language-portal' })
})
watchEffect(() => {
  const locale = canonicalLanguageTag(route.params.locale || route.query.lang)
  const edition = findLanguageEdition(locale)
  document.documentElement.lang = locale
  document.documentElement.dir = edition?.direction || 'ltr'
  if (edition?.writing_system) document.documentElement.dataset.writingSystem = edition.writing_system
})
</script>

<template>
  <div class="app-shell">
    <WikiHeader @toggle-navigation="navigationOpen = !navigationOpen" />
    <div class="app-frame">
      <WikiSidebar :open="navigationOpen" :support-label="supportLabel" @close="navigationOpen = false" />
      <button v-if="navigationOpen" class="navigation-scrim" type="button" :aria-label="t('close')" @click="navigationOpen = false"></button>
      <main class="page-stage" @click="navigationOpen = false">
        <RouterView />
      </main>
    </div>
    <ScrollNavigator />
    <ArticleImageLightbox />
  </div>
</template>
