<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getRevision, getRevisionDiff, getRevisions, resetDocumentHistory, restoreRevision as restorePageRevision } from '../services/api'
import type { RevisionDiff, RevisionListResponse, RevisionResponse } from '../types/api'
import { useWikiI18n } from '../i18n'
import { hasPermission } from '../services/auth'
import { useMobileViewer } from '../composables/useMobileViewer'
import { articleHTMLDirective } from '../services/articleHTML'
import { LatestRequest } from '../services/latestRequest'

const vArticleHtml = articleHTMLDirective

const route = useRoute()
const { locale, t } = useWikiI18n()
const name = computed(() => {
  const value = route.params.name
  return (Array.isArray(value) ? value.join('/') : value).replaceAll('_', ' ')
})
const history = ref<RevisionListResponse | null>(null)
const selected = ref<RevisionResponse | null>(null)
const compareSelection = ref<string[]>([])
const diff = ref<RevisionDiff | null>(null)
const comparing = ref(false)
const restoring = ref(false)
const restoreReason = ref('')
const resetConfirmation = ref('')
const resetting = ref(false)
const resetMessage = ref('')
const mobileViewer = useMobileViewer()
const loading = ref(true)
const error = ref('')
const historyRequests = new LatestRequest()
const previewRequests = new LatestRequest()
const compareRequests = new LatestRequest()
let pageGeneration = 0

async function load() {
  pageGeneration++
  previewRequests.invalidate()
  compareRequests.invalidate()
  loading.value = true
  history.value = null
  error.value = ''
  selected.value = null
  compareSelection.value = []
  diff.value = null
  comparing.value = false
  resetMessage.value = ''
  resetConfirmation.value = ''
  restoreReason.value = ''
  await historyRequests.run((signal) => getRevisions(locale.value, name.value, signal), {
    success: (value) => { history.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : t('historyError') },
    settled: () => { loading.value = false },
  })
}

function toggleCompare(revision: string) {
  compareRequests.invalidate()
  comparing.value = false
  diff.value = null
  if (compareSelection.value.includes(revision)) {
    compareSelection.value = compareSelection.value.filter((value) => value !== revision)
    return
  }
  if (compareSelection.value.length === 2) compareSelection.value.shift()
  compareSelection.value.push(revision)
}

async function compareRevisions() {
  if (compareSelection.value.length !== 2) return
  comparing.value = true
  error.value = ''
  const ordered = [...compareSelection.value].sort((a, b) => Number(a.slice(1)) - Number(b.slice(1)))
  await compareRequests.run((signal) => getRevisionDiff(locale.value, name.value, ordered[0], ordered[1], signal), {
    success: (value) => { diff.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '리비전을 비교하지 못했습니다.' },
    settled: () => { comparing.value = false },
  })
}

async function restoreSelected() {
  if (!selected.value || !history.value?.revisions[0] || !window.confirm(`${selected.value.revision} 내용을 새 리비전으로 복원하시겠습니까?`)) return
  restoring.value = true
  const generation = pageGeneration
  error.value = ''
  try {
    await restorePageRevision(locale.value, name.value, selected.value.revision, history.value.revisions[0].revision, restoreReason.value)
    if (generation !== pageGeneration) return
    restoreReason.value = ''
    await load()
  } catch (reason) {
    if (generation !== pageGeneration) return
    error.value = reason instanceof Error ? reason.message : '리비전을 복원하지 못했습니다.'
  } finally {
    restoring.value = false
  }
}

async function resetHistory() {
  const currentRevision = history.value?.revisions[0]?.revision
  if (!history.value || !currentRevision || resetConfirmation.value !== history.value.name) return
  if (!window.confirm('현재 문서만 새 r1로 남기고 기존 리비전과 관련 기록을 영구 삭제합니다. 이 작업은 되돌릴 수 없습니다. 계속하시겠습니까?')) return
  resetting.value = true
  const generation = pageGeneration
  error.value = ''
  resetMessage.value = ''
  try {
    const result = await resetDocumentHistory(locale.value, name.value, currentRevision, resetConfirmation.value)
    if (generation !== pageGeneration) return
    await load()
    if (generation + 1 !== pageGeneration) return
    resetMessage.value = `역사를 초기화했습니다. 리비전 ${result.removed_revisions}개와 최근 변경 ${result.removed_changes}개를 정리했습니다.`
  } catch (reason) {
    if (generation !== pageGeneration) return
    error.value = reason instanceof Error ? reason.message : '문서 역사를 초기화하지 못했습니다.'
  } finally {
    resetting.value = false
  }
}

