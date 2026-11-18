<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, ref, shallowRef, watch } from 'vue'
import { History } from '@lucide/vue'
import { useRoute } from 'vue-router'
import ArticleToc from './ArticleToc.vue'
import ArticleLength from './ArticleLength.vue'
import LanguageSwitch from './LanguageSwitch.vue'
import { documentURL, useWikiI18n } from '../i18n'
import { cancelMissingWikiLinks, markMissingWikiLinks } from '../services/wikiLinks'
import { revealCurrentHash } from '../services/sections'
import type { ArticleResponse } from '../types/api'
import { articleHTMLDirective } from '../services/articleHTML'
import { collectArticleToc } from '../services/toc'
import type { TocNode } from '../types/toc'

const BacklinksPanel = defineAsyncComponent(() => import('./BacklinksPanel.vue'))
const vArticleHtml = articleHTMLDirective

const props = defineProps<{ article: ArticleResponse }>()
const route = useRoute()
const { t } = useWikiI18n()
const tocCollapsed = ref(true)
const articleContent = ref<HTMLElement | null>(null)
const sections = shallowRef<TocNode[]>([])

const historyURL = computed(() => documentURL('history', props.article.locale, props.article.name))
const formattedDate = computed(() => {
  if (!props.article.updated_at) return ''
  return new Intl.DateTimeFormat(props.article.locale, {
    year: 'numeric', month: 'short', day: 'numeric',
  }).format(new Date(props.article.updated_at))
})

watch([() => props.article.html, () => props.article.locale], async (_, __, onCleanup) => {
  let active = true
  let container: HTMLElement | null = null
  onCleanup(() => { active = false; cancelMissingWikiLinks(container) })
  sections.value = []
  await nextTick()
  container = articleContent.value
  if (!active || !container?.isConnected) return
  sections.value = collectArticleToc(container, t('section'))
  void markMissingWikiLinks(container, props.article.locale)
  revealCurrentHash(container)
}, { immediate: true })

watch(() => route.hash, async () => {
  await nextTick()
  revealCurrentHash(articleContent.value)
})
</script>

<template>
  <section class="mobile-article-reader">
    <header class="mobile-reader-header">
      <div>
        <p>{{ t('document') }}</p>
        <h1>{{ article.name }}</h1>
        <span v-if="article.description">{{ article.description }}</span>
      </div>
      <LanguageSwitch :current="article.locale" :name="article.name" :languages="article.languages" :allow-create="false" />
    </header>

    <div class="mobile-reader-meta">
      <span v-if="formattedDate">{{ t('updated') }} {{ formattedDate }}</span>
      <RouterLink :to="historyURL"><History :size="15" />{{ t('history') }}</RouterLink>
    </div>

    <div class="mobile-reader-content" :class="{ 'toc-collapsed': tocCollapsed }">
      <ArticleToc :sections="sections" @collapse="tocCollapsed = $event" />
      <article id="article" class="article-body">
        <div ref="articleContent" v-article-html="article.html" class="techmark-content"></div>
        <footer class="article-footer"><ArticleLength :article="article" /><span>{{ t('contentLicense') }}</span></footer>
        <BacklinksPanel :locale="article.locale" :name="article.name" />
      </article>
    </div>
  </section>
</template>
