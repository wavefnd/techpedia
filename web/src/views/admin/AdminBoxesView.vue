<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { ApiError, request } from '../../services/api'
import { languageEditions, loadLanguageEditions } from '../../services/editions'
import { LatestRequest } from '../../services/latestRequest'
import { articleHTMLDirective } from '../../services/articleHTML'
import { boxSyntax } from '../../services/boxes'
import type { BoxDefinition, BoxEntry, BoxPage } from '../../types/box'

const TechMarkEditor = defineAsyncComponent(() => import('../../components/TechMarkEditor.vue'))
const vArticleHtml = articleHTMLDirective
const boxes = ref<BoxDefinition[]>([]), next = ref(''), query = ref('')
const name = ref(''), locale = ref(''), source = ref(''), fallback = ref(''), baseVersion = ref(0)
const current = ref<BoxDefinition | null>(null), creating = ref(false), dirty = ref(false)
const loading = ref(false), reading = ref(false), busy = ref(false), ready = ref(false)
const error = ref(''), notice = ref(''), preview = ref(''), copyFrom = ref('')
const unresolved = ref<string[]>([]), historyVersion = ref(1), historical = ref<BoxEntry | null>(null)
const listReads = new LatestRequest(), detailReads = new LatestRequest(), previews = new LatestRequest()
let timer = 0, previewTimer = 0, alive = true, suppressDirty = false
const editions = computed(() => languageEditions.value.filter((e) => e.status === 'approved' || e.status === 'active'))
const syntax = computed(() => boxSyntax(current.value?.name || name.value))
const selectedName = computed(() => current.value?.name || name.value.trim().replace(/\s+/gu, ' ').normalize('NFC'))
const canSave = computed(() => ready.value && !reading.value && !busy.value && Boolean(selectedName.value && locale.value && source.value.trim()))
const path = (boxName: string) => `/api/admin/boxes/${encodeURIComponent(boxName)}`
const explain = (reason: unknown) => reason instanceof Error ? reason.message : '박스를 처리하지 못했습니다.'

