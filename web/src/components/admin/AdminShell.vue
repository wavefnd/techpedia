<script setup lang="ts">
import { computed, onErrorCaptured, ref, watch } from 'vue'
import type { Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Bot,
  Box,
  ChartNoAxesCombined,
  Database,
  ExternalLink,
  Files,
  HeartHandshake,
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  MessagesSquare,
  Plug,
  ScrollText,
  ServerCog,
  Trash2,
  Users,
  X,
} from '@lucide/vue'
import { account, logout } from '../../services/auth'

const route = useRoute()
const router = useRouter()
const open = ref(false)
const viewFailed = ref(false)
onErrorCaptured(() => { viewFailed.value = true; return false })
watch(() => route.fullPath, () => { viewFailed.value = false; open.value = false })
function reload() { window.location.reload() }

document.documentElement.lang = 'ko'
document.documentElement.dir = 'ltr'
document.documentElement.dataset.writingSystem = 'Kore'

type NavigationItem = { to: string; label: string; icon: Component; exact?: boolean }

const navigation: Array<{ label: string; items: NavigationItem[] }> = [
  {
    label: '워크스페이스',
    items: [
      { to: '/admin', label: '관리 홈', icon: LayoutDashboard, exact: true },
      { to: '/admin/analytics', label: '접속 통계', icon: ChartNoAxesCombined },
    ],
  },
  {
    label: '콘텐츠',
    items: [
      { to: '/admin/pages', label: '문서', icon: Files },
      { to: '/admin/boxes', label: '박스', icon: Box },
      { to: '/admin/languages', label: '언어판', icon: Languages },
      { to: '/admin/trash', label: '휴지통', icon: Trash2 },
    ],
  },
  {
    label: '지식 운영',
    items: [
      { to: '/admin/techdata-audit', label: 'Techdata', icon: Database },
      { to: '/admin/discussions', label: '토론', icon: MessagesSquare },
      { to: '/admin/bots', label: '봇 검토', icon: Bot },
      { to: '/admin/proposals', label: 'ChatGPT 제안 검토', icon: ScrollText },
    ],
  },
  {
    label: '사이트 운영',
    items: [
      { to: '/admin/users', label: '사용자', icon: Users },
      { to: '/admin/connections', label: 'ChatGPT 연결', icon: Plug },
      { to: '/admin/support', label: '후원', icon: HeartHandshake },
      { to: '/admin/audit', label: '감사 로그', icon: ScrollText },
    ],
  },
]

const flatNavigation = navigation.flatMap((group) => group.items)
const currentItem = computed(() => {
  if (route.path.startsWith('/admin/techdata/')) return flatNavigation.find((item) => item.to === '/admin/techdata-audit')
  return flatNavigation.find((item) => active(item))
})
const currentTitle = computed(() => route.path === '/admin/server' ? '서버 정보' : currentItem.value?.label || '관리자 패널')
const accountInitial = computed(() => (account.value?.display_name || account.value?.username || 'T').trim().charAt(0).toLocaleUpperCase('ko'))
const accountRole = computed(() => account.value?.permissions?.includes('owner') ? '소유자' : '관리자')

function active(item: NavigationItem) {
  if (item.to === '/admin/techdata-audit' && route.path.startsWith('/admin/techdata/')) return true
  return item.exact ? route.path === item.to : route.path.startsWith(item.to)
}

async function signOut() {
  await logout()
  await router.push('/login')
}
</script>

<template>
  <div class="admin-shell">
    <aside id="admin-navigation" class="admin-sidebar" :class="{ open }">
      <div class="admin-sidebar-head">
        <RouterLink class="admin-brand" to="/admin" @click="open = false">
          <span class="admin-brand-mark" aria-hidden="true">T</span>
          <span><b>TechPedia</b><small>Control Center</small></span>
        </RouterLink>
        <button class="admin-sidebar-close" type="button" aria-label="관리 메뉴 닫기" @click="open = false"><X :size="20" /></button>
      </div>

      <nav aria-label="관리자 메뉴">
        <section v-for="group in navigation" :key="group.label" class="admin-nav-group">
          <h2>{{ group.label }}</h2>
          <RouterLink
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            :class="{ active: active(item) }"
            :aria-current="active(item) ? 'page' : undefined"
            @click="open = false"
          >
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </section>
      </nav>

      <div class="admin-sidebar-footer">
        <RouterLink class="admin-server-link" to="/admin/server" :class="{ active: route.path === '/admin/server' }" @click="open = false"><ServerCog :size="18" /><span>서버 정보</span></RouterLink>
        <RouterLink class="admin-return" to="/"><ExternalLink :size="17" /><span>위키로 돌아가기</span></RouterLink>
      </div>
    </aside>

    <button v-if="open" class="admin-scrim" type="button" aria-label="관리 메뉴 닫기" @click="open = false"></button>

    <header class="admin-topbar">
      <button class="admin-menu" type="button" aria-controls="admin-navigation" :aria-expanded="open" aria-label="관리 메뉴 열기" @click="open = true"><Menu :size="22" /></button>
      <div class="admin-location"><span>관리 센터</span><b>{{ currentTitle }}</b></div>
      <div class="admin-account">
        <span class="admin-account-avatar" aria-hidden="true">{{ accountInitial }}</span>
        <span class="admin-account-copy"><b>{{ account?.display_name }}</b><small>{{ accountRole }}</small></span>
        <button type="button" title="로그아웃" aria-label="로그아웃" @click="signOut"><LogOut :size="18" /></button>
      </div>
    </header>

    <main class="admin-content" @click="open = false">
      <section v-if="viewFailed" class="admin-panel" role="alert">
        <h1>관리자 화면을 표시하지 못했습니다</h1>
        <p>다른 메뉴로 이동하거나 페이지를 새로고침할 수 있습니다. 새로고침하면 저장하지 않은 입력은 사라질 수 있습니다.</p>
        <button type="button" @click="reload">페이지 새로고침</button>
      </section>
      <slot v-else />
    </main>
  </div>
</template>
