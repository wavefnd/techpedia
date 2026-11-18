<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getBacklinks } from '../services/api'
import { useWikiI18n } from '../i18n'
import type { LinkPage } from '../types/api'

const props = defineProps<{ locale: string; name: string }>()
const { t, uiLocale } = useWikiI18n()
const pages = ref<LinkPage[]>([])
const loading = ref(false)
const loaded = ref(false)
const error = ref('')

const title = computed(() => t('backlinks') === 'backlinks'
  ? (uiLocale.value === 'ko' ? '이 문서를 가리키는 문서' : 'What links here')
  : t('backlinks'))
const empty = computed(() => t('noBacklinks') === 'noBacklinks'
  ? (uiLocale.value === 'ko' ? '이 문서를 가리키는 다른 문서가 없습니다.' : 'No other pages link here.')
  : t('noBacklinks'))

async function load() {
  if (loaded.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    pages.value = (await getBacklinks(props.locale, props.name)).pages
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : (uiLocale.value === 'ko' ? '역링크를 불러오지 못했습니다.' : 'Unable to load backlinks.')
  } finally {
    loading.value = false
    loaded.value = true
  }
}

watch(() => `${props.locale}:${props.name}`, () => {
  pages.value = []
  loaded.value = false
  error.value = ''
})
</script>

<template>
  <details class="article-backlinks" @toggle="load">
    <summary>{{ title }}<span v-if="loaded">{{ pages.length }}</span></summary>
    <p v-if="loading">{{ uiLocale === 'ko' ? '역링크를 불러오는 중입니다.' : 'Loading backlinks.' }}</p>
    <p v-else-if="error" class="article-backlinks-error">{{ error }}</p>
    <ul v-else-if="pages.length"><li v-for="page in pages" :key="page.url"><RouterLink :to="page.url">{{ page.name }}</RouterLink></li></ul>
    <p v-else-if="loaded">{{ empty }}</p>
  </details>
</template>
