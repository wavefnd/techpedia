<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  archiveDiscussionTopic,
  createDiscussionReply,
  createDiscussionTopic,
  deleteDiscussionReply,
  getDiscussionThread,
  getDiscussionTopics,
} from '../services/api'
import { account } from '../services/auth'
import type { DiscussionThread, DiscussionTopic } from '../types/api'

const props = defineProps<{ pageName: string; pageTitle: string; pageUrl: string; locale: string }>()
const topics = ref<DiscussionTopic[]>([])
const thread = ref<DiscussionThread | null>(null)
const subject = ref('')
const body = ref('')
const replyBody = ref('')
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const creating = ref(false)
const activeTopics = computed(() => topics.value.filter((topic) => !topic.archived))
const archivedTopics = computed(() => topics.value.filter((topic) => topic.archived))
const korean = computed(() => props.locale.toLowerCase().startsWith('ko'))
const copy = computed(() => korean.value ? {
  title: '문서 토론', newTopic: '새 주제', subject: '주제', body: '내용', create: '토론 시작', cancel: '취소',
  empty: '아직 열린 토론이 없습니다.', replies: '답글', back: '주제 목록', reply: '답글 작성', submitReply: '답글 등록',
  login: '토론에 참여하려면 로그인하세요.', delete: '삭제', archive: '토론 종료', archived: '종료된 토론', archivedList: '종료된 토론',
  confirmArchive: '이 토론을 종료하고 읽기 전용으로 보관하시겠습니까?', confirmReply: '이 답글을 삭제하시겠습니까?', loading: '토론을 불러오는 중입니다.',
  readOnly: '이 토론은 종료되어 아카이브되었습니다. 내용은 계속 열람할 수 있지만 새 답글은 작성할 수 없습니다.',
} : {
  title: 'Article discussion', newTopic: 'New topic', subject: 'Subject', body: 'Message', create: 'Start discussion', cancel: 'Cancel',
  empty: 'There are no open topics yet.', replies: 'replies', back: 'Topic list', reply: 'Write a reply', submitReply: 'Post reply',
  login: 'Log in to participate in this discussion.', delete: 'Delete', archive: 'Close discussion', archived: 'Archived', archivedList: 'Archived discussions',
  confirmArchive: 'Close and archive this discussion as read-only?', confirmReply: 'Delete this reply?', loading: 'Loading discussion.',
  readOnly: 'This discussion is archived and read-only. Its contents remain available, but new replies are disabled.',
})

