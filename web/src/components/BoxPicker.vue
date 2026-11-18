<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { request } from '../services/api'
import { LatestRequest } from '../services/latestRequest'
import { articleHTMLDirective } from '../services/articleHTML'
import { boxAvailability, boxSyntax } from '../services/boxes'
import type { BoxDefinition, BoxPage } from '../types/box'
import './box-picker.css'

const props = defineProps<{ locale: string }>()
const emit = defineEmits<{ close: []; insert: [source: string] }>()
const vArticleHtml = articleHTMLDirective
const query = ref(''), boxes = ref<BoxDefinition[]>([]), next = ref(''), selected = ref<BoxDefinition | null>(null)
const preview = ref(''), error = ref(''), loading = ref(false), previewing = ref(false)
const dialog = ref<HTMLElement | null>(null), searchInput = ref<HTMLInputElement | null>(null)
const reads = new LatestRequest(), previews = new LatestRequest()
const availability = computed(() => selected.value ? boxAvailability(selected.value.locales, selected.value.fallback_locale, props.locale) : null)
let timer = 0, overflow = '', previousFocus: HTMLElement | null = null
async function search(more = false) {
  loading.value = true; error.value = ''
  const params = new URLSearchParams({ q: query.value, ...(more ? { after: next.value } : {}) })
  await reads.run((signal) => request<BoxPage>(`/api/editor/boxes?${params}`, { signal }), {
    success: (value) => { boxes.value = more ? [...boxes.value, ...value.boxes] : value.boxes; next.value = value.next },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '박스를 불러오지 못했습니다.' }, settled: () => { loading.value = false },
  })
}
async function choose(value: BoxDefinition) {
  selected.value = value; preview.value = ''; previewing.value = true; error.value = ''
  await previews.run((signal) => request<{ html: string }>(`/api/editor/boxes/${encodeURIComponent(value.name)}/preview/${props.locale}`, { signal }), {
    success: (value) => { preview.value = value.html }, error: (reason) => { error.value = reason instanceof Error ? reason.message : '미리보기를 불러오지 못했습니다.' }, settled: () => { previewing.value = false },
  })
}
function insert() { if (selected.value && availability.value?.available && !previewing.value && preview.value.trim() && !error.value) emit('insert', boxSyntax(selected.value.name)) }
function keyboard(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); emit('close'); return }
  if (event.key !== 'Tab' || !dialog.value) return
  const controls = [...dialog.value.querySelectorAll<HTMLElement>('button:not(:disabled), input, a[href], [tabindex="0"]')].filter((e) => e.getClientRects().length)
  const first = controls[0], last = controls.at(-1)
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
watch(query, () => { reads.invalidate(); next.value = ''; selected.value = null; previews.invalidate(); preview.value = ''; window.clearTimeout(timer); timer = window.setTimeout(() => void search(), 180) })
watch(() => props.locale, () => { selected.value = null; previews.invalidate(); preview.value = '' })
onMounted(async () => { previousFocus = document.activeElement as HTMLElement | null; overflow = document.body.style.overflow; document.body.style.overflow = 'hidden'; window.addEventListener('keydown', keyboard); await nextTick(); searchInput.value?.focus(); await search() })
onBeforeUnmount(() => { reads.dispose(); previews.dispose(); window.clearTimeout(timer); window.removeEventListener('keydown', keyboard); document.body.style.overflow = overflow; previousFocus?.focus() })
</script>

<template>
  <Teleport to="body">
    <div class="box-picker-backdrop" @mousedown.self="emit('close')">
      <section ref="dialog" class="box-picker" role="dialog" aria-modal="true" aria-labelledby="box-picker-title">
        <header><h2 id="box-picker-title">박스 삽입</h2><button type="button" aria-label="닫기" @click="emit('close')">×</button></header>
        <p v-if="error" class="box-message error" role="alert">{{ error }}</p>
        <div class="box-picker-body">
          <section class="box-picker-library"><label>박스 검색<input ref="searchInput" v-model="query" type="search" maxlength="120" placeholder="이름으로 검색"></label><div class="box-resource-list"><p v-if="loading">불러오는 중…</p><p v-else-if="!boxes.length">검색 결과가 없습니다.</p><button v-for="item in boxes" :key="item.name" type="button" :class="{ selected: selected?.name === item.name }" @click="choose(item)"><strong>{{ item.name }}</strong><small>{{ item.locales.join(' · ') }}</small></button><button v-if="next" type="button" :disabled="loading" @click="search(true)">더 불러오기</button></div></section>
          <section class="box-picker-preview"><p v-if="!selected">박스를 선택하면 현재 문서 언어의 내용을 보여줍니다.</p><template v-else><h3>{{ selected.name }}</h3><p v-if="!availability?.available">{{ locale }} 내용과 지정된 대체 언어가 없습니다. 관리자에게 언어별 내용을 요청하세요.</p><p v-else-if="availability.fallback">{{ locale }} 내용이 없어 관리자가 지정한 {{ availability.locale }} 내용으로 표시됩니다. 문서 링크도 해당 언어로 연결됩니다.</p><p v-if="previewing">미리보기 불러오는 중…</p><p v-else-if="availability?.available && !preview.trim() && !error">현재 공개할 수 있는 내용이 없습니다. 대체 언어판의 활성 상태와 박스 내용을 확인하세요.</p><article class="techmark-content" v-article-html="preview"></article></template></section>
        </div>
        <footer><small>원문 복사가 아닌 참조를 삽입합니다. 박스 수정 사항은 사용하는 문서에도 반영됩니다.</small><button type="button" :disabled="!selected || !availability?.available || previewing || !preview.trim() || Boolean(error)" @click="insert">박스 삽입</button></footer>
      </section>
    </div>
  </Teleport>
</template>
