<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Box, ImagePlus } from '@lucide/vue'
import MediaPicker from '../components/MediaPicker.vue'
import TechMarkEditor from '../components/TechMarkEditor.vue'
import { articleHTMLDirective } from '../services/articleHTML'
import { ApiError, deleteEditorDraft, getAdminPage, getEditorDraft, moveAdminPage, previewTechMark, saveAdminPage, saveEditorDraft } from '../services/api'
import { hasPermission } from '../services/auth'
import { documentURL, useWikiI18n } from '../i18n'
import type { AdminPageResponse, DocumentEditPolicy, EditorDraft } from '../types/api'

const vArticleHtml = articleHTMLDirective
const BoxPicker = defineAsyncComponent(() => import('../components/BoxPicker.vue'))
const boxPickerOpen = ref(false)

const route = useRoute()
const router = useRouter()
const { locale, uiLocale, mainPageName, t } = useWikiI18n()
const routeName = computed(() => {
  const value = route.params.name
  return (Array.isArray(value) ? value.join('/') : value).replaceAll('_', ' ')
})
const translationSource = computed(() => {
  const rawLocale = route.query.translation_from_locale
  const rawName = route.query.translation_from_name
  if (typeof rawLocale !== 'string' || typeof rawName !== 'string' || !rawName.trim()) return undefined
  try {
    const sourceLocale = Intl.getCanonicalLocales(rawLocale)[0]
    if (!sourceLocale || sourceLocale === locale.value) return undefined
    return { locale: sourceLocale, name: rawName.trim() }
  } catch {
    return undefined
  }
})
const name = ref(routeName.value)
const source = ref('')
const summary = ref('')
const editPolicy = ref<DocumentEditPolicy>('editors')
const previewHTML = ref('')
const loading = ref(true)
const saving = ref(false)
const moving = ref(false)
const moveName = ref('')
const pageExists = ref(false)
const baseRevision = ref('')
const pendingDraft = ref<EditorDraft | null>(null)
const conflict = ref<AdminPageResponse | null>(null)
const draftStatus = ref('')
const error = ref('')
const status = ref('')
const mediaPickerOpen = ref(false)
const techMarkEditor = ref<InstanceType<typeof TechMarkEditor> | null>(null)
const cancelURL = computed(() => translationSource.value
  ? documentURL('wiki', translationSource.value.locale, translationSource.value.name)
  : documentURL('wiki', locale.value, routeName.value))
const translationConflict = computed(() => Boolean(translationSource.value && pageExists.value))
let previewTimer = 0
let draftTimer = 0
let previewController: AbortController | null = null
let draftReady = false

const permissionCopy = computed(() => uiLocale.value === 'ko'
  ? {
      label: '편집 권한', editors: '편집 권한이 있는 사용자', admins: '관리자만 편집',
      note: '관리자 전용 문서는 일반 편집자가 원문을 열거나 저장할 수 없습니다.',
    }
  : {
      label: 'Editing permission', editors: 'Users with edit permission', admins: 'Administrators only',
      note: 'Regular editors cannot open or save the source of administrator-only articles.',
    })

async function load() {
  loading.value = true
  draftReady = false
  error.value = ''
  summary.value = ''
  draftStatus.value = ''
  conflict.value = null
  pendingDraft.value = null
  try {
    const [page, draft] = await Promise.all([
      getAdminPage(locale.value, routeName.value),
      getEditorDraft(locale.value, routeName.value).catch(() => null),
    ])
    name.value = page.name || routeName.value
    moveName.value = page.name || routeName.value
    source.value = page.source
    editPolicy.value = page.edit_policy
    baseRevision.value = page.revision
    pageExists.value = Boolean(page.updated_at)
    if (translationSource.value && !pageExists.value && !source.value) source.value = `# ${name.value}\n`
    if (translationConflict.value) error.value = '선택한 이름의 문서가 이미 존재합니다.'
    if (draft && (draft.source !== source.value || draft.summary !== summary.value)) pendingDraft.value = draft
    await nextTick()
    draftReady = true
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : t('sourceError')
  } finally {
    loading.value = false
  }
}

