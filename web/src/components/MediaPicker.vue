<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Image, Link, Search, Upload, X } from '@lucide/vue'
import { deleteMedia, searchMedia, uploadMedia } from '../services/api'
import { hasPermission } from '../services/auth'
import type { MediaFile } from '../types/api'

const emit = defineEmits<{ close: []; insert: [source: string] }>()
type Tab = 'library' | 'upload' | 'external'
type Layout = 'frame' | 'side' | 'wide'

const tab = ref<Tab>('library')
const query = ref('')
const files = ref<MediaFile[]>([])
const selected = ref<MediaFile | null>(null)
const loading = ref(false)
const uploading = ref(false)
const deleting = ref(false)
const notice = ref('')
const error = ref('')
const usage = ref(0)
const limit = ref(0)
const uploadInput = ref<HTMLInputElement | null>(null)
const uploadFile = ref<File | null>(null)
const uploadName = ref('')
const author = ref('')
const license = ref('')
const sourceURL = ref('')
const externalURL = ref('')
const alt = ref('')
const caption = ref('')
const layout = ref<Layout>('frame')
let searchTimer = 0
let searchController: AbortController | null = null
let previousOverflow = ''
let alive = true
let searchGeneration = 0

const storageLabel = computed(() => limit.value > 0
  ? `${formatBytes(usage.value)} / ${formatBytes(limit.value)}`
  : '')

async function loadMedia() {
  const generation = ++searchGeneration
  searchController?.abort()
  searchController = new AbortController()
  loading.value = true
  error.value = ''
  try {
    const result = await searchMedia(query.value.trim(), searchController.signal)
    if (!alive || generation !== searchGeneration) return
    files.value = result.files
    usage.value = result.usage_bytes
    limit.value = result.limit_bytes
    if (selected.value && !files.value.some((file) => file.name === selected.value?.name)) selected.value = null
  } catch (reason) {
    if (!alive || generation !== searchGeneration) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof Error ? reason.message : '파일을 검색하지 못했습니다.'
  } finally {
    if (alive && generation === searchGeneration) loading.value = false
  }
}

async function removeUploaded() {
  const file = selected.value
  if (!hasPermission('owner') || deleting.value || uploading.value || !file) return
  const confirmation = window.prompt(`업로드 파일 ${file.name}을 영구 삭제합니다. 복구할 수 없고 삭제한 파일 이름은 재사용할 수 없습니다. 현재 문서에서 사용 중이면 삭제되지 않습니다.\n계속하려면 정확한 파일 이름을 입력하세요.`, '')
  if (confirmation !== file.name) return
  deleting.value = true; error.value = ''; notice.value = ''
  window.clearTimeout(searchTimer)
  searchController?.abort(); searchGeneration++; loading.value = false
  try {
    const result = await deleteMedia(file.name, confirmation)
    if (!alive) return
    files.value = files.value.filter((item) => item.name !== file.name)
    if (selected.value?.name === file.name) selected.value = null
    await loadMedia()
    if (!alive) return
    notice.value = result.cleanup_pending ? '파일 접근은 차단했지만 디스크 정리가 완료되지 않았습니다. 서버 저장소를 확인하세요.' : '업로드 파일을 삭제했습니다. 외부 캐시의 사본은 남아 있을 수 있습니다.'
  } catch (reason) {
    if (alive) error.value = reason instanceof Error ? reason.message : '파일을 삭제하지 못했습니다.'
  } finally { if (alive) deleting.value = false }
}

function chooseUpload(event: Event) {
  const input = event.target as HTMLInputElement
  uploadFile.value = input.files?.[0] ?? null
  if (!uploadFile.value) return
  uploadName.value = `${uploadFile.value.name.replace(/\.[^.]+$/, '').trim() || 'image'}.webp`
}

async function submitUpload() {
  if (!uploadFile.value) {
    error.value = '업로드할 이미지를 선택하세요.'
    return
  }
  uploading.value = true
  error.value = ''
  try {
    const data = new FormData()
    data.set('file', uploadFile.value)
    data.set('name', uploadName.value)
    data.set('author', author.value)
    data.set('license', license.value)
    data.set('source', sourceURL.value)
    const uploaded = await uploadMedia(data)
    query.value = ''
    await loadMedia()
    selected.value = files.value.find((file) => file.name === uploaded.name) ?? uploaded
    tab.value = 'library'
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '파일을 업로드하지 못했습니다.'
  } finally {
    uploading.value = false
  }
}

function insertUploaded() {
  if (!selected.value) return
  emit('insert', imageSyntax('file', selected.value.name))
}

function insertExternal() {
  try {
    const parsed = new URL(externalURL.value.trim())
    if (parsed.protocol !== 'https:') throw new Error('HTTPS 주소만 사용할 수 있습니다.')
    emit('insert', imageSyntax('src', parsed.toString()))
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '올바른 외부 이미지 주소를 입력하세요.'
  }
}

function imageSyntax(attribute: 'file' | 'src', value: string): string {
  const safeValue = attributeValue(attribute === 'file' ? value.replaceAll(',', '\\,') : value)
  const safeAlt = attributeValue(alt.value.trim())
  return `<wiki-image ${attribute}="${safeValue}" layout="${layout.value}" alt="${safeAlt}">${caption.value.trim()}</wiki-image>`
}

function attributeValue(value: string): string {
  return value.replaceAll('"', "'").replace(/[<>\r\n]/g, '')
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('close')
}

watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => void loadMedia(), 180)
})
onMounted(() => {
  previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', handleKeydown)
  void loadMedia()
})
onBeforeUnmount(() => {
  alive = false; searchGeneration++
  document.body.style.overflow = previousOverflow
  window.clearTimeout(searchTimer)
  searchController?.abort()
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <Teleport to="body">
  <div class="media-picker-backdrop" @mousedown.self="emit('close')">
    <section class="media-picker" role="dialog" aria-modal="true" aria-labelledby="media-picker-title">
      <header class="media-picker-header">
        <div><h2 id="media-picker-title">이미지 삽입</h2><span v-if="storageLabel">{{ storageLabel }}</span></div>
        <button type="button" aria-label="닫기" title="닫기" @click="emit('close')"><X :size="18" /></button>
      </header>
      <nav class="media-picker-tabs" aria-label="이미지 소스">
        <button type="button" :disabled="deleting" :class="{ active: tab === 'library' }" @click="tab = 'library'"><Image :size="16" />파일 검색</button>
        <button type="button" :disabled="deleting" :class="{ active: tab === 'upload' }" @click="tab = 'upload'"><Upload :size="16" />업로드</button>
        <button type="button" :disabled="deleting" :class="{ active: tab === 'external' }" @click="tab = 'external'"><Link :size="16" />외부 이미지</button>
      </nav>

      <div class="media-picker-body">
        <p v-if="error" class="media-picker-error">{{ error }}</p>
        <p v-if="notice" class="media-picker-notice" role="status">{{ notice }}</p>

        <div v-if="tab === 'library'" class="media-library">
          <div class="media-library-list">
            <label class="media-search"><Search :size="16" /><input v-model="query" :disabled="deleting" type="search" placeholder="전역 파일 이름 검색" autofocus></label>
            <div class="media-results" role="listbox" aria-label="업로드된 이미지">
              <p v-if="loading" class="media-empty">검색 중...</p>
              <p v-else-if="files.length === 0" class="media-empty">일치하는 파일이 없습니다.</p>
              <template v-else>
                <button
                  v-for="file in files"
                  :key="file.name"
                  type="button"
                  :class="{ selected: selected?.name === file.name }"
                  role="option"
                  :disabled="deleting"
                  :aria-selected="selected?.name === file.name"
                  @mouseenter="!deleting && (selected = file)"
                  @focus="!deleting && (selected = file)"
                  @click="!deleting && (selected = file)"
                >
                  <img :src="file.thumbnail_url" alt="" loading="lazy" decoding="async">
                  <span><strong>{{ file.name }}</strong><small>{{ file.width }} x {{ file.height }} · {{ formatBytes(file.size_bytes) }}</small></span>
                </button>
              </template>
            </div>
          </div>
          <div class="media-library-preview">
            <template v-if="selected">
              <img :src="selected.preview_url" :alt="alt" decoding="async">
              <strong>{{ selected.name }}</strong>
              <small>{{ selected.original_name || selected.name }} · {{ selected.uploaded_by }}</small>
              <button v-if="hasPermission('owner')" class="media-file-delete" type="button" :disabled="deleting || uploading" @click="removeUploaded">{{ deleting ? '삭제 중…' : '업로드 파일 삭제' }}</button>
            </template>
            <p v-else>파일에 마우스를 올리거나 선택하면 미리보기가 표시됩니다.</p>
          </div>
        </div>

        <form v-else-if="tab === 'upload'" id="media-picker-upload-form" class="media-upload-form" @submit.prevent="submitUpload">
          <label class="media-file-select">원본 이미지<input ref="uploadInput" type="file" accept="image/jpeg,image/png,image/webp" required @change="chooseUpload"><span>{{ uploadFile?.name || 'JPEG, PNG, WebP · 최대 10 MiB' }}</span></label>
          <label>전역 파일 이름<input v-model="uploadName" type="text" maxlength="180" required placeholder="Linux_tree.webp"></label>
          <div class="media-form-row"><label>저작자<input v-model="author" type="text" maxlength="160"></label><label>라이선스<input v-model="license" type="text" maxlength="120"></label></div>
          <label>출처 URL<input v-model="sourceURL" type="url" maxlength="1000" placeholder="https://"></label>
        </form>

        <form v-else id="media-picker-external-form" class="media-upload-form" @submit.prevent="insertExternal">
          <label>HTTPS 이미지 주소<input v-model="externalURL" type="url" required placeholder="https://static.example.com/image.png"></label>
          <p>파일을 TechPedia에 복사하지 않고 원격 주소를 사용하며, 문서에 출처 호스트가 자동 표시됩니다.</p>
        </form>
      </div>

      <footer class="media-picker-footer">
        <div class="media-picker-options">
          <label>대체 텍스트<input v-model="alt" type="text" maxlength="500" placeholder="이미지를 볼 수 없을 때 전달할 설명"></label>
          <label>캡션<input v-model="caption" type="text" maxlength="500" placeholder="문서에 표시할 설명"></label>
          <label>배치<select v-model="layout"><option value="frame">본문</option><option value="side">측면</option><option value="wide">전체 너비</option></select></label>
        </div>
        <div class="media-picker-actions">
          <button v-if="tab === 'library'" type="button" :disabled="!selected || deleting" @click="insertUploaded">선택한 이미지 삽입</button>
          <button v-else-if="tab === 'upload'" type="submit" form="media-picker-upload-form" :disabled="uploading">{{ uploading ? '압축 및 저장 중...' : 'WebP로 업로드' }}</button>
          <button v-else type="submit" form="media-picker-external-form">외부 이미지 삽입</button>
        </div>
      </footer>
    </section>
  </div>
  </Teleport>
</template>
