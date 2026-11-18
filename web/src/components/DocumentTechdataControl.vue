<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Database, Link2, Search, X } from '@lucide/vue'
import { getAdminDocumentSummary, getTechdataItem, getTechdataItems, linkPageTechdata, techdataDuplicateResponse, unlinkPageTechdata } from '../services/api'
import type { AdminDocumentSummary, TechdataItemResponse } from '../types/api'

const props = defineProps<{ locale: string; name: string }>()
const emit = defineEmits<{ open: []; changed: [] }>()
const dialogOpen = ref(false)
const page = ref<AdminDocumentSummary | null>(null)
const currentItem = ref<TechdataItemResponse | null>(null)
const items = ref<TechdataItemResponse[]>([])
const query = ref('')
const loading = ref(false)
const itemLoading = ref(false)
const busy = ref(false)
const error = ref('')
const duplicateBlocked = ref(false)
let searchTimer = 0
let searchController: AbortController | null = null

function itemLabel(item: TechdataItemResponse): string {
  return item.labels[props.locale] || Object.values(item.labels).find(Boolean) || item.id
}

function itemDescription(item: TechdataItemResponse): string {
  return item.descriptions[props.locale] || Object.values(item.descriptions).find(Boolean) || ''
}

async function openDialog() {
  emit('open')
  dialogOpen.value = true
  page.value = null
  currentItem.value = null
  items.value = []
  query.value = ''
  error.value = ''
  duplicateBlocked.value = false
  loading.value = true
  try {
    const summary = await getAdminDocumentSummary(props.locale, props.name)
    page.value = summary
    if (summary.tid) currentItem.value = await getTechdataItem(summary.tid)
    else await loadItems()
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '현재 문서의 Techdata 상태를 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

function closeDialog() {
  if (busy.value) return
  dialogOpen.value = false
  searchController?.abort()
}

async function loadItems() {
  if (!dialogOpen.value || page.value?.tid) return
  searchController?.abort()
  const controller = new AbortController()
  searchController = controller
  itemLoading.value = true
  error.value = ''
  try {
    items.value = await getTechdataItems(query.value, controller.signal)
  } catch (reason) {
    if (!controller.signal.aborted) error.value = reason instanceof Error ? reason.message : 'Techdata 항목을 검색하지 못했습니다.'
  } finally {
    if (searchController === controller) {
      searchController = null
      itemLoading.value = false
    }
  }
}

async function connect(tid = '', confirmDistinct = false) {
  if (!page.value) return
  busy.value = true
  error.value = ''
  try {
    const item = await linkPageTechdata(page.value.locale, page.value.name, tid, confirmDistinct)
    page.value = { ...page.value, tid: item.id }
    currentItem.value = item
    items.value = []
    emit('changed')
  } catch (reason) {
    const duplicate = techdataDuplicateResponse(reason)
    if (duplicate) {
      items.value = duplicate.candidates
      duplicateBlocked.value = true
      error.value = '같은 이름의 Techdata 항목이 있습니다. 기존 항목을 확인한 뒤 연결하세요.'
    } else {
      error.value = reason instanceof Error ? reason.message : 'Techdata 항목을 연결하지 못했습니다.'
    }
  } finally {
    busy.value = false
  }
}

async function unlink() {
  if (!page.value?.tid) return
  busy.value = true
  error.value = ''
  try {
    await unlinkPageTechdata(page.value.locale, page.value.name)
    page.value = { ...page.value, tid: undefined }
    currentItem.value = null
    await loadItems()
    emit('changed')
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : 'Techdata 연결을 해제하지 못했습니다.'
  } finally {
    busy.value = false
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && dialogOpen.value) closeDialog()
}

watch(query, () => {
  duplicateBlocked.value = false
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => void loadItems(), 220)
})

onMounted(() => window.addEventListener('keydown', handleKeydown))
onBeforeUnmount(() => {
  window.clearTimeout(searchTimer)
  searchController?.abort()
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <button class="language-techdata-link" type="button" @click="openDialog">
    <span><Database :size="16" aria-hidden="true" /></span><strong>Techdata 연결 관리</strong>
  </button>

  <Teleport to="body">
    <div v-if="dialogOpen" class="techdata-quick-backdrop" @mousedown.self="closeDialog">
      <section class="techdata-quick-dialog" role="dialog" aria-modal="true" aria-labelledby="techdata-quick-title">
        <header>
          <div><p>관리자 도구</p><h2 id="techdata-quick-title">Techdata 연결</h2><span>{{ locale }} · {{ name }}</span></div>
          <button type="button" title="닫기" aria-label="닫기" @click="closeDialog"><X :size="20" /></button>
        </header>

        <p v-if="error" class="techdata-quick-error">{{ error }}</p>
        <p v-if="loading" class="techdata-quick-loading">연결 상태를 확인하는 중입니다.</p>

        <template v-else-if="page">
          <section v-if="page.tid" class="techdata-quick-current">
            <Database :size="22" aria-hidden="true" />
            <div><strong>{{ currentItem ? itemLabel(currentItem) : page.tid }}</strong><span>Techdata {{ page.tid }}에 연결됨</span></div>
            <button type="button" :disabled="busy" @click="unlink">연결 해제</button>
            <div v-if="currentItem" class="techdata-quick-languages">
              <span v-for="(_, itemLocale) in currentItem.wiki_pages" :key="itemLocale"><b>{{ itemLocale }}</b>{{ currentItem.labels[itemLocale] || '이름 없음' }}</span>
            </div>
          </section>

          <template v-else>
            <section class="techdata-quick-create">
              <div><Link2 :size="19" aria-hidden="true" /><span><strong>새 Techdata 항목</strong><small>{{ locale }} 이름을 “{{ name }}”으로 생성하고 연결합니다.</small></span></div>
              <button type="button" :disabled="busy" @click="connect()">새 항목 생성</button>
            </section>

            <div v-if="duplicateBlocked" class="techdata-duplicate-warning">
              <strong>중복 후보 {{ items.length }}개를 찾았습니다.</strong>
              <p>아래 항목이 같은 대상이면 기존 항목을 연결하세요. 실제로 다른 대상인 경우에만 별도 항목으로 생성할 수 있습니다.</p>
              <button type="button" :disabled="busy" @click="connect('', true)">다른 대상임을 확인하고 생성</button>
            </div>

            <div class="techdata-quick-divider"><span>또는 기존 항목 연결</span></div>
            <label class="techdata-quick-search"><Search :size="17" aria-hidden="true" /><span class="sr-only">Techdata 검색</span><input v-model="query" type="search" placeholder="TID, 이름, 설명, 별칭 검색" autofocus></label>
            <p v-if="itemLoading" class="techdata-quick-loading">항목을 검색하는 중입니다.</p>
            <div v-else class="techdata-quick-results">
              <button v-for="item in items" :key="item.id" type="button" :disabled="Boolean(item.wiki_pages[locale]) || busy" @click="connect(item.id)">
                <span><strong>{{ itemLabel(item) }}</strong><small>{{ itemDescription(item) || '설명 없음' }}</small></span>
                <span><code>{{ item.id }}</code><small v-if="item.wiki_pages[locale]">{{ locale }} 문서 연결됨</small><small v-else>{{ Object.keys(item.wiki_pages).length }}개 언어</small></span>
              </button>
              <p v-if="!items.length">검색 결과가 없습니다.</p>
            </div>
          </template>
        </template>
      </section>
    </div>
  </Teleport>
</template>
