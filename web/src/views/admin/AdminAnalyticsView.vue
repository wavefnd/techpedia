<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, Gauge, Globe2, MousePointerClick, ShieldCheck } from '@lucide/vue'
import { getAdminAnalytics } from '../../services/api'
import { LatestRequest } from '../../services/latestRequest'
import type { AdminAnalytics, AdminAnalyticsBreakdown } from '../../types/api'

const periods = [7, 30, 90]
const days = ref(30)
const analytics = ref<AdminAnalytics | null>(null)
const loading = ref(false)
const error = ref('')
const requests = new LatestRequest()
const maxDaily = computed(() => Math.max(1, ...(analytics.value?.daily.map((value) => value.views) || [1])))
const maxCountry = computed(() => Math.max(1, ...(analytics.value?.countries.map((value) => value.views) || [1])))
const busiest = computed(() => analytics.value?.daily.reduce((best, value) => value.views > best.views ? value : best, { date: '', views: 0, average_rtt_ms: 0 }))

const routeLabels: Record<string, string> = {
  portal: '언어 선택 포털',
  article: '문서 읽기',
  search: '검색',
  graph: '지식 그래프',
  support: '후원 안내',
  history: '문서 역사',
  languages: '언어판 안내',
  profile: '사용자 프로필',
}

onMounted(load)
onBeforeUnmount(() => requests.dispose())

