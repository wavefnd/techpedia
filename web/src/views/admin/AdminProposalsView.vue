<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { request } from '../../services/api'
import { LatestRequest } from '../../services/latestRequest'
import { documentURL } from '../../i18n'

type Page = { locale: string; name: string; revision?: string }
type Proposal = { id: string; kind: string; page: Page; target?: Page; summary: string; status: string; submitted_by: string; created_at: string; expires_at: string; decided_by?: string; result_revision?: string; supersedes?: string; superseded_by?: string; amended_at?: string }
type Diff = { lines: Array<{ kind: string; text: string; old_line?: number; new_line?: number }>; diff_offset: number; diff_limit: number; diff_total: number; changes_only: boolean; added_lines: number; removed_lines: number }
type Detail = Diff & { proposal: Proposal; before: string; after: string; review_digest: string; diff_truncated: boolean }
type BatchReview = { proposal: Proposal; review_digest: string; diff: Diff | null; loading: boolean; error: string }
type Preview = Diff & { proposal: Proposal; review_digest: string }
type BatchResult = { id: string; proposal?: Proposal; error?: string }
const route = useRoute()
const router = useRouter()
const proposals = ref<Proposal[]>([])
const detail = ref<Detail | null>(null)
const diff = ref<Diff | null>(null)
const diffLoading = ref(false)
const diffError = ref('')
const wrap = ref(true)
const reviewMode = ref<'diff' | 'before' | 'after'>('diff')
const loading = ref(false)
const reviewing = ref(false)
const busy = ref(false)
const checked = ref(false)
const error = ref('')
const detailError = ref('')
const notice = ref('')
const retentionDays = ref(2)
const savedRetentionDays = ref(2)
const policyReady = ref(false)
const policyError = ref('')
const selectedIDs = ref<string[]>([])
const batchOpen = ref(false)
const batchChecked = ref(false)
const batchReviews = ref<BatchReview[]>([])
const batchLoading = ref(false)
const batchPanel = ref<HTMLElement | null>(null)
let batchGeneration = 0
let batchController: AbortController | null = null
const batchResults = ref<BatchResult[]>([])
const batchError = ref('')
const queuePage = ref(0)
const onlyPending = ref(true)
const lists = new LatestRequest()
const reviews = new LatestRequest()
const diffRequests = new LatestRequest()
let mounted = true
const kindName = (kind: string) => ({ create: '문서 생성', edit: '문서 수정', link: '언어 연결' }[kind] || kind)
const statusName = (status: string) => ({ pending: '검토 대기', applied: '승인·적용', rejected: '거절', superseded: '수정본으로 대체됨' }[status] || status)
const pending = (p: Proposal) => p.status === 'pending' && Date.parse(p.expires_at) > Date.now()
const pendingCount = computed(() => proposals.value.filter(pending).length)
const filteredProposals = computed(() => onlyPending.value ? proposals.value.filter(pending) : proposals.value)
const queuePages = computed(() => Math.max(1, Math.ceil(filteredProposals.value.length / 50)))
const visibleProposals = computed(() => filteredProposals.value.slice(queuePage.value * 50, (queuePage.value + 1) * 50))
const selected = (id: string) => selectedIDs.value.includes(id)
const selectablePage = computed(() => visibleProposals.value.filter(pending))
const pageSelected = computed(() => selectablePage.value.length > 0 && selectablePage.value.every((p) => selected(p.id)))
const batchReady = computed(() => batchOpen.value && !batchLoading.value && batchReviews.value.length > 0 && batchReviews.value.length === selectedIDs.value.length && batchReviews.value.every((entry) => !!entry.review_digest && !entry.loading && !entry.error && pending(entry.proposal)))
const diffRange = computed(() => diff.value && diff.value.diff_total ? `${diff.value.diff_offset + 1}–${Math.min(diff.value.diff_offset + diff.value.lines.length, diff.value.diff_total)} / ${diff.value.diff_total}` : '표시할 변경 없음')
const formatDate = (value: string) => new Date(value).toLocaleString('ko', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })

