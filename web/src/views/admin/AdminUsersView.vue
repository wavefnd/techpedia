<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Check } from '@lucide/vue'
import { getUserManagement, setRegularUserPermissions, setUserPermissions } from '../../services/api'
import { account } from '../../services/auth'
import { LatestRequest } from '../../services/latestRequest'
import type { AdminUser, Permission } from '../../types/api'

const users = ref<AdminUser[]>([])
const regularPermissions = ref<Permission[]>([])
const error = ref('')
const saved = ref('')
const saving = ref(new Set<string>())
const savingPolicy = ref(false)
const loading = ref(false)
const ready = ref(false)
const requests = new LatestRequest()
let mounted = true
const policyLocked = computed(() => !ready.value || loading.value || savingPolicy.value || saving.value.size > 0)
const ownerUsers = computed(() => users.value.filter((user) => user.permissions.includes('owner')))
const administratorUsers = computed(() => users.value.filter((user) => user.permissions.includes('admin') && !user.permissions.includes('owner')))
const regularUsers = computed(() => users.value.filter((user) => !user.permissions.includes('admin') && !user.permissions.includes('owner')))
const policyOptions: Array<{ value: Permission; label: string; description: string }> = [
  { value: 'edit', label: '문서 편집', description: '일반 사용자가 문서를 만들고 수정할 수 있습니다.' },
  { value: 'techdata', label: 'Techdata 관리', description: '일반 사용자가 Techdata 항목을 관리할 수 있습니다.' },
]

