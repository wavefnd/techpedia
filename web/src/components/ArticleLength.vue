<script setup lang="ts">
import { computed } from 'vue'
import { useWikiI18n } from '../i18n'
import type { ArticleResponse } from '../types/api'

const props = defineProps<{ article: ArticleResponse }>()
const { uiLocale, t } = useWikiI18n()
const number = computed(() => new Intl.NumberFormat(props.article.locale))
const lineLabel = computed(() => t('lines') === 'lines' ? (uiLocale.value === 'ko' ? '줄' : 'lines') : t('lines'))
const characterLabel = computed(() => t('characters') === 'characters' ? (uiLocale.value === 'ko' ? '글자' : 'characters') : t('characters'))
</script>

<template>
  <span class="article-length">
    {{ number.format(article.line_count) }}{{ lineLabel }} · {{ number.format(article.character_count) }}{{ characterLabel }}
  </span>
</template>