async function persistDraft() {
  if (!draftReady || pendingDraft.value || saving.value) return
  draftStatus.value = '초안 저장 중'
  try {
    const draft = await saveEditorDraft(locale.value, routeName.value, {
      name: name.value, source: source.value, summary: summary.value, base_revision: baseRevision.value,
    })
    draftStatus.value = `초안 저장됨 · ${new Date(draft.updated_at).toLocaleTimeString()}`
  } catch {
    draftStatus.value = '초안을 저장하지 못했습니다.'
  }
}

function restoreDraft() {
  if (!pendingDraft.value) return
  name.value = pendingDraft.value.name || name.value
  source.value = pendingDraft.value.source
  summary.value = pendingDraft.value.summary || ''
  baseRevision.value = pendingDraft.value.base_revision
  pendingDraft.value = null
  draftStatus.value = '서버 초안을 복구했습니다.'
}

async function discardDraft() {
  try {
    await deleteEditorDraft(locale.value, routeName.value)
    pendingDraft.value = null
    draftStatus.value = '초안을 폐기했습니다.'
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '초안을 폐기하지 못했습니다.'
  }
}

function retryAfterConflict() {
  if (!conflict.value) return
  baseRevision.value = conflict.value.revision
  conflict.value = null
  status.value = '최신 리비전을 기준으로 다시 저장할 수 있습니다. 원문을 검토하세요.'
  window.clearTimeout(draftTimer)
  draftTimer = window.setTimeout(() => void persistDraft(), 0)
}

function loadConflictSource() {
  if (!conflict.value) return
  source.value = conflict.value.source
  editPolicy.value = conflict.value.edit_policy
  baseRevision.value = conflict.value.revision
  conflict.value = null
  status.value = '최신 원문을 불러왔습니다.'
}

async function moveDocument() {
  const target = moveName.value.trim()
  if (!target || target === name.value || !window.confirm(`문서 이름을 '${target}'(으)로 변경하시겠습니까? 이전 주소는 새 문서로 연결됩니다.`)) return
  moving.value = true
  error.value = ''
  try {
    const page = await moveAdminPage(locale.value, routeName.value, target)
    await router.replace(documentURL('edit', page.locale, page.name))
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '문서 이름을 변경하지 못했습니다.'
  } finally {
    moving.value = false
  }
}

async function updatePreview() {
  previewController?.abort()
  previewController = new AbortController()
  try {
    const preview = await previewTechMark(locale.value, name.value, source.value, previewController.signal)
    previewHTML.value = preview.html
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    status.value = t('previewError')
  }
}

async function save() {
  if (translationConflict.value) return
  saving.value = true
  error.value = ''
  status.value = ''
  try {
    const page = await saveAdminPage(locale.value, routeName.value, {
      name: name.value,
      source: source.value,
      edit_policy: editPolicy.value,
      base_revision: baseRevision.value,
    }, summary.value, translationSource.value)
    draftReady = false
    try { await deleteEditorDraft(locale.value, routeName.value) } catch { /* The saved revision remains authoritative. */ }
    status.value = t('saved')
    await router.push(documentURL('wiki', page.locale, page.name))
  } catch (reason) {
    if (reason instanceof ApiError && reason.status === 409 && reason.details && typeof reason.details === 'object' && 'current' in reason.details) {
      conflict.value = (reason.details as { current: AdminPageResponse }).current
      error.value = '다른 사용자가 먼저 저장했습니다. 두 원문을 비교한 뒤 다시 시도하세요.'
      return
    }
    error.value = reason instanceof Error ? reason.message : t('saveError')
  } finally {
    saving.value = false
  }
}

function insertImage(markup: string) {
  techMarkEditor.value?.insert(markup)
  mediaPickerOpen.value = false
}

function insertBox(markup: string) {
  techMarkEditor.value?.insert(markup)
  boxPickerOpen.value = false
}

watch(source, () => {
  window.clearTimeout(previewTimer)
  previewTimer = window.setTimeout(updatePreview, 220)
})

watch([source, summary], () => {
  if (!draftReady) return
  window.clearTimeout(draftTimer)
  draftTimer = window.setTimeout(() => void persistDraft(), 1200)
})

watch([locale, routeName], load, { immediate: true })
onBeforeUnmount(() => {
  window.clearTimeout(previewTimer)
  window.clearTimeout(draftTimer)
  previewController?.abort()
})
</script>

