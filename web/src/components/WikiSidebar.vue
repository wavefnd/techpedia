<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { FileClock, Heart, X } from '@lucide/vue'
import { useWikiI18n } from '../i18n'
import { getPublicConfig } from '../services/api'
import ArticleToc from './ArticleToc.vue'
import { useSidebarToc } from '../composables/useSidebarToc'
import { scheduleWhenIdle } from '../services/scheduling'

defineProps<{ open: boolean; supportLabel?: string }>()
const emit = defineEmits<{ close: [] }>()
const { locale, edition, t, wikiURL } = useWikiI18n()
const engineVersion = ref('')
const sidebarToc = useSidebarToc()

onMounted(() => {
  scheduleWhenIdle(async () => {
    try {
      engineVersion.value = (await getPublicConfig()).engine.version
    } catch {
      engineVersion.value = ''
    }
  })
})
</script>

<template>
  <aside class="wiki-sidebar" :class="{ open }" :aria-label="`TechPedia ${t('navigation')}`">
    <button class="sidebar-close icon-button" type="button" :title="t('close')" :aria-label="t('close')" @click="emit('close')"><X :size="20" /></button>
    <nav v-for="section in edition?.navigation || []" :key="section.title">
      <h2>{{ section.title }}</h2>
      <RouterLink v-for="item in section.items" :key="`${item.label}:${item.target}`" :to="wikiURL(item.target)">{{ item.label }}</RouterLink>
    </nav>
    <div class="sidebar-utility-links">
      <RouterLink class="sidebar-language-directory" :to="{ path: '/languages', query: { lang: locale } }"><span>{{ locale }}</span>{{ t('languages') }}</RouterLink>
      <RouterLink v-if="supportLabel" class="sidebar-support-link" to="/support"><Heart :size="15" aria-hidden="true" />{{ supportLabel }}</RouterLink>
      <RouterLink class="sidebar-changelog-link" to="/changelog"><FileClock :size="15" aria-hidden="true" />변경 기록</RouterLink>
    </div>
    <p class="sidebar-note">{{ t('sidebarNote') }}</p>
    <p v-if="engineVersion" class="sidebar-version">TechPedia Engine v{{ engineVersion }}</p>
    <div class="sidebar-article-toc"><ArticleToc :sections="sidebarToc" /></div>
  </aside>
</template>
