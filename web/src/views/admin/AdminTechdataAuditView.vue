<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Database, Link2, RefreshCw, Search, Unlink } from '@lucide/vue'
import { getAdminTechdataAudit, linkPageTechdata } from '../../services/api'
import { activeLanguageEditions, loadLanguageEditions } from '../../services/editions'
import { LatestRequest } from '../../services/latestRequest'
import type { AdminDocumentSummary, AdminTechdataAudit, AdminTechdataMetadataCandidate, AdminTechdataNameCandidate } from '../../types/api'

const audit = ref<AdminTechdataAudit | null>(null)
const query = ref('')
const locale = ref('')
const loading = ref(false)
const error = ref('')
const busyCandidate = ref('')
const editionError = ref('')
const requests = new LatestRequest()
let mounted = true

function pageMatches(page: AdminDocumentSummary, value: string): boolean {
  if (locale.value && page.locale !== locale.value) return false
  if (!value) return true
  return [page.name, page.locale, page.wid || '', page.tid || ''].some((item) => item.toLocaleLowerCase().includes(value))
}

const filteredUnlinked = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  return (audit.value?.unlinked_documents || []).filter((page) => pageMatches(page, value))
})

const filteredMetadataCandidates = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  return (audit.value?.metadata_candidates || []).filter((candidate) => {
    return pageMatches(candidate.page, value) || (!locale.value && [candidate.label, candidate.tid, ...candidate.linked_locales].some((item) => item.toLocaleLowerCase().includes(value)))
  })
})

const filteredNameCandidates = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  return (audit.value?.name_candidates || []).filter((candidate) => candidate.pages.some((page) => pageMatches(page, value)))
})

function reviewLink(page: AdminDocumentSummary) {
  return { path: '/admin/pages', query: { locale: page.locale, q: page.name } }
}

function candidateKey(candidate: AdminTechdataMetadataCandidate): string {
  return `${candidate.page.wid}:${candidate.tid}`
}

