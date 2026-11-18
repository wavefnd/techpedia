<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getAdminAudit } from '../../services/api'
import { auditActionNames, auditActionName, auditTargetNames, auditResult } from '../../services/adminAudit'
import { LatestRequest } from '../../services/latestRequest'
import type { AuditEntry } from '../../types/api'

const entries = ref<AuditEntry[]>([])
const actor = ref('')
const action = ref('')
const target = ref('')
const loading = ref(false)
const error = ref('')
const requests = new LatestRequest()

async function load() {
  loading.value = true
  error.value = ''
  const filter = { actor: actor.value.trim(), action: action.value.trim(), target: target.value.trim() }
  await requests.run((signal) => getAdminAudit(filter, signal), {
    success: (value) => { entries.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '감사 로그를 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

onMounted(load)
onBeforeUnmount(() => requests.dispose())
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header"><div><p>보안과 운영</p><h1>관리자 감사 로그</h1></div><span>{{ entries.length }}개</span></header>
    <VaCard><VaCardContent><form class="admin-audit-filters" @submit.prevent="load"><VaInput v-model="actor" label="작업자" input-aria-label="작업자" clearable /><label>작업 종류<select v-model="action"><option value="">모든 작업</option><option v-for="(label, code) in auditActionNames" :key="code" :value="code">{{ label }}</option></select></label><VaInput v-model="target" label="대상" input-aria-label="대상" clearable /><VaButton type="submit" :loading="loading">조회</VaButton></form></VaCardContent></VaCard>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaCard class="admin-table-card"><div class="admin-audit-list">
      <div class="admin-audit-row head"><span>시각</span><span>작업자</span><span>작업</span><span>대상</span><span>결과</span></div>
      <div v-for="entry in entries" :key="entry.id" class="admin-audit-row"><time :datetime="entry.created_at">{{ new Date(entry.created_at).toLocaleString() }}</time><span>{{ entry.actor }}</span><span><b>{{ auditActionName(entry.action) }}</b><small>{{ entry.action }}</small></span><span><b>{{ entry.target }}</b><small>{{ entry.detail || auditTargetNames[entry.target_type] || '알 수 없는 대상' }}</small></span><VaBadge :text="auditResult(entry.result).text" :color="auditResult(entry.result).color" /></div>
      <p v-if="!loading && !entries.length" class="admin-empty">기록이 없습니다.</p>
    </div></VaCard>
  </div>
</template>