async function load() {
  loading.value = true; error.value = ''
  await lists.run((signal) => request<Proposal[]>('/api/admin/mcp/proposals', { signal }), {
    success: (value) => {
      proposals.value = value
      const updated = value.find((p) => p.id === detail.value?.proposal.id)
      if (updated?.status === 'superseded' && detail.value) {
        reviews.invalidate(); diffRequests.invalidate(); checked.value = false; reviewing.value = false; diffLoading.value = false
        detail.value = { ...detail.value, proposal: updated }
      }
      const waiting = new Set(value.filter(pending).map((p) => p.id))
      const next = selectedIDs.value.filter((id) => waiting.has(id))
      if (next.length !== selectedIDs.value.length) { invalidateBatch(); selectedIDs.value = next }
      queuePage.value = Math.min(queuePage.value, queuePages.value - 1)
    },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '제안을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}
async function loadPolicy() {
  policyError.value = ''; policyReady.value = false
  try {
    const policy = await request<{ retention_days: number }>('/api/admin/mcp/proposal-policy')
    if (!mounted) return
    retentionDays.value = savedRetentionDays.value = policy.retention_days
    policyReady.value = true
  } catch (reason) {
    if (mounted) policyError.value = reason instanceof Error ? reason.message : '보관 설정을 불러오지 못했습니다.'
  }
}
async function savePolicy() {
  if (busy.value || !policyReady.value || retentionDays.value === savedRetentionDays.value) return
  if (!window.confirm(`제안 보관을 제출 시점부터 ${retentionDays.value}일로 설정할까요? 기간을 줄이면 기존 제안과 검토 원문도 만료될 수 있습니다. 문서와 리비전은 삭제하지 않습니다.`)) return
  busy.value = true; policyError.value = ''; notice.value = ''
  try {
    const policy = await request<{ retention_days: number }>('/api/admin/mcp/proposal-policy', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ retention_days: retentionDays.value }),
    })
    if (!mounted) return
    retentionDays.value = savedRetentionDays.value = policy.retention_days
    notice.value = '보관 기간을 저장했습니다. 기존 검토 선택은 초기화했습니다.'
  } catch (reason) {
    if (mounted) policyError.value = reason instanceof Error ? reason.message : '설정을 저장하지 못했습니다.'
  } finally {
    if (mounted) {
      reviews.invalidate(); diffRequests.invalidate(); detail.value = null; diff.value = null; checked.value = false; invalidateBatch(); selectedIDs.value = []
      reviewing.value = false; diffLoading.value = false
      await load(); busy.value = false
    }
  }
}
function invalidateBatch() {
  batchGeneration++; batchController?.abort(); batchController = null
  batchOpen.value = false; batchChecked.value = false; batchReviews.value = []; batchLoading.value = false
}
function setSelection(ids: string[]) {
  if (busy.value) return
  if (ids.length > 100) { batchError.value = '한 번에 최대 100건을 선택할 수 있습니다. 접수 건수 제한은 없습니다.'; return }
  invalidateBatch(); selectedIDs.value = ids; batchError.value = ''
}
function toggleSelected(p: Proposal) {
  if (!pending(p)) return
  setSelection(selected(p.id) ? selectedIDs.value.filter((id) => id !== p.id) : [...selectedIDs.value, p.id])
}
function togglePage() {
  const ids = new Set(selectedIDs.value)
  for (const p of selectablePage.value) { if (pageSelected.value) ids.delete(p.id); else ids.add(p.id) }
  setSelection([...ids])
}
async function reviewSelected() {
  if (busy.value || !selectedIDs.value.length) return
  const summaries = new Map(proposals.value.filter(pending).map((p) => [p.id, p]))
  if (selectedIDs.value.some((id) => !summaries.has(id))) {
    invalidateBatch(); batchError.value = '선택한 제안이 없거나 만료되었습니다. 목록을 새로고침하고 다시 선택하세요.'; return
  }
  invalidateBatch(); reviews.invalidate(); diffRequests.invalidate(); checked.value = false
  reviewing.value = false; diffLoading.value = false
  const generation = batchGeneration
  const controller = batchController = new AbortController()
  batchReviews.value = selectedIDs.value.map((id) => ({ proposal: summaries.get(id)!, review_digest: '', diff: null, loading: true, error: '' }))
  batchOpen.value = true; batchLoading.value = true
  let next = 0
  const current = () => mounted && generation === batchGeneration
  void nextTick(() => { if (current()) batchPanel.value?.scrollIntoView({ block: 'start', behavior: 'instant' }) })
  async function worker() {
    while (current() && next < batchReviews.value.length) {
      const index = next++, entry = batchReviews.value[index]!
      try {
        const value = await request<Preview>(`/api/admin/mcp/proposals/${encodeURIComponent(entry.proposal.id)}?preview=true`, { signal: controller.signal })
        if (!current()) return
        if (value.proposal.id !== entry.proposal.id || !pending(value.proposal)) throw new Error('이미 처리되었거나 만료된 제안입니다. 선택을 해제하고 목록을 새로고침하세요.')
        entry.proposal = value.proposal; entry.review_digest = value.review_digest; entry.diff = value
      } catch (reason) {
        if (!current()) return
        entry.error = reason instanceof Error ? reason.message : '검토 내용을 불러오지 못했습니다.'
      } finally { if (current()) entry.loading = false }
    }
  }
  await Promise.all([worker(), worker()])
  if (current()) batchLoading.value = false
}
async function loadBatchDiff(entry: BatchReview, offset: number) {
  if (busy.value || entry.loading || !entry.diff) return
  const generation = batchGeneration
  batchChecked.value = false; entry.loading = true; entry.error = ''
  try {
    const value = await request<Diff>(`/api/admin/mcp/proposals/${encodeURIComponent(entry.proposal.id)}/diff?offset=${offset}&limit=50&changes_only=true`, { signal: batchController?.signal })
    if (mounted && generation === batchGeneration) entry.diff = value
  } catch (reason) {
    if (mounted && generation === batchGeneration) entry.error = reason instanceof Error ? reason.message : '비교를 불러오지 못했습니다.'
  } finally { if (mounted && generation === batchGeneration) entry.loading = false }
}
async function decideBatch(allow: boolean) {
  if (busy.value || !batchReady.value || (allow && !batchChecked.value)) return
  const entries = [...batchReviews.value]
  busy.value = true; batchError.value = ''; notice.value = ''; batchResults.value = []
  reviews.invalidate(); diffRequests.invalidate(); checked.value = false; reviewing.value = false; diffLoading.value = false
  try {
    const response = await request<{ results: BatchResult[] }>('/api/admin/mcp/proposals/decide-batch', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ allow, confirmation: `${allow ? 'approve' : 'reject'}:${entries.length}`, proposals: entries.map((entry) => ({ id: entry.proposal.id, review_digest: entry.review_digest })) }),
    })
    if (!mounted) return
    batchResults.value = response.results
    const succeeded = response.results.filter((result) => !!result.proposal).length
    notice.value = `${succeeded}건 ${allow ? '승인·적용' : '거절'}, ${response.results.length - succeeded}건 처리 실패. 실패한 제안은 다시 검토하세요.`
    const current = response.results.find((result) => result.id === detail.value?.proposal.id)
    if (current?.proposal && detail.value) detail.value = { ...detail.value, proposal: current.proposal }
  } catch (reason) {
    if (!mounted) return
    batchError.value = `${reason instanceof Error ? reason.message : '일괄 처리 응답을 받지 못했습니다.'} 일부 항목이 처리되었을 수 있습니다. 목록과 처리 결과를 확인하세요. 자동 재시도하지 않습니다.`
  } finally {
    if (mounted) { invalidateBatch(); selectedIDs.value = []; await load(); busy.value = false }
  }
}
async function review(id: string) {
  if (busy.value) return
  invalidateBatch()
  diffRequests.invalidate(); diff.value = null; diffLoading.value = false; diffError.value = ''; reviewMode.value = 'diff'
  detail.value = null; checked.value = false; detailError.value = ''; reviewing.value = true
  await reviews.run((signal) => request<Detail>(`/api/admin/mcp/proposals/${encodeURIComponent(id)}`, { signal }), {
    success: (value) => { detail.value = value; diff.value = value },
    error: (reason) => { detailError.value = reason instanceof Error ? reason.message : '검토 내용을 불러오지 못했습니다.' },
    settled: () => { reviewing.value = false },
  })
}
async function loadDiff(offset: number, changesOnly = diff.value?.changes_only ?? true) {
  const snapshot = detail.value
  if (busy.value || !snapshot) return
  diffLoading.value = true; diffError.value = ''
  await diffRequests.run((signal) => request<Diff>(`/api/admin/mcp/proposals/${encodeURIComponent(snapshot.proposal.id)}/diff?offset=${offset}&limit=500&changes_only=${changesOnly}`, { signal }), {
    success: (value) => { if (detail.value?.proposal.id === snapshot.proposal.id) diff.value = value },
    error: (reason) => { diffError.value = reason instanceof Error ? reason.message : '변경 비교를 불러오지 못했습니다.'; checked.value = false },
    settled: () => { diffLoading.value = false },
  })
}
async function select(id: string) {
  if (busy.value) return
  if (id === route.query.id) { await review(id); return }
  await router.replace({ path: '/admin/proposals', query: { id } })
}
async function decide(allow: boolean) {
  const snapshot = detail.value
  if (busy.value || !snapshot || !pending(snapshot.proposal) || (allow && (!checked.value || diffLoading.value || !!diffError.value))) return
  if (!window.confirm(allow ? `${kindName(snapshot.proposal.kind)} 제안을 승인하고 적용할까요?` : '이 제안을 거절할까요?')) return
  busy.value = true; detailError.value = ''; notice.value = ''
  reviews.invalidate()
  try {
    const result = await request<Proposal>(`/api/admin/mcp/proposals/${encodeURIComponent(snapshot.proposal.id)}/decide`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ allow, confirmation: snapshot.proposal.id, review_digest: snapshot.review_digest }),
    })
    if (!mounted) return
    detail.value = { ...snapshot, proposal: result }
    checked.value = false
    notice.value = allow ? '승인하여 적용했습니다.' : '제안을 거절했습니다.'
    await load()
  } catch (reason) {
    if (!mounted) return
    detailError.value = reason instanceof Error ? reason.message : '처리하지 못했습니다.'
    // Never retry an approval automatically or leave a stale approval enabled.
    checked.value = false
  } finally { if (mounted) busy.value = false }
}
watch(() => route.query.id, (value) => {
  if (busy.value) return
  if (typeof value === 'string') void review(value)
  else { reviews.invalidate(); diffRequests.invalidate(); detail.value = null; diff.value = null; diffLoading.value = false; diffError.value = ''; checked.value = false; reviewing.value = false }
})
watch(onlyPending, () => { queuePage.value = 0 })
onMounted(async () => { await Promise.all([load(), loadPolicy()]); if (mounted && typeof route.query.id === 'string') await review(route.query.id) })
onBeforeUnmount(() => { mounted = false; invalidateBatch(); lists.dispose(); reviews.dispose(); diffRequests.dispose() })
</script>