async function selectRevision(revision: string) {
  error.value = ''
  selected.value = null
  await previewRequests.run((signal) => getRevision(locale.value, name.value, revision, signal), {
    success: (value) => { selected.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : t('revisionError') },
    settled: () => {},
  })
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(locale.value, {
    dateStyle: 'medium', timeStyle: 'short',
  }).format(new Date(value))
}

watch([locale, name], () => { void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => {
  pageGeneration++
  historyRequests.dispose()
  previewRequests.dispose()
  compareRequests.dispose()
})
</script>

<template>
  <div class="history-view">
    <div v-if="loading" class="page-state">{{ t('loadingHistory') }}</div>
    <div v-else-if="error && !history" class="page-state error">{{ error }}</div>
    <template v-else-if="history">
      <header class="history-header">
        <div><p>{{ t('historyTitle') }}</p><h1>{{ history.name }}</h1></div>
        <RouterLink :to="history.url">{{ t('currentArticle') }}</RouterLink>
      </header>
      <p v-if="error" class="editor-message error">{{ error }}</p>
      <p v-if="resetMessage" class="editor-message">{{ resetMessage }}</p>
      <details v-if="history.can_reset_history && !mobileViewer" class="history-reset-panel">
        <summary>개인정보 보호를 위한 역사 초기화</summary>
        <div>
          <p>현재 문서를 새 r1로 남기고 이전 리비전, 최근 변경, 초안과 봇 제안을 영구 삭제합니다. WID, Techdata 연결과 생성일은 유지됩니다.</p>
          <p>브라우저 캐시는 비우지만 Cloudflare 캐시와 서버 백업은 별도로 정리해야 합니다.</p>
          <label>확인하려면 문서 이름 <strong>{{ history.name }}</strong>을 입력하세요.<input v-model="resetConfirmation" type="text" autocomplete="off" :placeholder="history.name"></label>
          <button type="button" :disabled="resetting || resetConfirmation !== history.name" @click="resetHistory">{{ resetting ? '초기화 중' : '문서 역사 영구 초기화' }}</button>
        </div>
      </details>
      <section v-if="history.can_compare" class="history-compare-toolbar">
        <span>비교할 리비전 두 개를 선택하세요. {{ compareSelection.length }}/2</span>
        <button type="button" :disabled="compareSelection.length !== 2 || comparing" @click="compareRevisions">{{ comparing ? '비교 중' : '선택 비교' }}</button>
      </section>
      <section v-if="history.can_compare && diff" class="revision-diff" :aria-label="`${diff.from}와 ${diff.to} 비교`">
        <header><strong>{{ diff.from }} → {{ diff.to }}</strong><span>줄 단위 비교</span></header>
        <ol><li v-for="(line, index) in diff.lines" :key="index" :class="line.kind"><span>{{ line.old_line || '' }}</span><span>{{ line.new_line || '' }}</span><code>{{ line.kind === 'added' ? '+' : line.kind === 'removed' ? '-' : ' ' }} {{ line.text }}</code></li></ol>
      </section>
      <div class="history-layout">
        <ol class="revision-list">
          <li v-for="revision in history.revisions" :key="revision.revision">
            <label v-if="history.can_compare" class="revision-compare-option"><input type="checkbox" :checked="compareSelection.includes(revision.revision)" :aria-label="`${revision.revision} 비교 선택`" @change="toggleCompare(revision.revision)"></label>
            <button type="button" :class="{ active: selected?.revision === revision.revision }" @click="selectRevision(revision.revision)">
              <strong>{{ revision.revision }}</strong>
              <span>{{ revision.summary || t('noSummary') }}</span>
              <small><RouterLink v-if="revision.author" :to="`/user/${revision.author}`">{{ revision.author }}</RouterLink> · {{ formatDate(revision.updated_at) }}</small>
            </button>
          </li>
          <li v-if="history.revisions.length === 0" class="empty-history">{{ t('noRevisions') }}</li>
        </ol>
        <article v-if="selected" class="revision-preview">
          <header><strong>{{ selected.revision }}</strong><span>{{ selected.author }} · {{ formatDate(selected.updated_at) }}</span></header>
          <div v-if="hasPermission('edit') && !mobileViewer" class="revision-restore"><input v-model="restoreReason" type="text" maxlength="200" placeholder="복원 사유"><button type="button" :disabled="restoring || selected.revision === history.revisions[0]?.revision" @click="restoreSelected">{{ restoring ? '복원 중' : '이 리비전 복원' }}</button></div>
          <div class="techmark-content" v-article-html="selected.html"></div>
        </article>
        <div v-else class="revision-empty">{{ t('selectRevision') }}</div>
      </div>
    </template>
  </div>
</template>
