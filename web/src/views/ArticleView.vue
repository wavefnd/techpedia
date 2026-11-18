<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onUnmounted, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ArticleLength from '../components/ArticleLength.vue'
import LanguageSwitch from '../components/LanguageSwitch.vue'
import MobileArticleReader from '../components/MobileArticleReader.vue'
import { useArticle } from '../composables/useArticle'
import { useMobileViewer } from '../composables/useMobileViewer'
import { hasPermission, isLoggedIn } from '../services/auth'
import { cancelMissingWikiLinks, markMissingWikiLinks } from '../services/wikiLinks'
import { documentURL, useWikiI18n } from '../i18n'
import { useSidebarToc } from '../composables/useSidebarToc'
import { revealCurrentHash } from '../services/sections'
import { articleHTMLDirective } from '../services/articleHTML'
import { collectArticleToc } from '../services/toc'

const vArticleHtml = articleHTMLDirective
const BacklinksPanel = defineAsyncComponent(() => import('../components/BacklinksPanel.vue'))
const DiscussionPanel = defineAsyncComponent(() => import('../components/DiscussionPanel.vue'))
const RecentChangesPanel = defineAsyncComponent(() => import('../components/RecentChangesPanel.vue'))

const route = useRoute()
const router = useRouter()
const { locale, uiLocale, mainPageName, t, wikiURL } = useWikiI18n()
const name = computed(() => {
  const raw = route.params.name
  return (Array.isArray(raw) ? raw.join('/') : raw).replaceAll('_', ' ')
})
const { article, loading, missing, inactiveEdition, error, reload } = useArticle(locale, name)
const mobileViewer = useMobileViewer()
const discussionMode = computed(() => !mobileViewer.value && route.query.tab === 'discussion')
const recentChangesPage = computed(() => Boolean(article.value?.html.includes('wiki-special-recent')))
const articleContent = ref<HTMLElement | null>(null)
const sidebarToc = useSidebarToc()
onUnmounted(() => { sidebarToc.value = [] })

const formattedDate = computed(() => {
  if (!article.value?.updated_at) return ''
  return new Intl.DateTimeFormat(article.value.locale, {
    year: 'numeric', month: 'long', day: 'numeric',
  }).format(new Date(article.value.updated_at))
})

const editorURL = computed(() => documentURL('edit', locale.value, name.value))
const canEdit = computed(() => hasPermission('edit') && (article.value?.edit_policy !== 'admins' || hasPermission('admin')))
const historyURL = computed(() => documentURL('history', locale.value, name.value))
const loginTarget = computed(() => ({
  path: '/login',
  query: { redirect: editorURL.value },
}))
const signupTarget = computed(() => ({
  path: '/signup',
  query: { redirect: editorURL.value },
}))

watch([() => article.value?.html, discussionMode, mobileViewer, locale], async ([, discussion, mobile], _, onCleanup) => {
  let active = true
  let container: HTMLElement | null = null
  onCleanup(() => { active = false; cancelMissingWikiLinks(container) })
  sidebarToc.value = []
  if (discussion || mobile) return
  await nextTick()
  container = articleContent.value
  if (!active || !container?.isConnected) return
  sidebarToc.value = collectArticleToc(container, t('section'))
  void markMissingWikiLinks(container, locale.value)
  revealCurrentHash(container)
}, { immediate: true })

watch(() => route.hash, async () => {
  await nextTick()
  if (!mobileViewer.value && !discussionMode.value) revealCurrentHash(articleContent.value)
})

watch(() => article.value?.url, (url) => {
  if (url && route.path !== url) void router.replace({ path: url, query: route.query, hash: route.hash })
})

const missingCopy = computed(() => uiLocale.value === 'en'
  ? {
      eyebrow: 'Missing page',
      title: name.value,
      description: 'This page has not been written yet.',
      action: 'Create this page',
      login: 'Log in to create this page',
      signup: 'Create an account and write',
      permission: 'Editing permission is required to create this page.',
    }
  : {
      eyebrow: '없는 문서',
      title: name.value,
      description: '아직 작성되지 않은 문서입니다. 새 문서를 만들겠습니까?',
      action: '문서 만들기',
      login: '로그인 후 문서 만들기',
      signup: '회원가입 후 작성하기',
      permission: '이 문서를 만들려면 편집 권한이 필요합니다.',
    })

const missingDescription = computed(() => {
  if (!mobileViewer.value) return missingCopy.value.description
  return uiLocale.value === 'ko'
    ? '아직 작성되지 않은 문서입니다. 모바일에서는 읽기만 제공하며 문서 생성은 데스크톱에서 할 수 있습니다.'
    : 'This page has not been written yet. Mobile is read-only; create the page from a desktop browser.'
})

const inactiveCopy = computed(() => uiLocale.value === 'ko'
  ? {
      eyebrow: '비활성 언어판', title: `${locale.value} 언어판`,
      description: '이 언어판은 아직 메인 페이지가 없어 문서를 만들거나 수정할 수 없습니다. 관리자에게 메인 페이지 생성을 요청하세요.',
      action: '언어판 목록 보기', admin: '언어판 관리',
    }
  : {
      eyebrow: 'Inactive language edition', title: `${locale.value} edition`,
      description: 'This edition has no main page yet. Ask an administrator to create the main page before creating or editing articles.',
      action: 'View language editions', admin: 'Manage editions',
    })

function setMeta(name: string, content: string) {
  let element = document.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!element) {
    element = document.createElement('meta')
    element.name = name
    document.head.append(element)
  }
  element.content = content
}