function leaveDraft() { return !dirty.value || window.confirm('저장하지 않은 박스 입력을 버리고 이동할까요?') }
async function loadList(more = false) {
  loading.value = true
  const params = new URLSearchParams({ q: query.value, ...(more ? { after: next.value } : {}) })
  await listReads.run((signal) => request<BoxPage>(`/api/editor/boxes?${params}`, { signal }), {
    success: (page) => { boxes.value = more ? [...boxes.value, ...page.boxes] : page.boxes; next.value = page.next },
    error: (reason) => { error.value = explain(reason) }, settled: () => { loading.value = false },
  })
}
function applyEntry(entry: BoxEntry) {
  suppressDirty = true
  current.value = entry; name.value = entry.name; source.value = entry.content.source || ''
  locale.value = entry.content.locale; fallback.value = entry.fallback_locale; baseVersion.value = entry.version
  historyVersion.value = entry.version; historical.value = null; unresolved.value = []; copyFrom.value = ''
  creating.value = false; ready.value = true; dirty.value = false
  // Sync watchers mark only subsequent user changes, not the loaded snapshot.
  suppressDirty = false
}
async function openBox(value: BoxDefinition, targetLocale = locale.value) {
  if (busy.value || !leaveDraft()) return
  reading.value = true; ready.value = false; error.value = ''; notice.value = ''
  previews.invalidate(); preview.value = ''; historical.value = null
  await detailReads.run((signal) => request<BoxEntry>(`${path(value.name)}/${targetLocale || value.locales[0]}`, { signal }), {
    success: applyEntry, error: (reason) => { error.value = explain(reason) }, settled: () => { reading.value = false },
  })
}
function newBox() {
  if (busy.value || !leaveDraft()) return
  detailReads.invalidate(); previews.invalidate(); suppressDirty = true
  current.value = null; name.value = ''; source.value = '<wiki-navbox title="">\n- 분류: [[문서]]\n</wiki-navbox>'
  fallback.value = ''; baseVersion.value = 0; historical.value = null; unresolved.value = []; copyFrom.value = ''
  locale.value ||= editions.value[0]?.tag || ''; creating.value = true; reading.value = false
  ready.value = true; dirty.value = false; error.value = ''; notice.value = ''; preview.value = ''
  suppressDirty = false
}
async function changeLocale(event: Event) {
  const target = (event.target as HTMLSelectElement).value
  ;(event.target as HTMLSelectElement).value = locale.value
  if (current.value) await openBox(current.value, target)
  else if (leaveDraft()) { locale.value = target; dirty.value = true }
}
async function save() {
  if (!canSave.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const entry = await request<BoxEntry>(`${path(selectedName.value)}/${locale.value}`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source: source.value, fallback_locale: fallback.value, base_version: baseVersion.value }),
    })
    if (!alive) return
    applyEntry(entry); notice.value = '저장했습니다. 이 박스를 사용하는 문서의 다음 조회부터 반영됩니다.'
    await loadList()
  } catch (reason) {
    if (!alive) return
    error.value = reason instanceof ApiError && reason.status === 409
      ? '편집 충돌입니다. 내 입력은 보존했습니다. 원문을 복사한 뒤 최신 내용 다시 불러오기를 눌러 비교하세요.' : explain(reason)
  } finally { if (alive) busy.value = false }
}
async function copyLanguage() {
  if (!current.value || !copyFrom.value || busy.value || reading.value || !leaveDraft()) return
  busy.value = true; error.value = ''; historical.value = null
  const boxName = current.value.name, target = locale.value
  try {
    const draft = await request<{ source: string; base_version: number; unresolved: string[] }>(`${path(boxName)}/draft/${target}?from=${copyFrom.value}`)
    if (!alive) return
    if (draft.base_version !== baseVersion.value) { error.value = '박스가 변경되었습니다. 최신 내용을 불러온 다음 복사하세요.'; return }
    source.value = draft.source; unresolved.value = draft.unresolved
    notice.value = '저장 전 초안입니다. 표시 문구는 직접 번역하고, 연결하지 못한 항목은 확인하세요.'
  } catch (reason) { if (alive) error.value = explain(reason) }
  finally { if (alive) busy.value = false }
}
async function readHistory() {
  if (!current.value || busy.value || reading.value) return
  busy.value = true; error.value = ''; historical.value = null
  try {
    const entry = await request<BoxEntry>(`${path(current.value.name)}/revisions/${historyVersion.value}`)
    if (alive) historical.value = entry
  } catch (reason) { if (alive) error.value = explain(reason) }
  finally { if (alive) busy.value = false }
}
async function updatePreview() {
  if (!ready.value || !locale.value || !source.value.trim()) { preview.value = ''; return }
  await previews.run((signal) => request<{ html: string }>(`/api/admin/boxes/preview/${locale.value}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ source: source.value }), signal }), {
    success: (value) => { preview.value = value.html }, error: () => { preview.value = '' }, settled: () => {},
  })
}
function beforeLeave(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
watch([name, source, fallback], () => { if (!suppressDirty && ready.value) dirty.value = true }, { flush: 'sync' })
watch([source, locale], () => { previews.invalidate(); window.clearTimeout(previewTimer); previewTimer = window.setTimeout(updatePreview, 250) })
watch(query, () => { listReads.invalidate(); next.value = ''; window.clearTimeout(timer); timer = window.setTimeout(() => void loadList(), 180) })
onMounted(async () => {
  window.addEventListener('beforeunload', beforeLeave)
  try { await loadLanguageEditions(); if (alive) locale.value = editions.value[0]?.tag || '' } catch (reason) { if (alive) error.value = explain(reason) }
  if (alive) await loadList()
})
onBeforeUnmount(() => { alive = false; listReads.dispose(); detailReads.dispose(); previews.dispose(); window.clearTimeout(timer); window.clearTimeout(previewTimer); window.removeEventListener('beforeunload', beforeLeave) })
onBeforeRouteLeave(() => !busy.value && leaveDraft())
</script>

<template>
  <div class="admin-view admin-boxes-view">
    <header class="admin-page-header"><div><p>콘텐츠 자원</p><h1>박스</h1><span>문서와 독립된 공용 표시 블록입니다. 언어별 내용을 같은 이름으로 재사용합니다.</span></div><button type="button" :disabled="busy || !editions.length" @click="newBox">새 박스</button></header>
    <p v-if="error" class="box-message error" role="alert">{{ error }}</p>
    <p v-if="notice" class="box-message" role="status">{{ notice }}</p>
    <div class="box-manager">
      <section class="admin-panel box-library" aria-label="박스 목록">
        <label>박스 검색<input v-model="query" type="search" maxlength="120" placeholder="이름으로 검색"></label>
        <p v-if="loading">불러오는 중…</p><p v-else-if="!boxes.length">등록된 박스가 없습니다.</p>
        <div class="box-resource-list"><button v-for="item in boxes" :key="item.name" type="button" :class="{ selected: current?.name === item.name }" :disabled="busy" @click="openBox(item)"><strong>{{ item.name }}</strong><small>{{ item.locales.join(' · ') }}</small></button></div>
        <button v-if="next" type="button" :disabled="loading" @click="loadList(true)">더 불러오기</button>
      </section>
      <section class="admin-panel box-workspace" :aria-busy="reading">
        <p v-if="reading">박스를 불러오는 중…</p>
        <p v-else-if="!ready">박스를 선택하거나 새 박스를 만드세요.</p>
        <template v-else>
          <fieldset :disabled="busy || reading">
            <div class="box-fields">
              <label>박스 이름<input v-model="name" :readonly="!creating" maxlength="120" required><small>언어별로 동일한 이름을 사용합니다. 저장 후 이름은 고정됩니다.</small></label>
              <label>편집 언어<select :value="locale" @change="changeLocale"><option v-for="e in editions" :key="e.tag" :value="e.tag">{{ e.native_name }} · {{ e.tag }}</option></select></label>
              <label>번역 누락 시 대체 언어<select v-model="fallback"><option value="">대체하지 않음 · 해당 문서에서는 숨김</option><option v-for="tag in [...new Set([...(current?.locales || []), locale])].filter(Boolean)" :key="tag" :value="tag">{{ tag }}</option></select><small>모든 언어에 적용되는 공통 설정입니다.</small></label>
            </div>
            <p v-if="current && !current.locales.includes(locale)" class="box-message">이 언어의 내용이 없습니다. 직접 작성하거나 다른 언어에서 초안을 복사하세요.</p>
            <div v-if="current" class="box-inline-actions"><label>초안 원본 언어<select v-model="copyFrom"><option value="">언어 선택</option><option v-for="tag in current.locales.filter((v) => v !== locale)" :key="tag" :value="tag">{{ tag }}</option></select></label><button type="button" :disabled="!copyFrom" @click="copyLanguage">구조·연결 복사</button><button type="button" @click="openBox(current)">최신 내용 다시 불러오기</button></div>
            <p v-if="unresolved.length" class="box-message">대상 문서가 연결되지 않아 일반 글자로 복사한 항목: {{ unresolved.join(' · ') }}</p>
            <div class="box-edit-preview"><section><h2>TechMark 원문</h2><div class="box-code-editor"><TechMarkEditor v-model="source" :readonly="busy || reading" /></div></section><section><h2>미리보기</h2><article class="techmark-content" :lang="locale" :dir="editions.find((e) => e.tag === locale)?.direction" v-article-html="preview"></article></section></div>
            <p class="box-help">기존 navbox·infobox 문법을 그대로 사용합니다. 중첩 박스·문서 제목·각주·운영 예약 태그는 지원하지 않습니다. 자동 번역은 하지 않습니다.</p>
          </fieldset>
          <footer class="box-inline-actions"><code>{{ syntax }}</code><button type="button" :disabled="!canSave" @click="save">{{ busy ? '처리 중…' : '저장' }}</button><span v-if="dirty">저장하지 않은 변경</span></footer>
          <details v-if="current" class="box-history"><summary>박스 수정 이력 · 현재 {{ baseVersion }}판</summary><div class="box-inline-actions"><label>판 번호<input v-model.number="historyVersion" type="number" min="1" :max="baseVersion"></label><button type="button" :disabled="busy || reading" @click="readHistory">과거 원문 조회</button></div><template v-if="historical"><p>{{ historical.content.locale }} · {{ historical.content.author }} · {{ historical.content.updated_at }}</p><pre>{{ historical.content.source }}</pre><small>과거 기록 조회는 현재 입력이나 저장된 박스를 변경하지 않습니다.</small></template></details>
        </template>
      </section>
    </div>
  </div>
</template>
