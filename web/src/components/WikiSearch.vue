<script setup lang="ts">
import { ref } from 'vue'
import { Search } from '@lucide/vue'
import { useRouter } from 'vue-router'
import { useWikiI18n } from '../i18n'

const props = withDefaults(defineProps<{ mobile?: boolean }>(), { mobile: false })
const router = useRouter()
const { locale, t } = useWikiI18n()
const query = ref('')

function openSearchPage() {
  const value = query.value.trim()
  if (!value) return
  void router.push({ name: 'search', params: { locale: locale.value }, query: { q: value } })
}
</script>

<template>
  <form class="global-search" :class="{ 'mobile-search': mobile }" role="search" @submit.prevent="openSearchPage">
    <label class="sr-only" :for="mobile ? 'mobile-global-search' : 'desktop-global-search'">{{ t('searchLabel') }}</label>
    <input
      :id="mobile ? 'mobile-global-search' : 'desktop-global-search'"
      v-model="query"
      type="search"
      autocomplete="off"
      :placeholder="t('searchPlaceholder')"
    >
    <button type="submit" :title="mobile ? t('search') : undefined" :aria-label="mobile ? t('search') : undefined">
      <Search v-if="mobile" :size="18" />
      <template v-else>{{ t('search') }}</template>
    </button>
  </form>
</template>