function setCanonical(href: string) {
  let element = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!element) {
    element = document.createElement('link')
    element.rel = 'canonical'
    document.head.append(element)
  }
  element.href = href
}

watchEffect(() => {
  document.documentElement.lang = locale.value
  if (article.value) {
    document.title = `${article.value.name} - TechPedia`
    setMeta('description', article.value.meta_description || article.value.description)
    setMeta('robots', 'index,follow,max-image-preview:large,max-snippet:-1,max-video-preview:-1')
    setCanonical(new URL(article.value.url, window.location.origin).href)
    return
  }
  if (missing.value) {
    document.title = uiLocale.value === 'ko' ? `${name.value} - 문서 없음 - TechPedia` : `${name.value} - Page not found - TechPedia`
    setMeta('description', missingDescription.value)
    setMeta('robots', 'noindex,follow')
    setCanonical(window.location.href)
  }
  if (inactiveEdition.value) {
    document.title = `${inactiveCopy.value.title} - TechPedia`
    setMeta('description', inactiveCopy.value.description)
    setMeta('robots', 'noindex,follow')
    setCanonical(window.location.href)
  }
})
</script>

<template>
  <div class="article-view" :class="{ 'mobile-viewer-page': mobileViewer }">
    <div v-if="loading" class="page-state">{{ uiLocale === 'ko' ? '문서를 불러오는 중입니다.' : 'Loading article.' }}</div>
    <section v-else-if="inactiveEdition" class="missing-document" aria-labelledby="inactive-edition-title">
      <p>{{ inactiveCopy.eyebrow }}</p>
      <h1 id="inactive-edition-title">{{ inactiveCopy.title }}</h1>
      <p>{{ inactiveCopy.description }}</p>
      <div class="missing-actions">
        <RouterLink class="primary-action" :to="{ path: '/languages', query: { lang: locale } }">{{ inactiveCopy.action }}</RouterLink>
        <RouterLink v-if="!mobileViewer && hasPermission('admin')" :to="{ path: '/admin/languages', query: { lang: locale } }">{{ inactiveCopy.admin }}</RouterLink>
      </div>
    </section>
    <section v-else-if="missing" class="missing-document" aria-labelledby="missing-title">
      <p>{{ missingCopy.eyebrow }}</p>
      <h1 id="missing-title">{{ missingCopy.title }}</h1>
      <p>{{ missingDescription }}</p>
      <div class="missing-actions">
        <RouterLink v-if="mobileViewer" class="primary-action" :to="wikiURL(mainPageName)">
          {{ uiLocale === 'ko' ? '대문으로 돌아가기' : 'Back to Main Page' }}
        </RouterLink>
        <RouterLink v-else-if="hasPermission('edit')" class="primary-action" :to="editorURL">
          {{ missingCopy.action }}
        </RouterLink>
        <RouterLink v-else-if="!isLoggedIn" class="primary-action" :to="loginTarget">
          {{ missingCopy.login }}
        </RouterLink>
        <span v-else>{{ missingCopy.permission }}</span>
        <RouterLink v-if="!isLoggedIn" :to="signupTarget">{{ missingCopy.signup }}</RouterLink>
        <RouterLink v-if="!mobileViewer" :to="wikiURL(mainPageName)">
          {{ uiLocale === 'ko' ? '대문으로 돌아가기' : 'Back to Main Page' }}
        </RouterLink>
      </div>
    </section>
    <div v-else-if="error" class="page-state error">
      <strong>{{ uiLocale === 'ko' ? '문서를 불러오지 못했습니다.' : 'Unable to load the article.' }}</strong>
      <span>{{ error }}</span>
      <button type="button" @click="reload">{{ uiLocale === 'ko' ? '다시 시도' : 'Try again' }}</button>
    </div>
    <template v-else-if="article">
      <MobileArticleReader v-if="mobileViewer" :article="article" />
      <template v-else>
      <header class="article-header">
        <div>
          <h1>{{ article.name }}</h1>
          <p v-if="article.description">{{ article.description }}</p>
        </div>
        <LanguageSwitch :current="article.locale" :name="article.name" :languages="article.languages" @techdata-changed="reload" />
      </header>

      <div class="article-toolbar">
        <nav :aria-label="t('document')"><a class="active" href="#article">{{ t('document') }}</a></nav>
        <nav :aria-label="t('document')">
          <RouterLink :class="{ active: !discussionMode }" :to="article.url">{{ t('read') }}</RouterLink>
          <RouterLink :class="{ active: discussionMode }" :to="{ path: article.url, query: { tab: 'discussion' } }">{{ t('discussion') }}</RouterLink>
          <RouterLink v-if="canEdit" :to="editorURL">{{ t('edit') }}</RouterLink>
          <RouterLink :to="historyURL">{{ t('history') }}</RouterLink>
        </nav>
      </div>

      <DiscussionPanel v-if="discussionMode" :page-name="article.name" :page-title="article.name" :page-url="article.url" :locale="article.locale" />
      <div v-show="!discussionMode" class="article-grid">
        <article id="article" class="article-body">
          <RecentChangesPanel v-if="recentChangesPage" :locale="article.locale" />
          <div ref="articleContent" v-article-html="article.html" class="techmark-content"></div>
          <footer class="article-footer">
            <span v-if="formattedDate">{{ t('updated') }}: {{ formattedDate }}</span>
            <ArticleLength :article="article" />
            <span>{{ t('contentLicense') }}</span>
          </footer>
          <BacklinksPanel :locale="article.locale" :name="article.name" />
        </article>
      </div>
      </template>
    </template>
  </div>
</template>
