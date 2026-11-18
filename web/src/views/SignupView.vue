<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { signup } from '../services/auth'

const router = useRouter()
const route = useRoute()
const username = ref('')
const displayName = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await signup(username.value, displayName.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/ko/wiki/대문'
    await router.replace(redirect)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '계정을 만들지 못했습니다.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="auth-view">
    <form class="auth-form" @submit.prevent="submit">
      <p>TechPedia 계정</p>
      <h1>회원가입</h1>
      <label for="signup-username">사용자 이름</label>
      <input id="signup-username" v-model="username" type="text" minlength="3" maxlength="32" pattern="[A-Za-z0-9_]+" autocomplete="username" required autofocus>
      <small>영문 소문자, 숫자, 밑줄을 사용합니다.</small>
      <label for="signup-display-name">표시 이름</label>
      <input id="signup-display-name" v-model="displayName" type="text" maxlength="40" autocomplete="name">
      <label for="signup-password">비밀번호</label>
      <input id="signup-password" v-model="password" type="password" minlength="10" autocomplete="new-password" required>
      <small>10자 이상 입력합니다.</small>
      <p v-if="error" class="form-error">{{ error }}</p>
      <button type="submit" :disabled="busy">{{ busy ? '계정 생성 중' : '계정 만들기' }}</button>
      <small>가입 계정에는 관리자가 설정한 일반 사용자 전체 권한이 적용됩니다.</small>
      <p class="auth-alternate">이미 계정이 있습니까? <RouterLink :to="{ path: '/login', query: route.query }">로그인</RouterLink></p>
    </form>
  </div>
</template>
