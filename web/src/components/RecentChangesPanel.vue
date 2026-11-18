<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getRecentChanges } from '../services/api'
import { documentURL } from '../i18n'
import type { ChangeAction, WikiChange } from '../types/api'

const props = defineProps<{ locale: string }>()
const changes = ref<WikiChange[]>([])
const filterLocale = ref(props.locale)
const user = ref('')
const query = ref('')
const action = ref<ChangeAction | ''>('')
const since = ref('')
const until = ref('')
const loading = ref(false)
const error = ref('')
const actionNames = { create: '신규', edit: '수정', move: '이동', restore: '복원', delete: '삭제' }

async function load() {
  loading.value = true
  error.value = ''
  try {
    changes.value = await getRecentChanges({
      locale: filterLocale.value.trim(), user: user.value.trim(), q: query.value.trim(), action: action.value,
      since: since.value ? new Date(`${since.value}T00:00:00`).toISOString() : '',
      until: until.value ? new Date(`${until.value}T23:59:59.999`).toISOString() : '', limit: 200,
    })
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '최근 변경을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="recent-changes-panel">
    <form class="recent-change-filters" @submit.prevent="load">
      <label>언어판<input v-model="filterLocale" type="text" placeholder="전체 또는 BCP 47"></label>
      <label>문서 이름<input v-model="query" type="search" placeholder="문서 검색"></label>
      <label>사용자<input v-model="user" type="search" placeholder="사용자 이름"></label>
      <label>작업<select v-model="action"><option value="">모든 작업</option><option value="create">신규</option><option value="edit">수정</option><option value="move">이동</option><option value="restore">복원</option><option value="delete">삭제</option></select></label>
      <label>시작일<input v-model="since" type="date"></label>
      <label>종료일<input v-model="until" type="date"></label>
      <button type="submit" :disabled="loading">{{ loading ? '조회 중' : '필터 적용' }}</button>
    </form>
    <p v-if="error" class="editor-message error">{{ error }}</p>
    <ol v-else class="recent-change-results">
      <li v-for="change in changes" :key="`${change.created_at}:${change.locale}:${change.name}:${change.action}`">
        <span><b :class="`action-${change.action}`">{{ actionNames[change.action] }}</b><RouterLink :to="change.url">{{ change.name }}</RouterLink><RouterLink v-if="change.revision" :to="documentURL('history', change.locale, change.name)">{{ change.revision }}</RouterLink></span>
        <span><RouterLink v-if="change.author" :to="`/user/${change.author}`">{{ change.author }}</RouterLink><template v-if="change.summary"> · {{ change.summary }}</template> · <time :datetime="change.created_at">{{ new Date(change.created_at).toLocaleString() }}</time></span>
      </li>
      <li v-if="!loading && !changes.length">조건에 맞는 변경이 없습니다.</li>
    </ol>
  </section>
</template>
