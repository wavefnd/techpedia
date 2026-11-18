<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Activity, ArrowUpRight, Clock3, Database, Files, Gauge, Languages, PenLine, Plug, ScrollText, Users } from '@lucide/vue'
import { getAdminAnalytics, getAdminDashboard } from '../../services/api'
import { account } from '../../services/auth'
import type { AdminAnalytics, AdminDashboard } from '../../types/api'

const dashboard = ref<AdminDashboard | null>(null)
const analytics = ref<AdminAnalytics | null>(null)
const error = ref('')
const maxLocaleDocuments = computed(() => Math.max(1, ...(dashboard.value?.by_locale.map((value) => value.documents) || [1])))

const metrics = computed(() => dashboard.value ? [
  { label: '전체 문서', value: dashboard.value.documents, note: '활성 언어판 기준', to: '/admin/pages', icon: Files, tone: 'violet' },
  { label: '활성 언어판', value: dashboard.value.active_editions, note: '현재 공개 중', to: '/admin/languages', icon: Languages, tone: 'blue' },
  { label: '전체 사용자', value: dashboard.value.users, note: '등록 계정', to: '/admin/users', icon: Users, tone: 'teal' },
  { label: '처리 중 언어판', value: dashboard.value.pending_editions, note: '검토 또는 개설 대기', to: '/admin/languages', icon: Clock3, tone: 'amber' },
  ...(analytics.value ? [
    { label: '30일 화면 조회', value: analytics.value.total_views, note: '익명 집계 · 고유 방문자 아님', to: '/admin/analytics', icon: Activity, tone: 'pink' },
    { label: '평균 왕복시간', value: Math.round(analytics.value.average_rtt_ms), note: `브라우저–서버 · ${analytics.value.rtt_samples.toLocaleString()}회`, to: '/admin/analytics', icon: Gauge, tone: 'cyan' },
  ] : []),
] : [])

const quickActions = [
  { title: 'ChatGPT 연결', description: '서버 활성화 상태를 확인하고 관리자 전용 연결을 관리합니다.', to: '/admin/connections', icon: Plug },
  { title: '새 문서 만들기', description: '언어판과 문서 이름을 선택해 편집을 시작합니다.', to: '/admin/pages', icon: PenLine },
  { title: 'Techdata 점검', description: '중앙 항목과 연결되지 않은 문서를 검토합니다.', to: '/admin/techdata-audit', icon: Database },
  { title: '감사 로그 확인', description: '관리 작업과 보안 관련 기록을 살펴봅니다.', to: '/admin/audit', icon: ScrollText },
]

onMounted(async () => {
  try {
    const [dashboardValue, analyticsValue] = await Promise.all([
      getAdminDashboard(),
      getAdminAnalytics(30).catch(() => null),
    ])
    dashboard.value = dashboardValue
    analytics.value = analyticsValue
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '관리 정보를 불러오지 못했습니다.'
  }
})

function date(value: string) {
  return new Intl.DateTimeFormat('ko', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}
</script>

<template>
  <div class="admin-view admin-dashboard-view">
    <header class="admin-page-header admin-dashboard-header">
      <div><p>운영 워크스페이스</p><h1>{{ account?.display_name || '관리자' }}님, 위키 운영 현황입니다.</h1><span>콘텐츠와 언어판, 지식 데이터의 현재 상태를 한곳에서 확인하세요.</span></div>
    </header>

    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <div v-else-if="!dashboard" class="admin-loading"><VaProgressCircle indeterminate /> 데이터를 불러오는 중입니다.</div>
    <template v-else>
      <section class="admin-metrics" aria-label="주요 통계">
        <RouterLink v-for="metric in metrics" :key="metric.label" :to="metric.to" class="admin-metric-card" :class="`is-${metric.tone}`">
          <span class="admin-metric-icon"><component :is="metric.icon" :size="20" /></span>
          <span class="admin-metric-copy"><small>{{ metric.label }}</small><strong>{{ metric.value.toLocaleString() }}</strong><em>{{ metric.note }}</em></span>
          <ArrowUpRight :size="17" class="admin-metric-arrow" />
        </RouterLink>
      </section>

      <div class="admin-workspace-grid">
        <section class="admin-panel admin-recent-panel">
          <header class="admin-panel-header"><div><span>최근 활동</span><h2>최근 수정 문서</h2></div><RouterLink to="/admin/pages">모든 문서 <ArrowUpRight :size="15" /></RouterLink></header>
          <div class="recent-admin-pages">
            <RouterLink v-for="page in dashboard.recent_documents" :key="`${page.locale}:${page.url}`" :to="page.edit_url">
              <span class="admin-recent-locale">{{ page.locale }}</span>
              <span><b>{{ page.name }}</b><small>{{ page.author || '시스템' }} · {{ page.revision }}</small></span>
              <time>{{ date(page.updated_at) }}</time>
              <ArrowUpRight :size="15" />
            </RouterLink>
            <p v-if="dashboard.recent_documents.length === 0" class="admin-empty">저장된 일반 문서가 없습니다.</p>
          </div>
        </section>

        <aside class="admin-panel admin-attention-panel">
          <header class="admin-panel-header"><div><span>운영 대기열</span><h2>지금 확인할 일</h2></div></header>
          <RouterLink to="/admin/languages" class="admin-attention-item" :class="{ clear: dashboard.pending_editions === 0 }">
            <span class="admin-attention-count">{{ dashboard.pending_editions }}</span>
            <span><b>{{ dashboard.pending_editions ? '언어판 검토·개설 대기' : '언어판 대기열이 비었습니다' }}</b><small>{{ dashboard.pending_editions ? '제안 수락, 메뉴 번역과 대문 구성을 진행하세요.' : '새 제안이 들어오면 이곳에 표시됩니다.' }}</small></span>
            <ArrowUpRight :size="16" />
          </RouterLink>
          <div class="admin-attention-note">
            <b>자동 번역 원칙</b>
            <p>언어 제안을 수락할 때 메인 메뉴 초안만 생성합니다. 관리자가 수정한 값과 문서 본문은 자동 번역이 덮어쓰지 않습니다.</p>
          </div>
        </aside>
      </div>

      <div class="admin-workspace-grid admin-workspace-grid-secondary">
        <section class="admin-panel">
          <header class="admin-panel-header"><div><span>콘텐츠 분포</span><h2>언어별 문서</h2></div><RouterLink to="/admin/languages">언어판 관리 <ArrowUpRight :size="15" /></RouterLink></header>
          <div class="locale-stats">
            <div v-for="value in dashboard.by_locale" :key="value.locale">
              <b>{{ value.locale }}</b>
              <span><i :style="{ width: `${value.documents / maxLocaleDocuments * 100}%` }"></i></span>
              <strong>{{ value.documents.toLocaleString() }}</strong>
            </div>
            <p v-if="dashboard.by_locale.length === 0" class="admin-empty">활성 언어판이 없습니다.</p>
          </div>
        </section>

        <section class="admin-panel">
          <header class="admin-panel-header"><div><span>빠른 작업</span><h2>자주 쓰는 관리 도구</h2></div></header>
          <div class="admin-quick-actions">
            <RouterLink v-for="action in quickActions" :key="action.to" :to="action.to">
              <span><component :is="action.icon" :size="18" /></span>
              <span><b>{{ action.title }}</b><small>{{ action.description }}</small></span>
              <ArrowUpRight :size="16" />
            </RouterLink>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>
