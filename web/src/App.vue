<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useRoute } from 'vue-router'
import { RouterView } from 'vue-router'
import { navigationFailed, reportNavigationFailure } from './services/navigationFailure'

const AdminShell = defineAsyncComponent({ loader: () => import('./components/admin/AdminShell.vue'), onError: (_error, _retry, fail) => { reportNavigationFailure(); fail() } })
const WikiShell = defineAsyncComponent({ loader: () => import('./components/WikiShell.vue'), onError: (_error, _retry, fail) => { reportNavigationFailure(); fail() } })
const route = useRoute()
const adminMode = computed(() => route.path === '/admin' || route.path.startsWith('/admin/'))
const portalMode = computed(() => route.path === '/')
const standaloneMode = computed(() => portalMode.value || route.name === 'support' || route.name === 'not-found')
function reload() { window.location.reload() }
</script>

<template>
  <div v-if="navigationFailed" class="navigation-failure" role="alert">
    <p>화면을 불러오지 못했습니다. 네트워크 연결을 확인한 뒤 다시 시도하세요. 새로고침하면 저장하지 않은 입력은 사라질 수 있습니다.</p>
    <button type="button" @click="reload">페이지 새로고침</button>
    <a href="/admin">관리 홈으로 이동</a>
  </div>
  <template v-if="route.matched.length">
    <AdminShell v-if="adminMode">
      <RouterView />
    </AdminShell>
    <RouterView v-else-if="standaloneMode" />
    <WikiShell v-else />
  </template>
</template>

<style scoped>
.navigation-failure { position: relative; z-index: 100; margin: 1rem; padding: 1rem; border: 1px solid var(--border-color, #aaa); background: var(--surface-color, #fff); color: var(--text-color, #222); }
.navigation-failure button { margin-inline-end: 1rem; padding: .5rem; cursor: pointer; }
</style>
