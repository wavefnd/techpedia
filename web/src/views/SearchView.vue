<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, watchEffect } from 'vue'
import { FilePlus2, Search } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import { useWikiI18n } from '../i18n'
import { searchArticles } from '../services/api'
import type { SearchResponse } from '../types/api'

const route = useRoute()
const router = useRouter()
const { locale, t } = useWikiI18n()
const input = ref('')
const response = ref<SearchResponse | null>(null)
const loading = ref(false)
const searched = ref(false)
const error = ref(false)
let controller: AbortController | null = null

const routeQuery = computed(() => {
  const value = route.query.q
  const query = Array.isArray(value) ? value[0] : value
  return typeof query === 'string' ? query.trim() : ''
})

async function load() {
  controller?.abort()
  input.value = routeQuery.value
  response.value = null
  error.value = false
  searched.value = routeQuery.value !== ''
  if (!searched.value) {
    loading.value = false
    return
  }

  const activeController = new AbortController()
  controller = activeController
  loading.value = true
  try {
    response.value = await searchArticles(locale.value, routeQuery.value, activeController.signal, 50)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = true
  } finally {
    if (controller === activeController) loading.value = false
  }
}

function submit() {
  const query = input.value.trim()
  void router.push({ name: 'search', params: { locale: locale.value }, query: query ? { q: query } : {} })
}

function formatCount(value: number) {
  return new Intl.NumberFormat(locale.value).format(value)
}

watch([locale, routeQuery], load, { immediate: true })
watchEffect(() => { document.title = `${t('search')} - TechPedia` })
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <div class="search-view">
    <header class="search-page-header">
      <h1>{{ t('search') }}</h1>
      <form class="search-page-form" role="search" @submit.prevent="submit">
        <label class="sr-only" for="search-page-input">{{ t('searchLabel') }}</label>
        <input id="search-page-input" v-model="input" type="search" autocomplete="off" :placeholder="t('searchPlaceholder')">
        <button type="submit"><Search :size="18" /><span>{{ t('search') }}</span></button>
      </form>
    </header>

    <div v-if="loading" class="search-page-loading" role="status" :aria-label="t('search')"></div>
    <p v-else-if="error" class="page-state error">{{ t('noResults') }}</p>
    <template v-else-if="response">
      <p class="search-page-summary" aria-live="polite">{{ t('search') }} · {{ formatCount(response.results.length) }}</p>
      <ol v-if="response.results.length" class="search-result-list">
        <li v-for="result in response.results" :key="result.url">
          <RouterLink :to="result.url">
            <strong>{{ result.name }}</strong>
            <span v-if="result.description">{{ result.description }}</span>
            <small dir="ltr">{{ result.url }}</small>
          </RouterLink>
        </li>
      </ol>
      <p v-else class="search-page-empty">{{ t('noResults') }}</p>

      <RouterLink v-if="response.create" class="search-create-link" :to="response.create.url">
        <FilePlus2 :size="20" />
        <span><strong>{{ response.create.name }}</strong><small>{{ t('edit') }}</small></span>
      </RouterLink>
    </template>
  </div>
</template>