async function loadAudit() {
  loading.value = true
  error.value = ''
  await requests.run((signal) => getAdminTechdataAudit(signal), {
    success: (value) => { audit.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : 'Techdata 연결 점검 결과를 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

async function connectCandidate(candidate: AdminTechdataMetadataCandidate) {
  if (busyCandidate.value || loading.value) return
  const key = candidateKey(candidate)
  busyCandidate.value = key
  error.value = ''
  try {
    await linkPageTechdata(candidate.page.locale, candidate.page.name, candidate.tid)
    if (mounted) await loadAudit()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : 'Techdata 후보를 연결하지 못했습니다.'
  } finally {
    if (mounted) busyCandidate.value = ''
  }
}

function groupKey(candidate: AdminTechdataNameCandidate): string {
  return candidate.pages.map((page) => page.wid).join(':')
}

async function loadEditions() {
  editionError.value = ''
  try { await loadLanguageEditions() }
  catch { if (mounted) editionError.value = '언어판을 불러오지 못했습니다. 연결 점검은 사용할 수 있습니다.' }
}
onMounted(async () => { await Promise.all([loadEditions(), loadAudit()]) })
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view techdata-audit-view">
    <header class="admin-page-header">
      <div><p>중앙 데이터</p><h1>Techdata 연결 점검</h1></div>
      <VaButton preset="secondary" :loading="loading" @click="loadAudit"><RefreshCw :size="15" /> 다시 점검</VaButton>
    </header>

    <VaAlert color="info" outline>
      문서 생성을 제한하지 않고 연결 누락 가능성을 찾습니다. 후보는 자동 연결되지 않으며 관리자가 문서의 의미를 확인한 뒤 연결해야 합니다.
    </VaAlert>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaAlert v-if="editionError" color="warning">{{ editionError }} <VaButton preset="secondary" @click="loadEditions">언어판 다시 불러오기</VaButton></VaAlert>

    <div class="admin-metrics techdata-audit-metrics">
      <VaCard><VaCardContent><span>전체 저장 문서</span><strong>{{ audit?.documents ?? 0 }}</strong><Database :size="22" /></VaCardContent></VaCard>
      <VaCard><VaCardContent><span>Techdata 연결</span><strong>{{ audit?.linked_documents ?? 0 }}</strong><Link2 :size="22" /></VaCardContent></VaCard>
      <VaCard><VaCardContent><span>미연결 문서</span><strong>{{ audit?.unlinked_documents?.length ?? 0 }}</strong><Unlink :size="22" /></VaCardContent></VaCard>
      <VaCard><VaCardContent><span>검토 후보</span><strong>{{ (audit?.metadata_candidates?.length ?? 0) + (audit?.name_candidates?.length ?? 0) }}</strong><Search :size="22" /></VaCardContent></VaCard>
    </div>

    <VaCard>
      <VaCardContent class="admin-filters">
        <VaInput v-model="query" label="문서 또는 후보 검색" input-aria-label="문서 또는 후보 검색" placeholder="문서 이름, 언어 코드, WID, TID" clearable />
        <label>언어판<select v-model="locale"><option value="">모든 언어</option><option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option></select></label>
      </VaCardContent>
    </VaCard>

    <VaCard>
      <VaCardTitle>Techdata 메타데이터 일치 후보 <VaBadge :text="String(filteredMetadataCandidates.length)" color="primary" /></VaCardTitle>
      <VaCardContent class="techdata-audit-list">
        <div v-for="candidate in filteredMetadataCandidates" :key="candidateKey(candidate)" class="techdata-audit-candidate">
          <span><RouterLink :to="candidate.page.url">{{ candidate.page.name }}</RouterLink><small>{{ candidate.page.locale }} · {{ candidate.page.wid }}</small></span>
          <span><RouterLink :to="`/admin/techdata/${candidate.tid}`">{{ candidate.tid }}</RouterLink><small>{{ candidate.label }} · 연결 언어 {{ candidate.linked_locales.join(', ') || '없음' }}</small></span>
          <VaButton size="small" :loading="busyCandidate === candidateKey(candidate)" :disabled="loading || Boolean(busyCandidate)" @click="connectCandidate(candidate)">확인 후 연결</VaButton>
        </div>
        <p v-if="!loading && filteredMetadataCandidates.length === 0" class="admin-empty">현재 조건에서 메타데이터가 정확히 일치하는 후보가 없습니다.</p>
      </VaCardContent>
    </VaCard>

    <VaCard>
      <VaCardTitle>언어판 간 동명이름 후보 <VaBadge :text="String(filteredNameCandidates.length)" color="warning" /></VaCardTitle>
      <VaCardContent class="techdata-name-candidates">
        <section v-for="candidate in filteredNameCandidates" :key="groupKey(candidate)">
          <header><strong>{{ candidate.name }}</strong><span>서로 다른 언어판에서 같은 이름으로 독립 생성됨</span></header>
          <div>
            <RouterLink v-for="page in candidate.pages" :key="page.wid" :to="reviewLink(page)"><b>{{ page.name }}</b><span>{{ page.locale }}</span><code>{{ page.wid }}</code></RouterLink>
          </div>
        </section>
        <p v-if="!loading && filteredNameCandidates.length === 0" class="admin-empty">현재 조건에서 언어판 간 동명이름 후보가 없습니다.</p>
      </VaCardContent>
    </VaCard>

    <VaCard class="admin-table-card">
      <VaCardTitle>모든 미연결 문서 <VaBadge :text="String(filteredUnlinked.length)" color="danger" /></VaCardTitle>
      <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 연결 상태를 점검하는 중입니다.</div>
      <div v-else class="techdata-unlinked-table" role="table" aria-label="Techdata 미연결 문서">
        <div class="techdata-unlinked-row head" role="row"><span>문서</span><span>언어</span><span>문서 ID</span><span>최근 수정</span><span>작업</span></div>
        <div v-for="page in filteredUnlinked" :key="page.wid" class="techdata-unlinked-row" role="row">
          <span><RouterLink :to="page.url">{{ page.name }}</RouterLink><small v-if="page.system">시스템 문서</small></span>
          <span>{{ page.locale }}</span><code>{{ page.wid }}</code><time :datetime="page.updated_at">{{ new Date(page.updated_at).toLocaleString() }}</time>
          <RouterLink :to="reviewLink(page)">연결 관리</RouterLink>
        </div>
        <p v-if="filteredUnlinked.length === 0" class="admin-empty">현재 조건에 맞는 미연결 문서가 없습니다.</p>
      </div>
    </VaCard>
  </div>
</template>