function formatDate(value: string) {
  return new Intl.DateTimeFormat(props.locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function loadTopics() {
  loading.value = true
  error.value = ''
  thread.value = null
  try {
    topics.value = await getDiscussionTopics(props.locale, props.pageName)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '토론을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

async function openTopic(topic: DiscussionTopic) {
  loading.value = true
  error.value = ''
  try {
    thread.value = await getDiscussionThread(topic.id)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '토론을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

async function submitTopic() {
  submitting.value = true
  error.value = ''
  try {
    const topic = await createDiscussionTopic(props.locale, props.pageName, subject.value, body.value)
    subject.value = ''
    body.value = ''
    creating.value = false
    await loadTopics()
    await openTopic(topic)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '토론을 시작하지 못했습니다.'
  } finally {
    submitting.value = false
  }
}

async function submitReply() {
  if (!thread.value) return
  submitting.value = true
  error.value = ''
  try {
    await createDiscussionReply(thread.value.topic.id, replyBody.value)
    replyBody.value = ''
    thread.value = await getDiscussionThread(thread.value.topic.id)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '답글을 등록하지 못했습니다.'
  } finally {
    submitting.value = false
  }
}

async function archiveTopic(topic: DiscussionTopic) {
  if (!window.confirm(copy.value.confirmArchive)) return
  try {
    const archived = await archiveDiscussionTopic(topic.id)
    if (thread.value?.topic.id === archived.id) thread.value.topic = archived
    topics.value = topics.value.map((value) => value.id === archived.id ? archived : value)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '토론을 종료하지 못했습니다.'
  }
}

async function removeReply(replyID: string) {
  if (!thread.value || !window.confirm(copy.value.confirmReply)) return
  await deleteDiscussionReply(thread.value.topic.id, replyID)
  thread.value = await getDiscussionThread(thread.value.topic.id)
}

watch(() => [props.locale, props.pageName], loadTopics)
onMounted(loadTopics)
</script>

<template>
  <section class="discussion-panel" :aria-label="copy.title">
    <header class="discussion-header">
      <div><p>{{ pageTitle }}</p><h2>{{ copy.title }}</h2></div>
      <button v-if="!thread && account" type="button" class="primary-action" @click="creating = !creating">{{ creating ? copy.cancel : copy.newTopic }}</button>
    </header>

    <p v-if="error" class="editor-message error">{{ error }}</p>
    <div v-if="loading" class="page-state">{{ copy.loading }}</div>

    <template v-else-if="thread">
      <button type="button" class="discussion-back" @click="loadTopics">{{ copy.back }}</button>
      <article class="discussion-thread">
        <header><h3>{{ thread.topic.subject }} <small v-if="thread.topic.archived" class="discussion-status archived">{{ copy.archived }}</small></h3><span>{{ thread.topic.author }} · {{ formatDate(thread.topic.created_at) }}</span></header>
        <p>{{ thread.topic.body }}</p>
        <button v-if="thread.topic.can_archive" type="button" class="discussion-archive-button" @click="archiveTopic(thread.topic)">{{ copy.archive }}</button>
      </article>
      <p v-if="thread.topic.archived" class="discussion-archive-notice">{{ copy.readOnly }}<small v-if="thread.topic.archived_at">{{ formatDate(thread.topic.archived_at) }}</small></p>
      <ol class="discussion-replies">
        <li v-for="reply in thread.replies" :key="reply.id">
          <header><strong>{{ reply.author }}</strong><span>{{ formatDate(reply.created_at) }}</span></header>
          <p>{{ reply.body }}</p>
          <button v-if="reply.can_delete" type="button" class="danger-link" @click="removeReply(reply.id)">{{ copy.delete }}</button>
        </li>
      </ol>
      <form v-if="account && !thread.topic.archived" class="discussion-form reply-form" @submit.prevent="submitReply">
        <label>{{ copy.reply }}<textarea v-model="replyBody" required maxlength="20000" rows="5"></textarea></label>
        <button type="submit" :disabled="submitting">{{ copy.submitReply }}</button>
      </form>
      <p v-else-if="!account && !thread.topic.archived" class="discussion-login"><RouterLink :to="{ path: '/login', query: { redirect: pageUrl + '?tab=discussion' } }">{{ copy.login }}</RouterLink></p>
    </template>

    <template v-else>
      <form v-if="creating && account" class="discussion-form" @submit.prevent="submitTopic">
        <label>{{ copy.subject }}<input v-model="subject" required maxlength="160" type="text"></label>
        <label>{{ copy.body }}<textarea v-model="body" required maxlength="20000" rows="7"></textarea></label>
        <div><button type="button" class="secondary-action" @click="creating = false">{{ copy.cancel }}</button><button type="submit" :disabled="submitting">{{ copy.create }}</button></div>
      </form>
      <div v-if="activeTopics.length" class="discussion-topic-list" role="list">
        <button v-for="topic in activeTopics" :key="topic.id" type="button" role="listitem" @click="openTopic(topic)">
          <span><strong>{{ topic.subject }}</strong><small>{{ topic.author }} · {{ formatDate(topic.updated_at) }}</small></span>
          <b>{{ topic.reply_count }} {{ copy.replies }}</b>
        </button>
      </div>
      <details v-if="archivedTopics.length" class="discussion-archive-list">
        <summary>{{ copy.archivedList }} <span>{{ archivedTopics.length }}</span></summary>
        <div class="discussion-topic-list" role="list">
          <button v-for="topic in archivedTopics" :key="topic.id" type="button" role="listitem" @click="openTopic(topic)">
            <span><strong>{{ topic.subject }}</strong><small>{{ topic.author }} · {{ formatDate(topic.updated_at) }}</small></span>
            <b><span class="discussion-status archived">{{ copy.archived }}</span> · {{ topic.reply_count }} {{ copy.replies }}</b>
          </button>
        </div>
      </details>
      <p v-if="!topics.length && !creating" class="discussion-empty">{{ copy.empty }}</p>
      <p v-if="!account" class="discussion-login"><RouterLink :to="{ path: '/login', query: { redirect: pageUrl + '?tab=discussion' } }">{{ copy.login }}</RouterLink></p>
    </template>
  </section>
</template>
