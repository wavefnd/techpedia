<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { account, updateProfile } from '../services/auth'
import { getMyContributions } from '../services/api'
import type { WikiChange } from '../types/api'

const permissionNames = {
  edit: '문서 편집',
  techdata: 'Techdata 관리',
  admin: '사용자 및 권한 관리',
  owner: 'TechPedia 소유자',
}

const contributions = ref<WikiChange[]>([])
const loading = ref(true)
const error = ref('')
const profileError = ref('')
const profileSaved = ref('')
const savingProfile = ref(false)
const displayName = ref(account.value?.display_name ?? '')
const bio = ref(account.value?.bio ?? '')

onMounted(async () => {
  try {
    contributions.value = await getMyContributions({ limit: 100 })
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '기여 내역을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
})

const actionNames = { create: '문서 생성', edit: '수정', move: '이동', restore: '복원', delete: '삭제' }

async function saveProfile() {
  savingProfile.value = true
  profileError.value = ''
  profileSaved.value = ''
  try {
    const updated = await updateProfile(displayName.value, bio.value)
    displayName.value = updated.display_name
    bio.value = updated.bio
    profileSaved.value = '프로필을 저장했습니다.'
  } catch (reason) {
    profileError.value = reason instanceof Error ? reason.message : '프로필을 저장하지 못했습니다.'
  } finally {
    savingProfile.value = false
  }
}
</script>

<template>
  <div v-if="account" class="account-view">
    <header>
      <p>TechPedia 계정</p>
      <h1>{{ account.display_name }}</h1>
      <span>@{{ account.username }}</span>
      <RouterLink class="account-public-link" :to="`/user/${account.username}`">공개 프로필 보기</RouterLink>
    </header>
    <section>
      <h2>프로필 편집</h2>
      <form class="account-profile-form" @submit.prevent="saveProfile">
        <label>표시 이름<input v-model="displayName" type="text" required maxlength="40" autocomplete="name"></label>
        <label>자기소개<textarea v-model="bio" maxlength="1000" rows="7" placeholder="관심 기술과 활동 분야를 소개해 주세요."></textarea></label>
        <div class="account-profile-actions"><span>{{ bio.length }} / 1000</span><button type="submit" :disabled="savingProfile">{{ savingProfile ? '저장 중' : '프로필 저장' }}</button></div>
        <p v-if="profileError" class="form-error">{{ profileError }}</p>
        <p v-if="profileSaved" class="form-success">{{ profileSaved }}</p>
      </form>
    </section>
    <section>
      <h2>권한</h2>
      <ul>
        <li v-for="permission in account.permissions" :key="permission">{{ permissionNames[permission] }}</li>
      </ul>
    </section>
    <section class="account-contributions">
      <h2>기여 내역</h2>
      <p v-if="loading">불러오는 중입니다.</p>
      <p v-else-if="error" class="error">{{ error }}</p>
      <ol v-else>
        <li v-for="change in contributions" :key="`${change.created_at}:${change.locale}:${change.name}`">
          <RouterLink :to="change.url">{{ change.name }}</RouterLink><span>{{ actionNames[change.action] }}<template v-if="change.revision"> · {{ change.revision }}</template></span><small>{{ change.summary || '요약 없음' }} · {{ new Date(change.created_at).toLocaleString() }}</small>
        </li>
        <li v-if="!contributions.length">기여 내역이 없습니다.</li>
      </ol>
    </section>
  </div>
</template>
