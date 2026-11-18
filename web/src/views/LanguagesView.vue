<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { proposeLanguageEdition, searchLanguageCatalog } from '../services/api'
import { languageEditions, loadLanguageEditions } from '../services/editions'
import { hasPermission, isLoggedIn } from '../services/auth'
import { languageDisplayName, useWikiI18n } from '../i18n'
import type { LanguageCatalogEntry } from '../types/api'

const { locale, uiLocale } = useWikiI18n()
const selectedTag = ref('')
const proposalError = ref('')
const proposalBusy = ref(false)
const catalogResults = ref<LanguageCatalogEntry[]>([])
let catalogRequest: AbortController | null = null
let catalogTimer: ReturnType<typeof setTimeout> | null = null

const activeEditions = computed(() => languageEditions.value.filter((value) => value.status === 'active'))
const pendingEditions = computed(() => languageEditions.value.filter((value) => value.status !== 'active'))

function canonicalLanguageCode(value: string): string | null {
  const normalized = value.trim().toLowerCase()
  return /^[a-z]{2,3}$/.test(normalized) && normalized !== 'und' ? normalized : null
}

function languageName(tag: string) {
  return languageDisplayName(tag, locale.value)
}

async function loadCatalog(query: string) {
  catalogRequest?.abort()
  catalogRequest = new AbortController()
  try { catalogResults.value = await searchLanguageCatalog(query, catalogRequest.signal) } catch (reason) {
    if (!(reason instanceof DOMException && reason.name === 'AbortError')) catalogResults.value = []
  }
}

watch(selectedTag, (value) => {
  if (catalogTimer) clearTimeout(catalogTimer)
  catalogTimer = setTimeout(() => void loadCatalog(value.trim()), 150)
})

async function propose() {
  const tag = canonicalLanguageCode(selectedTag.value)
  proposalError.value = ''
  if (!tag) {
    proposalError.value = uiLocale.value === 'ko' ? '문자·지역 구분이 없는 언어 코드만 입력하세요.' : 'Enter one language code without a script or region.'
    return
  }
  proposalBusy.value = true
  try {
    await proposeLanguageEdition(tag)
    selectedTag.value = ''
    await loadLanguageEditions(true)
  } catch (reason) {
    proposalError.value = reason instanceof Error ? reason.message : 'Unable to add the language edition.'
  } finally { proposalBusy.value = false }
}

onMounted(async () => {
  await loadLanguageEditions()
  await loadCatalog('')
})
</script>

<template>
  <div class="languages-view">
    <header>
      <p>{{ uiLocale === 'ko' ? 'TechPedia 다국어' : 'TechPedia languages' }}</p>
      <h1>{{ uiLocale === 'ko' ? '언어판' : 'Language editions' }}</h1>
      <span>{{ uiLocale === 'ko' ? '활성 언어판을 선택하거나 새 언어판을 제안할 수 있습니다.' : 'Choose an active edition or propose a new one.' }}</span>
    </header>

    <section>
      <h2>{{ uiLocale === 'ko' ? '활성 언어판' : 'Active editions' }}</h2>
      <div class="edition-list">
        <RouterLink v-for="edition in activeEditions" :key="edition.tag" class="edition-row" :to="edition.main_page_url || '/'">
          <b :lang="edition.tag" :dir="edition.direction">{{ edition.native_name }}</b>
          <span>{{ languageName(edition.tag) }}</span><code>{{ edition.tag }}</code>
        </RouterLink>
      </div>
    </section>

    <section v-if="pendingEditions.length">
      <h2>{{ uiLocale === 'ko' ? '제안된 언어' : 'Proposed editions' }}</h2>
      <div class="edition-list pending-list">
        <div v-for="edition in pendingEditions" :key="edition.tag" class="edition-row">
          <b :lang="edition.tag" :dir="edition.direction">{{ edition.native_name }}</b>
          <span>{{ languageName(edition.tag) }}</span><code>{{ edition.tag }}</code>
        </div>
      </div>
      <p class="edition-empty">{{ uiLocale === 'ko' ? '메인 페이지가 개설될 때까지 문서를 만들 수 없습니다.' : 'Articles open after an administrator creates the main page.' }}</p>
    </section>

    <section class="edition-proposal">
      <h2>{{ uiLocale === 'ko' ? '언어 추가' : 'Add a language' }}</h2>
      <template v-if="hasPermission('edit')">
        <form @submit.prevent="propose">
          <label for="edition-tag">{{ uiLocale === 'ko' ? '언어 태그' : 'Language tag' }}</label>
          <input id="edition-tag" v-model="selectedTag" list="available-language-codes" placeholder="fr, ar, zh, sh" required>
          <datalist id="available-language-codes"><option v-for="language in catalogResults" :key="language.tag" :value="language.tag">{{ language.name }} · {{ language.script_name }}</option></datalist>
          <small>{{ uiLocale === 'ko' ? '문자·지역 하위 태그 없이 하나의 언어 코드만 제안합니다. 운영 여부는 관리자가 검토합니다.' : 'Propose one language code without script or region subtags. Administrators review every proposal.' }}</small>
          <button type="submit" :disabled="proposalBusy">{{ proposalBusy ? (uiLocale === 'ko' ? '추가 중' : 'Adding') : (uiLocale === 'ko' ? '언어판 제안' : 'Propose edition') }}</button>
        </form>
        <p v-if="proposalError" class="form-error">{{ proposalError }}</p>
      </template>
      <p v-else>{{ uiLocale === 'ko' ? '언어판 제안은 편집자 계정이 필요합니다.' : 'An editor account is required to propose an edition.' }} <RouterLink v-if="!isLoggedIn" to="/login">{{ uiLocale === 'ko' ? '로그인' : 'Log in' }}</RouterLink></p>
    </section>
  </div>
</template>
