<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Database, Link2, Search, Trash2, X } from '@lucide/vue'
import { ApiError, createAdminRedirect, deleteAdminPage, deleteAdminRedirect, getAdminLinkReport, getAdminPages, getAdminRedirects, getTechdataItems, linkPageTechdata, techdataDuplicateResponse, unlinkPageTechdata } from '../../services/api'
import { activeLanguageEditions, loadLanguageEditions } from '../../services/editions'
import { documentURL } from '../../i18n'
import { LatestRequest } from '../../services/latestRequest'
import { account } from '../../services/auth'
import type { AdminDocumentRedirect, AdminDocumentSummary, AdminLinkReport, TechdataItemResponse } from '../../types/api'

const pages = ref<AdminDocumentSummary[]>([])
const route = useRoute()
const locale = ref(typeof route.query.locale === 'string' ? route.query.locale : '')
const query = ref(typeof route.query.q === 'string' ? route.query.q : '')
const loading = ref(false)
const deleteBusy = ref(false)
const error = ref('')
const bootstrapFailed = ref(false)
const bootstrapBusy = ref(false)
const editionsReady = ref(false)
const linkReport = ref<AdminLinkReport | null>(null)
const linkLoading = ref(false)
const newLocale = ref('')
const newName = ref('')
const redirects = ref<AdminDocumentRedirect[]>([])
const redirectLocale = ref('')
const redirectName = ref('')
const redirectTarget = ref('')
const redirectLoading = ref(false)
const redirectBusy = ref(false)
const redirectError = ref('')
const techdataPage = ref<AdminDocumentSummary | null>(null)
const techdataItems = ref<TechdataItemResponse[]>([])
const techdataQuery = ref('')
const techdataLoading = ref(false)
const techdataBusy = ref(false)
const techdataError = ref('')
const techdataDuplicateBlocked = ref(false)
const router = useRouter()
let timer = 0
let techdataTimer = 0
const pageRequests = new LatestRequest()
const redirectRequests = new LatestRequest()
const linkRequests = new LatestRequest()
const techdataRequests = new LatestRequest()
let mounted = true