async function load() {
  loading.value = true
  error.value = ''
  analytics.value = null
  const selectedDays = days.value
  await requests.run((signal) => getAdminAnalytics(selectedDays, signal), {
    success: (value) => { analytics.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '접속 통계를 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

function selectPeriod(value: number) {
  if (days.value === value) return
  days.value = value
  void load()
}

function countryName(code: string) {
  if (code === 'ZZ' || code === 'XX') return '알 수 없음'
  try { return new Intl.DisplayNames(['ko'], { type: 'region' }).of(code) || code } catch { return code }
}

function percentage(value: number, total = analytics.value?.total_views || 0) {
  return total ? `${(value / total * 100).toFixed(1)}%` : '0%'
}

function shortDate(value: string) {
  return new Intl.DateTimeFormat('ko', { month: 'short', day: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`))
}

function top(values: AdminAnalyticsBreakdown[], limit = 8) {
  return values.slice(0, limit)
}
</script>

<template>
  <div class="admin-view admin-analytics-view">
    <header class="admin-page-header admin-analytics-header">
      <div><p>Audience intelligence</p><h1>접속 통계</h1><span>개인을 추적하지 않고 화면 조회와 서버 체감 속도의 흐름만 봅니다.</span></div>
      <div class="analytics-period" aria-label="통계 기간">
        <button v-for="period in periods" :key="period" type="button" :class="{ active: days === period }" @click="selectPeriod(period)">{{ period }}일</button>
      </div>
    </header>

    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <div v-else-if="loading && !analytics" class="admin-loading"><VaProgressCircle indeterminate /> 통계를 불러오는 중입니다.</div>

    <template v-if="analytics">
      <section class="analytics-hero-metrics">
        <article><span><MousePointerClick :size="19" /></span><small>화면 조회</small><strong>{{ analytics.total_views.toLocaleString() }}</strong><em>고유 방문자 수가 아닙니다</em></article>
        <article><span><Gauge :size="19" /></span><small>평균 왕복시간</small><strong>{{ Math.round(analytics.average_rtt_ms).toLocaleString() }}<i>ms</i></strong><em>{{ analytics.rtt_samples.toLocaleString() }}회 측정</em></article>
        <article><span><Globe2 :size="19" /></span><small>확인된 국가</small><strong>{{ analytics.countries.filter((value) => !['ZZ', 'XX'].includes(value.key)).length.toLocaleString() }}</strong><em>Cloudflare 국가 코드 기준</em></article>
        <article><span><Activity :size="19" /></span><small>가장 활발한 날</small><strong>{{ busiest?.views.toLocaleString() || 0 }}</strong><em>{{ busiest?.date ? shortDate(busiest.date) : '데이터 없음' }}</em></article>
      </section>

      <section class="admin-panel analytics-traffic-panel">
        <header class="admin-panel-header"><div><span>Traffic pulse</span><h2>일별 화면 조회</h2></div><small>{{ analytics.from }} — {{ analytics.through }}</small></header>
        <div class="analytics-chart" role="img" :aria-label="`${days}일간 일별 화면 조회 막대 그래프`">
          <div v-for="(value, index) in analytics.daily" :key="value.date" class="analytics-chart-column" :title="`${value.date}: ${value.views.toLocaleString()}회 · 평균 ${Math.round(value.average_rtt_ms)}ms`">
            <span class="analytics-chart-value">{{ value.views ? value.views.toLocaleString() : '' }}</span>
            <i :style="{ height: `${Math.max(value.views ? 8 : 2, value.views / maxDaily * 100)}%` }"></i>
            <small v-if="analytics.daily.length <= 14 || index === 0 || index === analytics.daily.length - 1 || index % 5 === 0">{{ shortDate(value.date) }}</small>
          </div>
        </div>
      </section>

      <div class="analytics-insight-grid">
        <section class="admin-panel analytics-country-panel">
          <header class="admin-panel-header"><div><span>Geography</span><h2>접속 국가</h2></div><small>상위 {{ Math.min(8, analytics.countries.length) }}개</small></header>
          <div class="analytics-ranking">
            <div v-for="value in top(analytics.countries)" :key="value.key">
              <span><b>{{ countryName(value.key) }}</b><small>{{ value.key }}</small></span>
              <span class="analytics-progress"><i :style="{ width: `${value.views / maxCountry * 100}%` }"></i></span>
              <strong>{{ value.views.toLocaleString() }}</strong><em>{{ percentage(value.views) }}</em>
            </div>
            <p v-if="analytics.countries.length === 0" class="admin-empty">아직 국가 데이터가 없습니다.</p>
          </div>
        </section>

        <section class="admin-panel analytics-breakdown-panel">
          <header class="admin-panel-header"><div><span>Content paths</span><h2>이용 화면</h2></div></header>
          <div class="analytics-donut-layout">
            <div class="analytics-orbit" aria-hidden="true"><span><b>{{ analytics.total_views.toLocaleString() }}</b><small>views</small></span></div>
            <div class="analytics-legend">
              <div v-for="value in analytics.routes" :key="value.key"><i></i><span><b>{{ routeLabels[value.key] || value.key }}</b><small>{{ percentage(value.views) }}</small></span><strong>{{ value.views.toLocaleString() }}</strong></div>
              <p v-if="analytics.routes.length === 0" class="admin-empty">아직 화면 데이터가 없습니다.</p>
            </div>
          </div>
        </section>

        <section class="admin-panel analytics-locale-panel">
          <header class="admin-panel-header"><div><span>Language editions</span><h2>언어판별 조회</h2></div></header>
          <div class="analytics-locale-cloud">
            <span v-for="value in top(analytics.locales, 12)" :key="value.key"><b>{{ value.key }}</b><small>{{ value.views.toLocaleString() }} · {{ percentage(value.views) }}</small></span>
            <p v-if="analytics.locales.length === 0" class="admin-empty">언어판이 없는 화면만 조회되었습니다.</p>
          </div>
        </section>

        <section class="admin-panel analytics-privacy-panel">
          <header><span><ShieldCheck :size="21" /></span><div><small>Privacy by design</small><h2>통계에 남기지 않는 것</h2></div></header>
          <p>통계 저장소에는 IP 주소, 계정, 쿠키, 세션, 브라우저 지문, 문서 제목과 정확한 URL을 기록하지 않습니다.</p>
          <ul><li>날짜별 합계만 BadgerDB에 저장</li><li>국가 코드는 역방향 프록시가 제공할 때만 사용</li><li>Firefox의 추적 거부와 Global Privacy Control 존중</li></ul>
        </section>
      </div>
    </template>
  </div>
</template>