async function load() {
  if (savingPolicy.value || saving.value.size) return
  loading.value = true
  ready.value = false
  error.value = ''
  await requests.run((signal) => getUserManagement(signal), {
    success: (response) => {
      users.value = response.users.map((user) => ({ ...user, permissions: user.permissions ?? [] }))
      regularPermissions.value = response.regular_permissions ?? []
      ready.value = true
    },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '사용자 목록을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}
onMounted(load)
onBeforeUnmount(() => { mounted = false; requests.dispose() })

async function togglePolicy(permission: Permission, enabled: boolean) {
  if (policyLocked.value) return
  const previous = [...regularPermissions.value]
  regularPermissions.value = enabled
    ? [...new Set([...regularPermissions.value, permission])]
    : regularPermissions.value.filter((value) => value !== permission)
  savingPolicy.value = true
  error.value = ''
  saved.value = ''
  try {
    const response = await setRegularUserPermissions(regularPermissions.value)
    if (!mounted) return
    regularPermissions.value = response.permissions
    regularUsers.value.forEach((user) => { user.permissions = [...response.permissions] })
    saved.value = `일반 사용자 ${response.updated_users}명과 이후 가입 계정의 권한 정책을 저장했습니다.`
  } catch (reason) {
    if (!mounted) return
    regularPermissions.value = previous
    error.value = reason instanceof Error ? reason.message : '전체 권한 정책을 저장하지 못했습니다.'
  } finally { if (mounted) savingPolicy.value = false }
}

async function promote(user: AdminUser) {
  if (!ready.value || loading.value || savingPolicy.value || saving.value.has(user.id)) return
  if (!window.confirm(`${user.display_name} 계정에 관리자 권한을 부여하시겠습니까?`)) return
  saving.value.add(user.id)
  error.value = ''
  saved.value = ''
  try {
    const updated = await setUserPermissions(user.id, [...regularPermissions.value, 'admin'])
    if (!mounted) return
    replaceUser(updated)
    saved.value = `${user.display_name} 계정을 관리자로 지정했습니다.`
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '관리자 권한을 부여하지 못했습니다.'
  } finally { if (mounted) saving.value.delete(user.id) }
}

async function demote(user: AdminUser) {
  if (!ready.value || loading.value || savingPolicy.value || saving.value.has(user.id)) return
  if (!window.confirm(`${user.display_name} 계정의 관리자 권한을 해제하시겠습니까? 일반 사용자 전체 권한이 적용됩니다.`)) return
  saving.value.add(user.id)
  error.value = ''
  saved.value = ''
  try {
    const updated = await setUserPermissions(user.id, regularPermissions.value)
    if (!mounted) return
    replaceUser(updated)
    saved.value = `${user.display_name} 계정을 일반 사용자로 전환했습니다.`
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '관리자 권한을 해제하지 못했습니다.'
  } finally { if (mounted) saving.value.delete(user.id) }
}

function replaceUser(updated: AdminUser) {
  users.value = users.value.map((user) => user.id === updated.id ? { ...updated, permissions: updated.permissions ?? [] } : user)
}

function isCurrentUser(user: AdminUser): boolean {
  return user.username === account.value?.username
}
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header"><div><p>계정</p><h1>사용자와 권한</h1></div><span>전체 사용자 {{ users.length }}명</span></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaButton v-if="error && !ready" preset="secondary" :disabled="loading" @click="load">다시 불러오기</VaButton>
    <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 사용자 목록을 불러오는 중입니다.</div>
    <template v-if="ready">
    <p v-if="saved" class="admin-saved">{{ saved }}</p>
    <VaCard class="regular-permission-policy">
      <VaCardTitle>일반 사용자 전체 권한</VaCardTitle>
      <VaCardContent>
        <p>변경한 권한은 관리자 계정을 제외한 기존 사용자 전체와 이후 가입하는 계정에 동일하게 적용됩니다.</p>
        <div class="admin-policy-options">
          <label v-for="permission in policyOptions" :key="permission.value" class="admin-permission-toggle">
            <input type="checkbox" :checked="regularPermissions.includes(permission.value)" :disabled="policyLocked" @change="togglePolicy(permission.value, ($event.target as HTMLInputElement).checked)">
            <span class="permission-check" aria-hidden="true"><Check :size="14" /></span>
            <span><b>{{ permission.label }}</b><small>{{ permission.description }}</small></span>
          </label>
        </div>
      </VaCardContent>
    </VaCard>
    <h2 class="admin-subheading">소유자</h2>
    <p v-if="ownerUsers.length === 0" class="admin-empty">Bootstrap 사용자명으로 지정된 소유자가 없습니다.</p>
    <div class="admin-user-grid">
      <VaCard v-for="user in ownerUsers" :key="user.id">
        <VaCardContent>
          <div class="admin-user-title"><span><RouterLink :to="`/user/${user.username}`"><b>{{ user.display_name }}</b></RouterLink><small>@{{ user.username }}</small></span><VaBadge text="소유자" color="warning" /></div>
          <div class="admin-user-actions">
            <span>모든 권한 · 변경 불가</span>
            <span v-if="isCurrentUser(user)" class="admin-current-user">현재 계정</span>
          </div>
        </VaCardContent>
      </VaCard>
    </div>

    <h2 class="admin-subheading">관리자 <span>{{ administratorUsers.length }}명</span></h2>
    <p v-if="administratorUsers.length === 0" class="admin-empty">소유자 외 관리자 계정이 없습니다.</p>
    <div class="admin-user-grid">
      <VaCard v-for="user in administratorUsers" :key="user.id">
        <VaCardContent>
          <div class="admin-user-title"><span><RouterLink :to="`/user/${user.username}`"><b>{{ user.display_name }}</b></RouterLink><small>@{{ user.username }}</small></span><VaBadge text="관리자" color="primary" /></div>
          <div class="admin-user-actions">
            <span>관리자 권한</span>
            <span v-if="isCurrentUser(user)" class="admin-current-user">현재 계정</span>
            <VaButton v-else size="small" preset="secondary" color="danger" :disabled="savingPolicy || saving.has(user.id)" :loading="saving.has(user.id)" @click="demote(user)">관리자 해제</VaButton>
          </div>
        </VaCardContent>
      </VaCard>
    </div>

    <h2 class="admin-subheading">일반 사용자 <span>{{ regularUsers.length }}명</span></h2>
    <p v-if="regularUsers.length === 0" class="admin-empty">관리 대상인 일반 사용자가 없습니다.</p>
    <div class="admin-user-grid">
      <VaCard v-for="user in regularUsers" :key="user.id">
        <VaCardContent>
          <div class="admin-user-title"><span><RouterLink :to="`/user/${user.username}`"><b>{{ user.display_name }}</b></RouterLink><small>@{{ user.username }}</small></span></div>
          <div class="admin-user-actions">
            <span>{{ user.permissions.includes('edit') ? '편집 가능' : '읽기 전용' }}</span>
            <VaButton size="small" preset="secondary" :disabled="savingPolicy || saving.has(user.id)" :loading="saving.has(user.id)" @click="promote(user)">관리자로 지정</VaButton>
          </div>
        </VaCardContent>
      </VaCard>
    </div>
    </template>
  </div>
</template>