async function load() {
  loading.value = true
  error.value = ''
  const selectedLocale = locale.value
  const search = query.value.trim()
  await pageRequests.run((signal) => getAdminPages(selectedLocale, search, signal), {
    success: (value) => { pages.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '문서 목록을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

watch([locale, query], () => {
  pageRequests.invalidate()
  pages.value = []
  loading.value = true
  error.value = ''
  window.clearTimeout(timer)
  timer = window.setTimeout(() => void load(), 180)
}, { flush: 'sync' })

watch(locale, () => {
  linkRequests.invalidate()
  linkReport.value = null
  linkLoading.value = false
}, { flush: 'sync' })

watch(redirectLocale, () => {
  redirectRequests.invalidate()
  redirects.value = []
  redirectLoading.value = false
}, { flush: 'sync' })

watch(techdataQuery, () => {
  techdataRequests.invalidate()
  techdataItems.value = []
  techdataError.value = ''
  techdataLoading.value = !!techdataPage.value
  techdataDuplicateBlocked.value = false
  window.clearTimeout(techdataTimer)
  techdataTimer = window.setTimeout(() => void loadTechdataItems(), 180)
}, { flush: 'sync' })

onBeforeUnmount(() => {
  mounted = false
  for (const requests of [pageRequests, redirectRequests, linkRequests, techdataRequests]) requests.dispose()
  window.clearTimeout(timer)
  window.clearTimeout(techdataTimer)
})

async function bootstrap() {
  if (bootstrapBusy.value) return
  bootstrapBusy.value = true
  bootstrapFailed.value = false
  error.value = ''
  try {
    await loadLanguageEditions()
    if (!mounted) return
    editionsReady.value = true
    newLocale.value = activeLanguageEditions.value[0]?.tag || ''
    redirectLocale.value = activeLanguageEditions.value[0]?.tag || ''
    await Promise.all([load(), loadRedirects()])
  } catch (reason) {
    if (mounted) {
      editionsReady.value = false
      bootstrapFailed.value = true
      error.value = `언어판을 불러오지 못했습니다.${reason instanceof Error ? ` ${reason.message}` : ''}`
    }
  } finally {
    if (mounted) bootstrapBusy.value = false
  }
}
onMounted(bootstrap)

async function createPage() {
  if (!editionsReady.value || !newLocale.value || !newName.value.trim()) return
  await router.push(documentURL('edit', newLocale.value, newName.value.trim()))
}

async function loadRedirects() {
  redirectLoading.value = true
  redirectError.value = ''
  const selectedLocale = redirectLocale.value
  await redirectRequests.run((signal) => getAdminRedirects(selectedLocale, signal), {
    success: (value) => { redirects.value = value },
    error: (reason) => { redirectError.value = reason instanceof Error ? reason.message : '리다이렉트 목록을 불러오지 못했습니다.' },
    settled: () => { redirectLoading.value = false },
  })
}

async function createRedirect() {
  if (redirectBusy.value || !redirectLocale.value || !redirectName.value.trim() || !redirectTarget.value.trim()) return
  redirectBusy.value = true
  redirectError.value = ''
  try {
    await createAdminRedirect(redirectLocale.value, redirectName.value.trim(), redirectTarget.value.trim())
    redirectName.value = ''
    redirectTarget.value = ''
    await loadRedirects()
  } catch (reason) {
    redirectError.value = reason instanceof ApiError && reason.status === 404
      ? '입력한 정식 문서를 이 언어판에서 찾지 못했습니다.'
      : reason instanceof ApiError && reason.status === 409
        ? '별칭이 실제 문서, 시스템 문서 또는 다른 리다이렉트에서 이미 사용 중입니다.'
        : '리다이렉트를 만들지 못했습니다.'
  } finally {
    redirectBusy.value = false
  }
}

async function removeRedirect(redirect: AdminDocumentRedirect) {
  if (redirectBusy.value) return
  if (!window.confirm(`'${redirect.name}' 리다이렉트를 삭제할까요? 정식 문서는 삭제되지 않습니다.`)) return
  redirectBusy.value = true
  redirectError.value = ''
  try {
    await deleteAdminRedirect(redirect.locale, redirect.name)
    await loadRedirects()
  } catch (reason) {
    redirectError.value = reason instanceof ApiError && reason.status === 404
      ? '이미 삭제되었거나 존재하지 않는 리다이렉트입니다.'
      : '리다이렉트를 삭제하지 못했습니다.'
  } finally {
    redirectBusy.value = false
  }
}

async function loadLinkReport() {
  linkLoading.value = true
  error.value = ''
  const selectedLocale = locale.value
  await linkRequests.run((signal) => getAdminLinkReport(selectedLocale, signal), {
    success: (value) => { linkReport.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '링크 점검 결과를 불러오지 못했습니다.' },
    settled: () => { linkLoading.value = false },
  })
}

function techdataLabel(item: TechdataItemResponse): string {
  const currentLocale = techdataPage.value?.locale || ''
  return item.labels[currentLocale] || Object.values(item.labels).find(Boolean) || item.id
}

function techdataDescription(item: TechdataItemResponse): string {
  const currentLocale = techdataPage.value?.locale || ''
  return item.descriptions[currentLocale] || Object.values(item.descriptions).find(Boolean) || ''
}

async function openTechdata(page: AdminDocumentSummary) {
  if (techdataBusy.value) return
  resetTechdataDialog()
  techdataPage.value = page
  techdataQuery.value = ''
  techdataError.value = ''
  techdataDuplicateBlocked.value = false
  window.clearTimeout(techdataTimer)
  await loadTechdataItems()
}

function closeTechdata() {
  if (techdataBusy.value) return
  resetTechdataDialog()
}

function resetTechdataDialog() {
  window.clearTimeout(techdataTimer)
  techdataRequests.invalidate()
  techdataLoading.value = false
  techdataPage.value = null
  techdataItems.value = []
  techdataError.value = ''
  techdataDuplicateBlocked.value = false
}

async function loadTechdataItems() {
  if (!techdataPage.value || techdataBusy.value) return
  techdataLoading.value = true
  techdataError.value = ''
  const search = techdataQuery.value
  await techdataRequests.run((signal) => getTechdataItems(search, signal), {
    success: (value) => { techdataItems.value = value },
    error: (reason) => { techdataError.value = reason instanceof Error ? reason.message : 'Techdata 항목을 불러오지 못했습니다.' },
    settled: () => { techdataLoading.value = false },
  })
}

async function connectTechdata(tid = '', confirmDistinct = false) {
  const page = techdataPage.value
  if (!page || techdataBusy.value) return
  window.clearTimeout(techdataTimer)
  techdataRequests.invalidate()
  techdataLoading.value = false
  techdataBusy.value = true
  techdataError.value = ''
  try {
    await linkPageTechdata(page.locale, page.name, tid, confirmDistinct)
    await load()
    resetTechdataDialog()
  } catch (reason) {
    const duplicate = techdataDuplicateResponse(reason)
    if (duplicate) {
      techdataItems.value = duplicate.candidates
      techdataDuplicateBlocked.value = true
      techdataError.value = '같은 이름의 Techdata 항목이 있습니다. 기존 항목을 확인한 뒤 연결하세요.'
    } else {
      techdataError.value = reason instanceof Error ? reason.message : 'Techdata 항목을 연결하지 못했습니다.'
    }
  } finally {
    techdataBusy.value = false
  }
}

async function unlinkTechdata() {
  const page = techdataPage.value
  if (!page?.tid || techdataBusy.value) return
  window.clearTimeout(techdataTimer)
  techdataRequests.invalidate()
  techdataLoading.value = false
  techdataBusy.value = true
  techdataError.value = ''
  try {
    await unlinkPageTechdata(page.locale, page.name)
    await load()
    resetTechdataDialog()
  } catch (reason) {
    techdataError.value = reason instanceof Error ? reason.message : 'Techdata 연결을 해제하지 못했습니다.'
  } finally {
    techdataBusy.value = false
  }
}

async function deletePage(page: AdminDocumentSummary) {
  if (!account.value?.permissions?.includes('owner')) return
  if (deleteBusy.value) return
  const reason = window.prompt(`'${page.name}' 문서를 휴지통으로 이동합니다. 삭제 사유를 입력하세요.`, '')
  if (reason === null) return
  deleteBusy.value = true
  try {
    await deleteAdminPage(page.locale, page.name, reason)
    await load()
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '문서를 삭제하지 못했습니다.'
  } finally {
    deleteBusy.value = false
  }
}
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header"><div><p>콘텐츠</p><h1>문서 관리</h1></div><span>{{ pages.length }}개 표시</span></header>
    <VaCard>
      <VaCardContent class="admin-filters">
        <VaInput v-model="query" label="문서 검색" input-aria-label="문서 검색" placeholder="이름 또는 본문 검색" clearable />
        <label>언어판<select v-model="locale"><option value="">모든 언어</option><option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option></select></label>
      </VaCardContent>
    </VaCard>
    <VaCard>
      <VaCardContent>
        <form class="admin-create-page" @submit.prevent="createPage">
          <label>언어판<select v-model="newLocale" :disabled="!editionsReady" required><option value="" disabled>언어판 선택</option><option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option></select></label>
          <VaInput v-model="newName" label="새 문서 이름" input-aria-label="새 문서 이름" placeholder="저작권, 도움말, 프로그래밍" required />
          <VaButton type="submit" :disabled="!editionsReady || !newLocale || !newName.trim()">문서 만들기</VaButton>
        </form>
      </VaCardContent>
    </VaCard>
    <VaCard class="admin-redirect-card">
      <VaCardContent>
        <header class="admin-redirect-heading">
          <span><Link2 :size="20" /></span>
          <div><small>문서 별칭</small><h2>리다이렉트</h2><p>약칭이나 다른 표기를 정식 문서 이름으로 연결합니다. 별칭에는 별도 문서나 리비전이 생기지 않습니다.</p></div>
          <b>{{ redirects.length }}개</b>
        </header>
        <form class="admin-redirect-form" @submit.prevent="createRedirect">
          <label>언어판<select v-model="redirectLocale" :disabled="redirectBusy || !editionsReady" required @change="loadRedirects"><option value="" disabled>언어판 선택</option><option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option></select></label>
          <VaInput v-model="redirectName" label="별칭" input-aria-label="별칭" placeholder="BSD" :disabled="redirectBusy" required />
          <VaInput v-model="redirectTarget" label="정식 문서 이름" input-aria-label="정식 문서 이름" placeholder="Berkeley Software Distribution" :disabled="redirectBusy" required />
          <VaButton type="submit" :loading="redirectBusy" :disabled="redirectBusy || !editionsReady || !redirectLocale || !redirectName.trim() || !redirectTarget.trim()">리다이렉트 만들기</VaButton>
        </form>
        <VaAlert v-if="redirectError" color="danger">{{ redirectError }}</VaAlert>
        <div v-if="redirectLoading" class="admin-loading"><VaProgressCircle indeterminate /> 불러오는 중입니다.</div>
        <div v-else-if="redirects.length" class="admin-redirect-list">
          <article v-for="redirect in redirects" :key="`${redirect.locale}:${redirect.name}`">
            <VaBadge :text="redirect.locale" color="secondary" />
            <RouterLink :to="redirect.url"><strong>{{ redirect.name }}</strong></RouterLink>
            <span class="admin-redirect-arrow" aria-hidden="true">→</span>
            <RouterLink v-if="redirect.target_exists && redirect.target_url" :to="redirect.target_url">{{ redirect.target_name }}</RouterLink>
            <span v-else class="admin-redirect-broken">{{ redirect.target_name || '대상 문서 없음' }}</span>
            <small>{{ redirect.created_by || '관리자' }} · {{ new Date(redirect.created_at).toLocaleString() }}</small>
            <button type="button" :disabled="redirectBusy" title="리다이렉트 삭제" @click="removeRedirect(redirect)"><Trash2 :size="15" /></button>
          </article>
        </div>
        <p v-else class="admin-empty">등록된 리다이렉트가 없습니다.</p>
      </VaCardContent>
    </VaCard>
    <VaAlert v-if="error" color="danger">{{ error }} <VaButton v-if="bootstrapFailed" :disabled="bootstrapBusy" @click="bootstrap">언어판 다시 불러오기</VaButton></VaAlert>
    <VaCard class="admin-table-card">
      <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 불러오는 중입니다.</div>
      <div v-else class="admin-table" role="table" aria-label="문서 목록">
        <div class="admin-table-row head" role="row"><span>문서</span><span>언어</span><span>리비전</span><span>수정자</span><span>작업</span></div>
        <div v-for="page in pages" :key="`${page.locale}:${page.url}`" class="admin-table-row" role="row">
          <span><b>{{ page.name }} <VaBadge v-if="page.system" text="시스템" color="info" /> <VaBadge v-if="page.edit_policy === 'admins'" text="관리자 편집" color="warning" /></b><small>{{ new Date(page.updated_at).toLocaleString() }}</small><span class="admin-document-identifiers"><span v-if="page.wid" title="변경되지 않는 위키 문서 식별자">문서 ID <code>{{ page.wid }}</code></span><span v-if="page.tid">Techdata <RouterLink :to="`/admin/techdata/${page.tid}`">{{ page.tid }}</RouterLink></span><button type="button" @click="openTechdata(page)"><Link2 :size="13" /> {{ page.tid ? '연결 관리' : 'Techdata 연결' }}</button></span></span>
          <span><VaBadge :text="page.locale" color="secondary" /></span><span>{{ page.revision }}</span><span>{{ page.author || '-' }}</span>
          <span class="admin-row-actions"><RouterLink :to="page.url">보기</RouterLink><RouterLink v-if="page.editable" :to="page.edit_url">편집</RouterLink><button v-if="page.editable && account?.permissions?.includes('owner')" type="button" :disabled="deleteBusy" title="휴지통으로 이동 (소유자 전용)" @click="deletePage(page)"><Trash2 :size="15" /></button><small v-if="!page.editable">자동 생성</small></span>
        </div>
        <p v-if="pages.length === 0" class="admin-empty">조건에 맞는 문서가 없습니다.</p>
      </div>
    </VaCard>
    <VaCard>
      <VaCardTitle class="admin-link-report-title"><span>링크 점검</span><VaButton size="small" :loading="linkLoading" @click="loadLinkReport">보고서 새로 계산</VaButton></VaCardTitle>
      <VaCardContent><p>선택한 언어판의 전체 문서를 점검합니다. 문서 검색어는 링크 점검에 적용되지 않습니다.</p></VaCardContent>
      <VaCardContent v-if="linkReport" class="admin-link-report">
        <div class="admin-link-metrics"><span>문서 <b>{{ linkReport.documents }}</b></span><span>내부 링크 <b>{{ linkReport.links }}</b></span><span>없는 문서 링크 <b>{{ linkReport.missing_links.length }}</b></span><span>고립 문서 <b>{{ linkReport.orphan_documents.length }}</b></span></div>
        <section><h3>없는 문서 링크</h3><div v-for="entry in linkReport.missing_links" :key="`${entry.source.url}:${entry.target.url}`" class="admin-link-row"><span><RouterLink :to="entry.source.url">{{ entry.source.name }}</RouterLink><small>{{ entry.source.locale }}</small></span><span>→ <RouterLink class="missing-wiki-link" :to="entry.target.url">{{ entry.target.name }}</RouterLink></span><RouterLink :to="entry.source.edit_url || entry.source.url">원문 수정</RouterLink></div><p v-if="!linkReport.missing_links.length" class="admin-empty">깨진 내부 링크가 없습니다.</p></section>
        <section><h3>고립 문서</h3><div class="admin-orphan-list"><RouterLink v-for="page in linkReport.orphan_documents" :key="page.url" :to="page.url">{{ page.name }} <small>{{ page.locale }}</small></RouterLink></div><p v-if="!linkReport.orphan_documents.length" class="admin-empty">고립 문서가 없습니다.</p></section>
      </VaCardContent>
      <VaCardContent v-else><p class="admin-empty">현재 언어판의 링크 구조를 계산하려면 보고서를 실행하세요.</p></VaCardContent>
    </VaCard>
    <Teleport to="body">
      <div v-if="techdataPage" class="admin-modal-backdrop" @mousedown.self="closeTechdata">
        <section class="techdata-link-dialog" role="dialog" aria-modal="true" aria-labelledby="techdata-link-title">
          <header><div><p>문서와 중앙 데이터 연결</p><h2 id="techdata-link-title">{{ techdataPage.name }}</h2><span>{{ techdataPage.locale }} · 문서 ID {{ techdataPage.wid }}</span></div><button type="button" title="닫기" aria-label="닫기" @click="closeTechdata"><X :size="20" /></button></header>
          <VaAlert v-if="techdataError" color="danger">{{ techdataError }}</VaAlert>
          <div v-if="techdataPage.tid" class="current-techdata-link">
            <Database :size="22" />
            <div><strong>Techdata {{ techdataPage.tid }}에 연결됨</strong><p>같은 항목에 다른 언어판 문서를 연결하면 언어 전환 관계가 만들어집니다.</p></div>
            <RouterLink :to="`/admin/techdata/${techdataPage.tid}`">항목 보기</RouterLink>
            <VaButton preset="secondary" color="danger" :loading="techdataBusy" @click="unlinkTechdata">연결 해제</VaButton>
          </div>
          <template v-else>
            <div class="techdata-create-option"><div><Database :size="21" /><span><strong>새 Techdata 항목 생성</strong><small>{{ techdataPage.locale }} 이름을 “{{ techdataPage.name }}”으로 만들고 이 문서를 즉시 연결합니다.</small></span></div><VaButton :loading="techdataBusy" @click="connectTechdata()">새 항목 만들기</VaButton></div>
            <div v-if="techdataDuplicateBlocked" class="techdata-duplicate-warning"><strong>중복 후보 {{ techdataItems.length }}개를 찾았습니다.</strong><p>같은 대상이면 아래 기존 항목을 연결하세요. 실제로 다른 대상인 경우에만 별도 항목 생성을 확인할 수 있습니다.</p><VaButton size="small" preset="secondary" :loading="techdataBusy" @click="connectTechdata('', true)">다른 대상임을 확인하고 생성</VaButton></div>
            <div class="techdata-dialog-divider"><span>또는 기존 항목 연결</span></div>
            <label class="techdata-item-search"><Search :size="17" /><input v-model="techdataQuery" type="search" :disabled="techdataBusy" placeholder="TID, 이름, 설명, 별칭 검색" autofocus></label>
            <div v-if="techdataLoading" class="admin-loading"><VaProgressCircle indeterminate /> 항목을 찾는 중입니다.</div>
            <div v-else class="techdata-item-options">
              <button v-for="item in techdataItems" :key="item.id" type="button" :disabled="Boolean(item.wiki_pages[techdataPage.locale]) || techdataBusy" @click="connectTechdata(item.id)">
                <span><strong>{{ techdataLabel(item) }}</strong><small>{{ techdataDescription(item) || '설명 없음' }}</small></span><span><code>{{ item.id }}</code><small v-if="item.wiki_pages[techdataPage.locale]">{{ techdataPage.locale }} 문서가 이미 연결됨</small><small v-else>{{ Object.keys(item.wiki_pages).length }}개 언어 연결</small></span>
              </button>
              <p v-if="!techdataItems.length" class="admin-empty">검색 결과가 없습니다.</p>
            </div>
          </template>
        </section>
      </div>
    </Teleport>
  </div>
</template>
