<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { archiveDiscussionTopic, deleteAdminDiscussionTopic, getAdminDiscussionTopics } from '../../services/api'
import { LatestRequest } from '../../services/latestRequest'
import type { DiscussionTopic } from '../../types/api'

const topics = ref<DiscussionTopic[]>([])
const total = ref(0)
const query = ref('')
const loading = ref(false)
const error = ref('')
const busy = ref(new Set<string>())
const requests = new LatestRequest()
let mounted = true

async function loadTopics() {
  loading.value = true
  error.value = ''
  const search = query.value.trim()
  await requests.run((signal) => getAdminDiscussionTopics(search, signal), {
    success: (response) => { topics.value = response.topics; total.value = response.count },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '토론 목록을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}

async function removeTopic(topic: DiscussionTopic) {
  if (busy.value.has(topic.id)) return
  if (!window.confirm(`'${topic.subject}' 토론과 모든 답글을 삭제하시겠습니까?`)) return
  busy.value.add(topic.id)
  try {
    await deleteAdminDiscussionTopic(topic.id)
    if (mounted) await loadTopics()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '토론을 삭제하지 못했습니다.'
  } finally { if (mounted) busy.value.delete(topic.id) }
}

async function archiveTopic(topic: DiscussionTopic) {
  if (busy.value.has(topic.id)) return
  if (!window.confirm(`'${topic.subject}' 토론을 종료하고 읽기 전용으로 보관하시겠습니까?`)) return
  busy.value.add(topic.id)
  try {
    await archiveDiscussionTopic(topic.id)
    if (mounted) await loadTopics()
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '토론을 종료하지 못했습니다.'
  } finally { if (mounted) busy.value.delete(topic.id) }
}

onMounted(loadTopics)
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header">
      <div><p>문서 토론</p><h1>토론 관리</h1></div>
      <VaBadge :text="`전체 ${total}개`" color="primary" />
    </header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaCard>
      <VaCardContent class="admin-filters discussion-filters">
        <VaInput v-model="query" label="토론 검색" input-aria-label="토론 검색" placeholder="주제, 본문, 작성자 또는 문서" clearable @keyup.enter="loadTopics" />
        <VaButton :loading="loading" @click="loadTopics">검색</VaButton>
      </VaCardContent>
    </VaCard>
    <VaCard class="admin-table-card discussion-list">
      <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 불러오는 중입니다.</div>
      <div v-else class="admin-table" role="table" aria-label="토론 주제 목록">
        <div class="admin-table-row discussion-head head" role="row"><span>주제</span><span>문서</span><span>상태</span><span>작업</span></div>
        <div v-for="topic in topics" :key="topic.id" class="admin-table-row discussion-row" role="row">
          <span><b>{{ topic.subject }}</b><small>{{ topic.author }} · {{ new Date(topic.updated_at).toLocaleString() }}</small><small>{{ topic.body }}</small></span>
          <span><RouterLink :to="{ path: topic.page_url, query: { tab: 'discussion' } }">{{ topic.locale }} · {{ topic.page_title }}</RouterLink></span>
          <span><VaBadge :text="topic.archived ? `종료 · 답글 ${topic.reply_count}개` : `진행 중 · 답글 ${topic.reply_count}개`" :color="topic.archived ? 'secondary' : 'success'" /></span>
          <span class="discussion-admin-actions"><VaButton v-if="!topic.archived" preset="secondary" size="small" :disabled="busy.has(topic.id)" @click="archiveTopic(topic)">종료</VaButton><VaButton preset="secondary" color="danger" size="small" :disabled="busy.has(topic.id)" @click="removeTopic(topic)">삭제</VaButton></span>
        </div>
        <p v-if="topics.length === 0" class="admin-empty">표시할 토론이 없습니다.</p>
      </div>
    </VaCard>
  </div>
</template>
