<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../services/auth'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')
const signupTarget = {
  path: '/signup',
  query: typeof route.query.redirect === 'string' ? { redirect: route.query.redirect } : {},
}

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/ko/wiki/대문'
    await router.replace(redirect)
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '로그인하지 못했습니다.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="auth-view">
    <form class="auth-form" @submit.prevent="submit">
      <p>TechPedia 계정</p>
      <h1>로그인</h1>
      <label for="login-username">사용자 이름</label>
      <input id="login-username" v-model="username" type="text" autocomplete="username" required autofocus>
      <label for="login-password">비밀번호</label>
      <input id="login-password" v-model="password" type="password" autocomplete="current-password" required>
      <p v-if="error" class="form-error">{{ error }}</p>
      <button type="submit" :disabled="busy">{{ busy ? '로그인 중' : '로그인' }}</button>
      <p class="auth-alternate">계정이 없습니까? <RouterLink :to="signupTarget">회원가입</RouterLink></p>
    </form>
  </div>
</template>
