<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { RotateCcw } from '@lucide/vue'
import { getAdminTrash, restoreAdminTrash } from '../../services/api'
import { LatestRequest } from '../../services/latestRequest'
import type { TrashItem } from '../../types/api'

const items = ref<TrashItem[]>([])
const loading = ref(true)
const busy = ref(new Set<string>())
const error = ref('')
const requests = new LatestRequest()
let mounted = true

async function load() {
  loading.value = true
  error.value = ''
  await requests.run((signal) => getAdminTrash(signal), {
    success: (value) => { items.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '휴지통을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

async function restore(item: TrashItem) {
  if (busy.value.has(item.wid)) return
  if (!window.confirm(`'${item.name}' 문서를 같은 WID와 리비전으로 복구하시겠습니까?`)) return
  busy.value.add(item.wid)
  try { await restoreAdminTrash(item.wid, '관리자 휴지통 복구'); if (mounted) await load() }
  catch (reason) { if (mounted) error.value = reason instanceof Error ? reason.message : '문서를 복구하지 못했습니다.' }
  finally { if (mounted) busy.value.delete(item.wid) }
}

onMounted(load)
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header"><div><p>콘텐츠</p><h1>문서 휴지통</h1></div><span>{{ items.length }}개</span></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaButton v-if="error" preset="secondary" :disabled="loading" @click="load">다시 불러오기</VaButton>
    <VaCard class="admin-table-card">
      <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 불러오는 중입니다.</div>
      <div v-else class="admin-table admin-trash-table">
        <div class="admin-table-row head"><span>문서</span><span>언어</span><span>리비전</span><span>삭제자</span><span>작업</span></div>
        <div v-for="item in items" :key="item.wid" class="admin-table-row">
          <span><b>{{ item.name }}</b><small>{{ item.reason || '삭제 사유 없음' }} · {{ new Date(item.deleted_at).toLocaleString() }}</small><code>{{ item.wid }}</code></span>
          <span>{{ item.locale }}</span><span>{{ item.revision }}</span><span>{{ item.deleted_by }}</span><span><VaButton size="small" :disabled="busy.has(item.wid)" :loading="busy.has(item.wid)" @click="restore(item)"><RotateCcw :size="15" />복구</VaButton></span>
        </div>
        <p v-if="!items.length" class="admin-empty">삭제된 문서가 없습니다.</p>
      </div>
    </VaCard>
  </div>
</template>
