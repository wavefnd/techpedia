<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getAdminServerInfo } from '../../services/api'
import type { AdminServerInfo } from '../../types/api'

const info = ref<AdminServerInfo | null>(null)
const loading = ref(false)
const error = ref('')

const uptime = computed(() => {
  const total = info.value?.runtime.uptime_seconds || 0
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  return [days ? `${days}일` : '', hours ? `${hours}시간` : '', `${minutes}분`].filter(Boolean).join(' ')
})

function bytes(value: number): string {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index < 2 ? 0 : 1)} ${units[index]}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    info.value = await getAdminServerInfo()
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '서버 정보를 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="admin-view server-info-view">
    <header class="admin-page-header"><div><p>운영 환경</p><h1>서버 정보</h1></div><VaButton preset="secondary" :loading="loading" @click="load">새로 고침</VaButton></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <div v-if="loading && !info" class="admin-loading"><VaProgressCircle indeterminate /> 서버 정보를 확인하는 중입니다.</div>
    <template v-else-if="info">
      <VaAlert color="info">서버 호스트와 TechPedia 컨테이너를 구분해 표시합니다. 비밀번호, 세션, 환경 변수 원문, 네트워크 인터페이스와 프로세스 목록은 노출하지 않습니다.</VaAlert>
      <section class="server-info-grid">
        <VaCard><VaCardTitle>TechPedia 런타임</VaCardTitle><VaCardContent class="server-property-list"><div><span>엔진 버전</span><b>v{{ info.runtime.engine_version }}</b></div><div><span>Go 런타임</span><code>{{ info.runtime.go_version }}</code></div><div><span>시작 시각</span><b>{{ new Date(info.runtime.started_at).toLocaleString() }}</b></div><div><span>가동 시간</span><b>{{ uptime }}</b></div><div><span>Goroutine</span><b>{{ info.runtime.goroutines }}</b></div></VaCardContent></VaCard>
        <VaCard><VaCardTitle>서버 호스트</VaCardTitle><VaCardContent class="server-property-list"><div><span>서버 위치</span><b>{{ info.location || '미설정' }}</b></div><div><span>호스트 이름</span><code>{{ info.host.hostname }}</code></div><div><span>호스트 운영체제</span><b>{{ info.host.name || `${info.host.platform} ${info.host.platform_version}` }}</b></div><div><span>호스트 커널</span><code>{{ info.host.kernel_version }}</code></div><div><span>아키텍처</span><b>{{ info.host.architecture }}</b></div></VaCardContent></VaCard>
        <VaCard><VaCardTitle>컨테이너 런타임</VaCardTitle><VaCardContent class="server-property-list"><div><span>컨테이너 운영체제</span><b>{{ info.container.name || `${info.container.platform} ${info.container.platform_version}` }}</b></div><div><span>아키텍처</span><b>{{ info.container.architecture }}</b></div><div><span>격리 환경</span><b>{{ [info.container.virtualization, info.container.virtualization_role].filter(Boolean).join(' / ') || '감지되지 않음' }}</b></div></VaCardContent></VaCard>
        <VaCard><VaCardTitle>프로세서</VaCardTitle><VaCardContent class="server-property-list"><div class="server-wide-property"><span>모델</span><b>{{ info.cpu.model || '확인할 수 없음' }}</b></div><div><span>물리 코어</span><b>{{ info.cpu.physical_cores }}</b></div><div><span>논리 코어</span><b>{{ info.cpu.logical_cores }}</b></div></VaCardContent></VaCard>
        <VaCard><VaCardTitle>메모리</VaCardTitle><VaCardContent class="server-resource"><div><b>{{ bytes(info.memory.used) }}</b><span>/ {{ bytes(info.memory.total) }}</span></div><VaProgressBar :model-value="info.memory.used_percent" /><small>사용률 {{ info.memory.used_percent.toFixed(1) }}% · 사용 가능 {{ bytes(info.memory.available) }}</small></VaCardContent></VaCard>
        <VaCard><VaCardTitle>데이터 저장소</VaCardTitle><VaCardContent class="server-resource"><code>{{ info.storage.path }}</code><div><b>{{ bytes(info.storage.used) }}</b><span>/ {{ bytes(info.storage.total) }}</span></div><VaProgressBar :model-value="info.storage.used_percent" /><small>사용률 {{ info.storage.used_percent.toFixed(1) }}% · 여유 {{ bytes(info.storage.free) }}</small></VaCardContent></VaCard>
      </section>
    </template>
  </div>
</template>