<template>
  <div class="editor-view">
    <header class="editor-header">
      <div>
        <p>{{ t('editor') }}</p>
        <h1>{{ name }}</h1>
      </div>
      <div class="editor-actions">
        <RouterLink :to="cancelURL">{{ t('cancel') }}</RouterLink>
        <button type="button" :disabled="saving || loading || translationConflict" @click="save">{{ saving ? t('saving') : t('save') }}</button>
      </div>
    </header>

    <div v-if="loading" class="page-state">{{ t('loadingSource') }}</div>
    <template v-else>
      <section v-if="pendingDraft" class="editor-draft-recovery">
        <div><strong>저장되지 않은 서버 초안</strong><span>{{ new Date(pendingDraft.updated_at).toLocaleString() }}에 저장된 초안이 있습니다.</span></div>
        <button type="button" @click="restoreDraft">초안 복구</button><button type="button" @click="discardDraft">초안 폐기</button>
      </section>
      <p v-if="translationSource" class="editor-translation-source"><span>{{ translationSource.locale }}</span><strong>{{ translationSource.name }}</strong><span>→</span><strong>{{ locale }} · {{ name }}</strong></p>
      <div class="editor-meta" :class="{ 'has-permission': hasPermission('admin') }">
        <label>{{ t('documentName') }}<input v-model="name" type="text" readonly></label>
        <label>{{ t('editSummary') }}<input v-model="summary" type="text" maxlength="200" :placeholder="t('summaryPlaceholder')"></label>
        <label v-if="hasPermission('admin')" class="editor-permission">
          {{ permissionCopy.label }}
          <select v-model="editPolicy">
            <option value="editors">{{ permissionCopy.editors }}</option>
            <option value="admins">{{ permissionCopy.admins }}</option>
          </select>
          <small>{{ permissionCopy.note }}</small>
        </label>
      </div>
      <section v-if="hasPermission('admin') && pageExists && name !== mainPageName" class="editor-move">
        <div><strong>문서 이름 변경</strong><small>전체 역사와 토론을 유지하고 이전 주소에는 영구 이동을 남깁니다. 관리자만 실행할 수 있습니다.</small></div>
        <input v-model="moveName" type="text" maxlength="200" aria-label="새 문서 이름">
        <button type="button" :disabled="moving || moveName.trim() === name" @click="moveDocument">{{ moving ? '변경 중' : '이름 변경' }}</button>
      </section>
      <p v-if="error" class="editor-message error">{{ error }}</p>
      <p v-else-if="status" class="editor-message">{{ status }}</p>
      <p v-if="draftStatus" class="editor-draft-status">{{ draftStatus }}</p>

      <section v-if="conflict" class="editor-conflict">
        <header><div><strong>편집 충돌</strong><span>현재 서버 리비전 {{ conflict.revision }}</span></div><div><button type="button" @click="retryAfterConflict">내 원문 유지</button><button type="button" @click="loadConflictSource">최신 원문 사용</button></div></header>
        <div><section><h2>내 원문</h2><pre>{{ source }}</pre></section><section><h2>최신 원문</h2><pre>{{ conflict.source }}</pre></section></div>
      </section>

      <div class="editor-workspace">
        <section class="editor-pane">
          <header><h2>{{ t('source') }}</h2><div class="editor-pane-tools"><span>{{ t('autocomplete') }}</span><button type="button" title="박스 삽입" aria-label="박스 삽입" @click="boxPickerOpen = true"><Box :size="17" /></button><button type="button" title="이미지 삽입" aria-label="이미지 삽입" @click="mediaPickerOpen = true"><ImagePlus :size="17" /></button></div></header>
          <TechMarkEditor ref="techMarkEditor" v-model="source" />
        </section>
        <section class="preview-pane">
          <header><h2>{{ t('livePreview') }}</h2><span>{{ t('serverRendering') }}</span></header>
          <article class="techmark-content" v-article-html="previewHTML"></article>
        </section>
      </div>
      <MediaPicker v-if="mediaPickerOpen" @close="mediaPickerOpen = false" @insert="insertImage" />
      <BoxPicker v-if="boxPickerOpen" :locale="locale" @close="boxPickerOpen = false" @insert="insertBox" />
    </template>
  </div>
</template>
