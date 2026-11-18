<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getUserProfile } from '../services/api'
import { account } from '../services/auth'
import type { PublicUserProfile } from '../types/api'

const route = useRoute()
const profile = ref<PublicUserProfile | null>(null)
const loading = ref(true)
const error = ref('')
const username = computed(() => String(route.params.username || ''))
const isOwnProfile = computed(() => account.value?.username === profile.value?.username)
const actionNames = { create: '문서 생성', edit: '수정', move: '이동', restore: '복원', delete: '삭제' }

watch(username, async (value, _, onCleanup) => {
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  loading.value = true
  error.value = ''
  profile.value = null
  try {
    profile.value = await getUserProfile(value, controller.signal)
  } catch (reason) {
    if (!controller.signal.aborted) error.value = reason instanceof Error ? reason.message : '사용자 프로필을 불러오지 못했습니다.'
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}, { immediate: true })
</script>

<template>
  <div class="account-view public-profile-view">
    <p v-if="loading" class="profile-loading">프로필을 불러오는 중입니다.</p>
    <div v-else-if="error" class="profile-not-found">
      <p>사용자 프로필</p>
      <h1>프로필을 찾을 수 없습니다</h1>
      <span>{{ error }}</span>
    </div>
    <template v-else-if="profile">
      <header>
        <p>TechPedia 사용자</p>
        <h1>{{ profile.display_name }}</h1>
        <span>@{{ profile.username }}</span>
        <RouterLink v-if="isOwnProfile" class="account-public-link" to="/account">프로필 편집</RouterLink>
      </header>
      <section class="public-profile-bio">
        <h2>소개</h2>
        <p v-if="profile.bio">{{ profile.bio }}</p>
        <p v-else class="profile-empty">작성된 자기소개가 없습니다.</p>
        <small>가입일 {{ new Date(profile.created_at).toLocaleDateString() }}</small>
      </section>
      <section class="account-contributions">
        <h2>최근 기여</h2>
        <ol>
          <li v-for="change in profile.contributions" :key="`${change.created_at}:${change.locale}:${change.name}`">
            <RouterLink :to="change.url">{{ change.name }}</RouterLink><span>{{ actionNames[change.action] }}<template v-if="change.revision"> · {{ change.revision }}</template></span><small>{{ change.summary || '요약 없음' }} · {{ new Date(change.created_at).toLocaleString() }}</small>
          </li>
          <li v-if="!profile.contributions.length">공개 기여 내역이 없습니다.</li>
        </ol>
      </section>
    </template>
  </div>
</template>