<template>
  <div class="admin-view admin-proposals-view">
    <header class="admin-page-header"><div><p>사람이 검토하는 변경</p><h1>ChatGPT 제안 검토</h1><span>제출만으로는 문서가 바뀌지 않습니다. TechMark 원문·diff 또는 언어 연결을 확인한 뒤 승인하세요.</span></div><VaButton preset="secondary" :disabled="loading || busy" @click="load">목록 새로고침</VaButton></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaAlert v-if="notice" color="success">{{ notice }}</VaAlert>
    <VaAlert v-if="batchError" color="danger">{{ batchError }}</VaAlert>
    <section class="admin-panel proposal-batch" aria-label="보관 설정 및 일괄 검토">
      <div class="proposal-policy"><label>제안 보관 <select v-model.number="retentionDays" :disabled="busy || !policyReady"><option :value="1">1일</option><option :value="2">2일</option><option :value="3">3일</option></select></label><VaButton preset="secondary" :disabled="busy || !policyReady || retentionDays === savedRetentionDays" @click="savePolicy">설정 저장</VaButton><span>접수 건수·누적 원문 용량 제한 없음 · 건별 원문 최대 2 MiB</span></div>
      <p>보관은 제출 시점부터 계산합니다. 기간을 줄이면 기존 제안에도 적용되며, 연장해도 만료된 제안은 복구되지 않습니다.</p>
      <p v-if="policyError" role="alert">{{ policyError }} <button type="button" :disabled="busy" @click="loadPolicy">설정 다시 불러오기</button></p>
      <div class="proposal-batch-actions"><b>선택 {{ selectedIDs.length }}건</b><VaButton :disabled="busy || batchLoading || !selectedIDs.length" @click="reviewSelected">선택한 제안 모아서 검토</VaButton><VaButton preset="secondary" :disabled="busy || !selectedIDs.length" @click="setSelection([])">선택 해제</VaButton></div>
      <p>목록에서 여러 제안을 선택하거나 현재 페이지를 전체 선택하세요. 모아 본 변경 내용을 한 번 확인하고 일괄 승인할 수 있습니다.</p>
      <ul v-if="batchResults.some((result) => result.error)" class="proposal-batch-errors" aria-label="일괄 처리 실패"><li v-for="result in batchResults.filter((item) => item.error)" :key="result.id"><button type="button" :disabled="busy" @click="select(result.id)">{{ proposals.find((p) => p.id === result.id)?.page.name || '제안 다시 확인' }}</button>: {{ result.error }}</li></ul>
    </section>
    <div class="proposal-workspace">
      <section class="admin-panel proposal-queue" aria-label="변경 제안 목록">
        <header class="admin-panel-header"><h2>제안 목록</h2><small>대기 {{ pendingCount }} · 전체 {{ proposals.length }}</small></header>
        <p class="proposal-queue-note">{{ policyReady ? `제출 후 ${savedRetentionDays}일간 보관됩니다.` : '보관 설정 확인 중입니다.' }}<br>삭제·연결 해제·병합은 지원하지 않습니다.</p>
        <label class="proposal-queue-filter"><input v-model="onlyPending" type="checkbox" :disabled="busy"> 대기 제안만</label>
        <label class="proposal-queue-filter proposal-select-page"><input type="checkbox" :checked="pageSelected" :disabled="busy || !selectablePage.length" @change="togglePage"> 현재 페이지 전체 선택</label>
        <p v-if="loading">목록을 불러오는 중입니다.</p>
        <div v-for="p in visibleProposals" :key="p.id" class="proposal-queue-entry" :class="{ 'batch-selected': selected(p.id) }">
          <label class="proposal-row-select"><input type="checkbox" :checked="selected(p.id)" :disabled="busy || !pending(p)" :aria-label="`${p.page.locale} ${p.page.name} 일괄 선택`" @change="toggleSelected(p)"></label>
          <button class="proposal-queue-row" :class="{ selected: !batchOpen && detail?.proposal.id === p.id }" :aria-current="!batchOpen && detail?.proposal.id === p.id ? 'true' : undefined" type="button" :disabled="busy" @click="select(p.id)">
          <span class="proposal-row-meta"><span>{{ kindName(p.kind) }}</span><span class="proposal-status" :class="p.status">{{ statusName(p.status) }}</span></span><b>{{ p.page.name }}</b><small>{{ p.page.locale }} · {{ p.submitted_by }}</small><time :datetime="p.created_at">{{ formatDate(p.created_at) }}</time>
          </button>
        </div>
        <nav v-if="queuePages > 1" class="proposal-queue-pagination" aria-label="제안 목록 페이지"><button type="button" :disabled="busy || queuePage === 0" @click="queuePage--">이전</button><span>{{ queuePage + 1 }} / {{ queuePages }}</span><button type="button" :disabled="busy || queuePage + 1 >= queuePages" @click="queuePage++">다음</button></nav>
        <p v-if="!loading && !filteredProposals.length && !error" class="admin-empty">{{ onlyPending ? '대기 중인 제안이 없습니다.' : '보관된 제안이 없습니다.' }}</p>
      </section>
      <section v-if="batchOpen" ref="batchPanel" class="admin-panel proposal-review proposal-batch-review" aria-label="선택한 제안 일괄 검토">
        <header class="proposal-review-header"><div><p>선택한 변경 모아 보기</p><h2>{{ batchReviews.length }}건 일괄 검토</h2></div><VaButton preset="secondary" :disabled="busy" @click="invalidateBatch">단건 보기</VaButton></header>
        <p v-if="batchLoading" role="status">{{ batchReviews.filter((entry) => !entry.loading).length }} / {{ batchReviews.length }}건 불러오는 중…</p>
        <p v-else>선택한 제안들의 변경 내용과 언어 연결을 검토한 뒤 하단에서 일괄 승인하세요.</p>
        <article v-for="entry in batchReviews" :key="entry.proposal.id" class="proposal-batch-item" :aria-label="`${entry.proposal.page.name} 변경 검토`">
          <header class="proposal-review-header"><div><p>{{ kindName(entry.proposal.kind) }} · {{ entry.proposal.page.locale }} · {{ entry.proposal.page.revision || '새 문서' }}</p><h3>{{ entry.proposal.page.name }}</h3></div><RouterLink :to="{ path: '/admin/proposals', query: { id: entry.proposal.id } }" target="_blank" rel="noopener">전체 원문 ↗</RouterLink></header>
          <p class="proposal-batch-summary">{{ entry.proposal.summary }}</p>
          <p v-if="entry.proposal.superseded_by" class="proposal-diff-warning">이 제안은 수정본으로 대체되었습니다. <button type="button" :disabled="busy" @click="select(entry.proposal.superseded_by)">수정본 검토</button></p>
          <p v-if="entry.loading" role="status">검토 내용을 불러오는 중…</p>
          <p v-if="entry.error" class="proposal-diff-warning" role="alert">{{ entry.error }} <button type="button" :disabled="busy || batchLoading" @click="reviewSelected">검토 다시 불러오기</button></p>
          <div v-if="entry.review_digest && entry.proposal.kind === 'link' && entry.proposal.target" class="proposal-link-pair"><RouterLink :to="documentURL('wiki', entry.proposal.page.locale, entry.proposal.page.name)" target="_blank" rel="noopener">{{ entry.proposal.page.locale }} · {{ entry.proposal.page.name }} ({{ entry.proposal.page.revision }})</RouterLink><span>↔ 같은 Techdata 항목으로 연결</span><RouterLink :to="documentURL('wiki', entry.proposal.target.locale, entry.proposal.target.name)" target="_blank" rel="noopener">{{ entry.proposal.target.locale }} · {{ entry.proposal.target.name }} ({{ entry.proposal.target.revision }})</RouterLink></div>
          <div v-else-if="entry.diff" class="proposal-editor">
            <div class="proposal-diff-toolbar"><div class="proposal-diff-counts"><span class="added">+{{ entry.diff.added_lines }} 추가</span><span class="removed">−{{ entry.diff.removed_lines }} 삭제</span></div><span>변경 부분 비교</span></div>
            <div class="proposal-diff-columns" aria-hidden="true"><span>이전</span><span>제안</span><span></span><span>TechMark 원문</span></div>
            <div class="proposal-diff-scroll" tabindex="0" role="region" :aria-label="`${entry.proposal.page.name} TechMark 변경 비교`"><ol><li v-for="(line, index) in entry.diff.lines" :key="`${entry.diff.diff_offset}-${index}`" :class="line.kind"><span class="proposal-line-number">{{ line.old_line || '' }}</span><span class="proposal-line-number">{{ line.new_line || '' }}</span><span class="proposal-line-marker" aria-hidden="true">{{ line.kind === 'added' ? '+' : line.kind === 'removed' ? '−' : line.kind === 'context' ? '⋯' : '' }}</span><code>{{ line.text }}</code></li></ol></div>
            <footer class="proposal-diff-pagination"><span>{{ entry.diff.diff_total ? `${entry.diff.diff_offset + 1}–${Math.min(entry.diff.diff_offset + entry.diff.lines.length, entry.diff.diff_total)} / ${entry.diff.diff_total}` : '표시할 변경 없음' }}</span><div class="proposal-view-controls"><button type="button" :disabled="busy || entry.loading || entry.diff.diff_offset === 0" @click="loadBatchDiff(entry, Math.max(0, entry.diff.diff_offset - entry.diff.diff_limit))">이전</button><button type="button" :disabled="busy || entry.loading || entry.diff.diff_offset + entry.diff.lines.length >= entry.diff.diff_total" @click="loadBatchDiff(entry, entry.diff.diff_offset + entry.diff.diff_limit)">다음</button></div></footer>
          </div>
        </article>
        <div class="proposal-decision proposal-batch-decision"><div class="proposal-decision-copy"><label><input v-model="batchChecked" type="checkbox" :disabled="busy || !batchReady"> 선택한 {{ batchReviews.length }}건의 변경 내용과 언어 연결을 검토했습니다.</label><p>승인 전에 모든 제안의 권한·리비전·연결을 서버에서 다시 검사합니다. 충돌한 건은 적용하지 않습니다.</p></div><div class="oauth-actions"><VaButton :disabled="busy || !batchReady || !batchChecked" @click="decideBatch(true)">{{ busy ? '처리 중…' : `${batchReviews.length}건 승인하고 적용` }}</VaButton><VaButton preset="secondary" color="danger" :disabled="busy || !batchReady" @click="decideBatch(false)">선택한 제안 거절</VaButton></div></div>
      </section>
      <section v-else class="admin-panel proposal-review" aria-label="제안 검토">
        <p v-if="reviewing">검토 내용을 불러오는 중입니다.</p>
        <VaAlert v-if="detailError" color="danger">{{ detailError }}</VaAlert>
        <VaButton v-if="detailError && typeof route.query.id === 'string'" preset="secondary" :disabled="busy || reviewing" @click="review(route.query.id)">검토 내용 다시 불러오기</VaButton>
        <p v-if="!detail && !reviewing && !detailError" class="admin-empty">왼쪽에서 제안을 선택하세요.</p>
        <template v-if="detail">
          <header class="proposal-review-header"><div><p>{{ kindName(detail.proposal.kind) }} · {{ detail.proposal.page.locale }}</p><h2>{{ detail.proposal.page.name }}</h2></div><span class="proposal-status" :class="detail.proposal.status">{{ statusName(detail.proposal.status) }}</span></header>
          <dl class="proposal-metadata"><div><dt>제출자</dt><dd>{{ detail.proposal.submitted_by }}</dd></div><div><dt>기준</dt><dd>{{ detail.proposal.page.revision || '새 문서' }}</dd></div><div><dt>검토 기한</dt><dd>{{ formatDate(detail.proposal.expires_at) }}</dd></div></dl>
          <div class="proposal-summary"><b>제안 사유</b><p>{{ detail.proposal.summary }}</p></div>
          <p v-if="detail.proposal.superseded_by" class="proposal-diff-warning" role="status">이전 제안은 승인할 수 없습니다. 수정본을 새로 검토하세요. <button type="button" :disabled="busy" @click="select(detail.proposal.superseded_by)">수정본 검토</button></p>
          <p v-if="detail.proposal.supersedes" class="proposal-queue-note">{{ detail.proposal.amended_at ? `${formatDate(detail.proposal.amended_at)} 수정 제출 · ` : '' }}기존 제안의 수정본입니다. 보관 기한은 연장되지 않습니다. <button type="button" :disabled="busy" @click="select(detail.proposal.supersedes)">이전 제안 보기</button></p>
          <template v-if="detail.proposal.kind === 'link' && detail.proposal.target">
            <div class="proposal-link-pair"><RouterLink :to="documentURL('wiki', detail.proposal.page.locale, detail.proposal.page.name)">{{ detail.proposal.page.locale }} · {{ detail.proposal.page.name }} ({{ detail.proposal.page.revision }})</RouterLink><span>↔ 같은 Techdata 항목으로 연결</span><RouterLink :to="documentURL('wiki', detail.proposal.target.locale, detail.proposal.target.name)">{{ detail.proposal.target.locale }} · {{ detail.proposal.target.name }} ({{ detail.proposal.target.revision }})</RouterLink></div>
            <p>두 문서가 같은 개념인지 직접 확인하세요. 현재 연결이 바뀌었거나 해당 언어가 이미 연결된 경우 승인은 차단됩니다. 기존 항목끼리 자동 병합하지 않습니다.</p>
          </template>
          <template v-else>
            <div class="proposal-editor">
              <header class="proposal-editor-header"><div class="proposal-view-controls" aria-label="검토 보기"><button type="button" :aria-pressed="reviewMode === 'diff'" @click="reviewMode = 'diff'">변경 비교</button><button type="button" :aria-pressed="reviewMode === 'before'" @click="reviewMode = 'before'">변경 전 원문</button><button type="button" :aria-pressed="reviewMode === 'after'" @click="reviewMode = 'after'">제안 원문</button></div><span class="proposal-editor-mode">읽기 전용</span></header>
              <div v-if="reviewMode === 'diff' && diff" class="proposal-diff-view">
                <div class="proposal-diff-toolbar"><div class="proposal-diff-counts"><span class="added">+{{ diff.added_lines }} 추가</span><span class="removed">−{{ diff.removed_lines }} 삭제</span></div><div class="proposal-view-controls"><button type="button" :aria-pressed="diff.changes_only" :disabled="diffLoading || busy" @click="loadDiff(0, !diff.changes_only)">{{ diff.changes_only ? '변경 부분만' : '전체 줄' }}</button><button type="button" :aria-pressed="wrap" @click="wrap = !wrap">줄바꿈</button></div></div>
                <p v-if="detail.diff_truncated" class="proposal-diff-warning">일부 비교가 생략되었습니다. 변경 전·제안 전체 원문도 확인하세요.</p>
                <div v-if="diffError" class="proposal-diff-warning" role="alert">{{ diffError }} <button type="button" @click="loadDiff(diff.diff_offset)">다시 불러오기</button></div>
                <div class="proposal-diff-columns" aria-hidden="true"><span>이전</span><span>제안</span><span></span><span>TechMark 원문 · {{ detail.proposal.page.revision || '빈 문서' }} → 제안</span></div>
                <div class="proposal-diff-scroll" :class="{ nowrap: !wrap }" tabindex="0" role="region" aria-label="TechMark 변경 비교" :aria-busy="diffLoading"><ol><li v-for="(line, index) in diff.lines" :key="`${diff.diff_offset}-${index}`" :class="line.kind"><span class="proposal-line-number">{{ line.old_line || '' }}</span><span class="proposal-line-number">{{ line.new_line || '' }}</span><span class="proposal-line-marker" aria-hidden="true">{{ line.kind === 'added' ? '+' : line.kind === 'removed' ? '−' : line.kind === 'context' ? '⋯' : '' }}</span><code>{{ line.text }}</code></li></ol></div>
                <footer class="proposal-diff-pagination"><span aria-live="polite">{{ diffLoading ? '비교를 불러오는 중…' : diffRange }}</span><div class="proposal-view-controls"><button type="button" :disabled="diffLoading || busy || diff.diff_offset === 0" @click="loadDiff(Math.max(0, diff.diff_offset - diff.diff_limit))">이전</button><button type="button" :disabled="diffLoading || busy || diff.diff_offset + diff.lines.length >= diff.diff_total" @click="loadDiff(diff.diff_offset + diff.diff_limit)">다음</button></div></footer>
              </div>
              <template v-else><p class="proposal-source-note">{{ reviewMode === 'before' ? '변경 전' : '제안' }} 전체 TechMark 원문입니다. 승인할 내용은 제출 당시 고정되며 이 화면에서 수정하지 않습니다.</p><textarea :key="`${detail.proposal.id}-${reviewMode}`" class="proposal-source" :value="reviewMode === 'before' ? detail.before : detail.after" :aria-label="reviewMode === 'before' ? '변경 전 전체 TechMark 원문' : '제안 전체 TechMark 원문'" readonly spellcheck="false" :wrap="wrap ? 'soft' : 'off'"></textarea></template>
            </div>
          </template>
          <p v-if="detail.proposal.result_revision">적용 리비전: {{ detail.proposal.result_revision }} · 승인자 {{ detail.proposal.decided_by }}</p>
          <div v-if="pending(detail.proposal)" class="proposal-decision">
            <div class="proposal-decision-copy"><label><input v-model="checked" type="checkbox" :disabled="busy || diffLoading || !!diffError"> 원문 변경 또는 언어 연결을 검토했습니다.</label><p>승인 직전에 문서·권한·연결을 다시 확인합니다. 충돌 시 새 제안이 필요합니다.</p></div>
            <div class="oauth-actions"><VaButton :disabled="busy || !checked || diffLoading || !!diffError" @click="decide(true)">{{ busy ? '처리 중…' : '승인하고 적용' }}</VaButton><VaButton preset="secondary" color="danger" :disabled="busy" @click="decide(false)">거절</VaButton></div>
          </div>
        </template>
      </section>
    </div>
  </div>
</template>
