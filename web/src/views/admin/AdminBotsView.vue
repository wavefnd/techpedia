<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  acceptNamingSuggestion, getNamingSuggestions, rejectNamingSuggestion, scanNamingIndex,
} from '../../services/api'
import { activeLanguageEditions, loadLanguageEditions } from '../../services/editions'
import { documentURL } from '../../i18n'
import { LatestRequest } from '../../services/latestRequest'
import type { NamingScanResult, NamingSuggestion, NamingSuggestionStatus } from '../../types/api'

const suggestions = ref<NamingSuggestion[]>([])
const loading = ref(false)
const scanning = ref(false)
const error = ref('')
const notice = ref('')
const status = ref<NamingSuggestionStatus | ''>('open')
const scanLocale = ref('')
const scanResult = ref<NamingScanResult | null>(null)
const editionsReady = ref(false)
const editionError = ref('')
const busy = ref(new Set<string>())
const requests = new LatestRequest()
let mounted = true

const areaLabels = { title: '문서 이름', link: '내부 링크', mention: '본문 언급' }
const openCount = computed(() => suggestions.value.filter((value) => value.status === 'open').length)

async function load() {
  loading.value = true
  error.value = ''
  const filter = status.value
  await requests.run((signal) => getNamingSuggestions(filter, signal), {
    success: (value) => { suggestions.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '봇 데이터를 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

async function scan() {
  if (!editionsReady.value || scanning.value || busy.value.size) return
  scanning.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await scanNamingIndex(scanLocale.value)
    if (!mounted) return
    scanResult.value = result
    notice.value = `${scanResult.value.documents_scanned}개 문서를 순회해 새 제안 ${scanResult.value.created}개를 등록했습니다.`
    if (status.value !== 'open') status.value = 'open'
    else await load()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '문서 검사를 실행하지 못했습니다.'
  } finally {
    if (mounted) scanning.value = false
  }
}

async function accept(suggestion: NamingSuggestion) {
  if (scanning.value || busy.value.has(suggestion.id)) return
  if (!window.confirm(`'${suggestion.page_name}' 수정 제안을 적용하시겠습니까?`)) return
  busy.value.add(suggestion.id)
  try {
    await acceptNamingSuggestion(suggestion.id, suggestion.proposed_name, suggestion.proposed_source)
    if (!mounted) return
    notice.value = '제안을 적용하고 검토 기록을 남겼습니다.'
    await load()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '제안을 적용하지 못했습니다.'
  } finally { if (mounted) busy.value.delete(suggestion.id) }
}

async function reject(suggestion: NamingSuggestion) {
  if (scanning.value || busy.value.has(suggestion.id)) return
  if (!window.confirm(`'${suggestion.page_name}' 수정 제안을 거절하시겠습니까?`)) return
  busy.value.add(suggestion.id)
  try {
    await rejectNamingSuggestion(suggestion.id)
    if (!mounted) return
    notice.value = '제안을 거절했습니다.'
    await load()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '제안을 거절하지 못했습니다.'
  } finally { if (mounted) busy.value.delete(suggestion.id) }
}

watch(status, () => void load())
async function loadEditions() {
  editionError.value = ''
  try { await loadLanguageEditions(); if (mounted) editionsReady.value = true }
  catch { if (mounted) { editionsReady.value = false; editionError.value = '언어판을 불러오지 못했습니다. 기존 제안은 검토할 수 있지만 새 검사는 실행할 수 없습니다.' } }
}
onMounted(async () => { await Promise.all([loadEditions(), load()]) })
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view naming-bot-view">
    <header class="admin-page-header">
      <div><p>Techdata와 내부 링크 그래프 기반 검토 자동화</p><h1>TechPedia 크롤러</h1></div>
      <VaBadge :text="`검토 대기 ${openCount}개`" color="primary" />
    </header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaButton v-if="error" preset="secondary" :disabled="loading" @click="load">제안 다시 불러오기</VaButton>
    <VaAlert v-if="editionError" color="warning">{{ editionError }} <VaButton preset="secondary" @click="loadEditions">언어판 다시 불러오기</VaButton></VaAlert>
    <VaAlert v-if="notice" color="success">{{ notice }}</VaAlert>

    <VaCard>
      <VaCardTitle>내부 문서 순회</VaCardTitle>
      <VaCardContent class="bot-scan-form">
        <label>검사 언어판<select v-model="scanLocale" :disabled="!editionsReady || scanning"><option value="">모든 활성 언어판</option><option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option></select></label>
        <VaButton :loading="scanning" :disabled="!editionsReady || scanning || busy.size > 0" @click="scan">위키 크롤러 실행</VaButton>
        <div v-if="scanResult" class="bot-crawl-result">
          <span>생성기 <b>{{ scanResult.generator_version }}</b></span>
          <span>문서 <b>{{ scanResult.documents_scanned }}</b></span>
          <span>명칭 색인 <b>{{ scanResult.indexed_terms }}</b></span>
          <span>색인 충돌 <b>{{ scanResult.index_collisions }}</b></span>
          <span>내부 링크 <b>{{ scanResult.links_discovered }}</b></span>
          <span>없는 문서 링크 <b>{{ scanResult.missing_links }}</b></span>
          <span>고립 문서 <b>{{ scanResult.orphan_documents }}</b></span>
          <span>새 제안 <b>{{ scanResult.created }}</b></span>
          <small>기존 제안 {{ scanResult.existing }}개는 중복 등록하지 않았습니다.</small>
        </div>
      </VaCardContent>
    </VaCard>

    <section class="admin-section">
      <div class="bot-suggestion-heading"><h2>수정 제안</h2><label>상태<select v-model="status"><option value="open">검토 대기</option><option value="accepted">수락됨</option><option value="rejected">거절됨</option><option value="">전체</option></select></label></div>
      <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 제안을 불러오는 중입니다.</div>
      <div v-else class="naming-suggestion-list">
        <VaCard v-for="suggestion in suggestions" :key="suggestion.id" class="naming-suggestion-card">
          <VaCardTitle>
            <span><RouterLink :to="documentURL('wiki', suggestion.locale, suggestion.page_name)">{{ suggestion.page_name }}</RouterLink><small>{{ suggestion.locale }} · {{ suggestion.base_revision }}</small></span>
            <VaBadge :text="suggestion.status === 'open' ? '검토 대기' : suggestion.status === 'accepted' ? '수락됨' : '거절됨'" :color="suggestion.status === 'open' ? 'warning' : suggestion.status === 'accepted' ? 'success' : 'secondary'" />
          </VaCardTitle>
          <VaCardContent>
            <div class="naming-findings"><span v-for="finding in suggestion.findings" :key="`${finding.checker_id}:${finding.area}:${finding.before}`"><b>{{ areaLabels[finding.area] }}</b> {{ finding.before }} → {{ finding.after }}</span></div>
            <label>제안 문서 이름<input v-model="suggestion.proposed_name" :readonly="suggestion.status !== 'open' || busy.has(suggestion.id) || scanning"></label>
            <label v-if="suggestion.original_source !== suggestion.proposed_source">제안 원문<textarea v-model="suggestion.proposed_source" rows="12" spellcheck="false" :readonly="suggestion.status !== 'open' || busy.has(suggestion.id) || scanning"></textarea></label>
            <div v-if="suggestion.status === 'open'" class="suggestion-actions"><VaButton :disabled="scanning || busy.has(suggestion.id)" @click="accept(suggestion)">변경 내용 수락</VaButton><VaButton preset="secondary" color="danger" :disabled="scanning || busy.has(suggestion.id)" @click="reject(suggestion)">거절</VaButton><RouterLink :to="documentURL('edit', suggestion.locale, suggestion.page_name)">편집기에서 확인</RouterLink></div>
            <small v-else-if="suggestion.reviewed_by">{{ suggestion.reviewed_by }} · {{ new Date(suggestion.reviewed_at || '').toLocaleString() }}</small>
          </VaCardContent>
        </VaCard>
        <p v-if="!suggestions.length" class="admin-empty">해당 상태의 수정 제안이 없습니다.</p>
      </div>
    </section>
  </div>
</template>
