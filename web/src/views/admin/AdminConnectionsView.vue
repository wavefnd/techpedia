<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plug, ShieldCheck } from '@lucide/vue'
import { request } from '../../services/api'
import { LatestRequest } from '../../services/latestRequest'

type Client = { id: string; name: string; redirect_uri: string; disabled: boolean }
type Grant = { id: string; client_id: string; scope: string; revoked: boolean; expires_at: string; created_at: string }
type Approval = { request: { id: string; expires_at: string; authorization?: { scope: string } }; client: Client }
type Overview = { enabled: boolean; clients: Client[]; grants: Grant[]; resource: string; scope: string; proposal_scope: string }
const route = useRoute()
const router = useRouter()
const data = ref<Overview | null>(null)
const approval = ref<Approval | null>(null)
const name = ref('')
const redirect = ref('')
const error = ref('')
const notice = ref('')
const busy = ref(false)
const loading = ref(false)
const requests = new LatestRequest()
let mounted = true
const post = <T,>(path: string, body: unknown) => request<T>(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

async function load() {
  loading.value = true
  error.value = ''
  await requests.run((signal) => request<Overview>('/api/admin/oauth', { signal }), {
    success: (value) => { data.value = value },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '연결을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}
async function change(work: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try { await work() }
  catch (reason) { if (mounted) error.value = reason instanceof Error ? reason.message : '연결을 변경하지 못했습니다.' }
  finally { if (mounted) busy.value = false }
}
async function register() {
  if (data.value?.enabled !== true || loading.value) return
  if (!name.value.trim() || !redirect.value.trim()) return
  await change(async () => {
    await post<Client>('/api/admin/oauth/clients', { name: name.value.trim(), redirect_uri: redirect.value.trim() })
    if (!mounted) return
    name.value = ''; redirect.value = ''; notice.value = '등록했습니다. 아래 Client ID를 ChatGPT 연결 설정에 사용하세요.'
    await load()
  })
}
async function disable(client: Client) {
  if (!window.confirm(`'${client.name}'의 모든 연결을 중지할까요? 다시 사용할 때는 새로 등록해야 합니다.`)) return
  await change(async () => {
    await post<void>(`/api/admin/oauth/clients/${client.id}/disable`, { confirmation: client.id })
    if (mounted) await load()
  })
}
async function revoke(grant: Grant) {
  if (!window.confirm('이 연결의 접근 권한을 즉시 해제할까요?')) return
  await change(async () => {
    await post<void>(`/api/admin/oauth/grants/${grant.id}/revoke`, { confirmation: grant.id })
    if (mounted) await load()
  })
}
async function decide(allow: boolean) {
  const pending = approval.value
  if (!pending) return
  await change(async () => {
    const result = await post<{ redirect_url: string }>(`/api/admin/oauth/requests/${pending.request.id}/decide`, { allow })
    if (mounted) window.location.assign(result.redirect_url)
  })
}
function clientName(id: string) { return data.value?.clients.find((client) => client.id === id)?.name || '등록된 클라이언트' }
function active(grant: Grant) { return !grant.revoked && new Date(grant.expires_at).getTime() > Date.now() && !data.value?.clients.find((client) => client.id === grant.client_id)?.disabled }

onMounted(async () => {
  await load()
  if (!mounted || data.value?.enabled === false || !data.value || !route.query.client_id) return
  await change(async () => {
    const payload: Record<string, string> = {}
    for (const key of ['client_id', 'redirect_uri', 'response_type', 'code_challenge', 'code_challenge_method', 'scope', 'resource', 'state']) {
      const value = route.query[key]
      if (Array.isArray(value)) throw new Error('중복 연결 요청입니다. ChatGPT에서 다시 연결하세요.')
      payload[key] = typeof value === 'string' ? value : ''
    }
    const value = await post<Approval>('/api/admin/oauth/requests', payload)
    if (!mounted) return
    approval.value = value
    await router.replace('/admin/connections')
  })
})
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view admin-connections-view">
    <header class="admin-page-header"><div><p>연결과 접근 제어</p><h1>ChatGPT 연결</h1><span>관리자만 연결을 승인하고 사용할 수 있습니다. 관리자 권한을 잃으면 접근도 중지됩니다.</span></div><ShieldCheck :size="28" /></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaButton v-if="error && !data" preset="secondary" :disabled="loading" @click="load">다시 불러오기</VaButton>
    <VaAlert v-if="notice" color="success">{{ notice }}</VaAlert>
    <section v-if="approval" class="admin-panel oauth-consent" aria-labelledby="oauth-consent-title">
      <header class="admin-panel-header"><div><span>새 연결 요청</span><h2 id="oauth-consent-title">{{ approval.client.name }} 연결을 승인할까요?</h2></div></header>
      <p>활성 언어판, 검색 결과, 공개 TechMark 원문과 리비전을 읽을 수 있습니다. 관리자 전용 원문·계정 관리·문서 삭제는 허용하지 않습니다.</p>
      <p v-if="approval.request.authorization?.scope?.includes('wiki:propose')"><b>변경 제안 권한을 요청했습니다.</b> 문서 생성·수정·언어 연결 제안을 보관할 수 있습니다. 각 제안은 TechPedia 관리자가 별도로 원문 diff를 검토하고 승인해야 적용됩니다. 이 연결 승인은 문서 변경 승인이 아닙니다.</p>
      <p v-else>읽기 전용 연결입니다. 문서 변경 제안 권한은 없습니다.</p>
      <p><b>응답을 보낼 주소</b><br><code>{{ approval.client.redirect_uri }}</code></p>
      <p class="admin-muted">{{ new Date(approval.request.expires_at).toLocaleTimeString('ko') }}까지 결정하세요. 등록 이름만 신뢰하지 말고 주소도 확인하세요.</p>
      <div class="oauth-actions"><VaButton :disabled="busy" @click="decide(true)">요청 범위로 연결 승인</VaButton><VaButton preset="secondary" :disabled="busy" @click="decide(false)">거절</VaButton></div>
    </section>
    <div v-if="loading && !data" class="admin-loading"><VaProgressCircle indeterminate /> 연결을 불러오는 중입니다.</div>
    <section v-if="data && !data.enabled" class="admin-panel">
      <header class="admin-panel-header"><div><span>서버 설정 필요</span><h2>ChatGPT 연결이 비활성화되어 있습니다</h2></div><Plug :size="21" /></header>
      <p>서버의 <code>.env</code>에 <code>TECHPEDIA_MCP_ENABLED=true</code>를 설정하고 <code>TECHPEDIA_PUBLIC_URL</code>이 실제 HTTPS 사이트 주소인지 확인한 뒤 서버를 다시 시작하세요.</p>
      <p>활성화 후 이 화면에서 클라이언트를 등록하고 연결을 승인할 수 있습니다. 설정이 켜져 있어도 잘못된 공개 주소 등으로 초기화가 실패하면 연결은 비활성 상태를 유지합니다.</p>
      <VaButton preset="secondary" :disabled="loading" @click="load">연결 상태 다시 확인</VaButton>
    </section>
    <template v-if="data && data.enabled">
      <section class="admin-panel">
        <header class="admin-panel-header"><div><span>OAuth 2.1 · PKCE</span><h2>클라이언트 등록</h2></div><Plug :size="21" /></header>
        <p>ChatGPT 연결 설정에 표시된 HTTPS callback URL을 정확히 등록하세요. Client secret은 사용하지 않는 공개 PKCE 클라이언트입니다.</p>
        <dl class="oauth-settings"><dt>MCP 주소</dt><dd><code>{{ data.resource }}</code></dd><dt>읽기 전용 범위</dt><dd><code>{{ data.scope }}</code></dd><dt>읽기 + 변경 제안 범위</dt><dd><code>{{ data.proposal_scope }}</code></dd></dl>
        <p>기존 읽기 연결에는 제안 권한이 자동으로 추가되지 않습니다. ChatGPT에서 <code>wiki:read wiki:propose</code> 범위로 다시 연결하고 위 요청 범위를 승인하세요. 제안은 <RouterLink to="/admin/proposals">ChatGPT 제안 검토</RouterLink>에서 개별 승인합니다. 삭제 도구는 없습니다.</p>
        <form class="oauth-client-form" @submit.prevent="register">
          <VaInput v-model="name" label="연결 이름" input-aria-label="연결 이름" :maxlength="80" :disabled="busy" required />
          <VaInput v-model="redirect" label="HTTPS callback URL" input-aria-label="HTTPS callback URL" :disabled="busy" required />
          <VaButton type="submit" :disabled="busy || !name.trim() || !redirect.trim()">등록</VaButton>
        </form>
      </section>
      <section class="admin-panel">
        <header class="admin-panel-header"><div><span>전체 관리자 공통 설정</span><h2>등록된 클라이언트</h2></div><small>{{ data.clients.length }}개</small></header>
        <div v-for="client in data.clients" :key="client.id" class="oauth-row"><div><b>{{ client.name }}</b><span>{{ client.disabled ? '사용 중지' : '사용 가능' }}</span><small>Client ID · <code>{{ client.id }}</code></small><small>Callback · <code>{{ client.redirect_uri }}</code></small></div><VaButton v-if="!client.disabled" preset="secondary" color="danger" :disabled="busy" @click="disable(client)">전체 연결 중지</VaButton></div>
        <p v-if="!data.clients.length" class="admin-empty">등록된 클라이언트가 없습니다.</p>
      </section>
      <section class="admin-panel">
        <header class="admin-panel-header"><div><span>현재 관리자 계정</span><h2>내 승인 연결</h2></div></header>
        <div v-for="grant in data.grants" :key="grant.id" class="oauth-row"><div><b>{{ clientName(grant.client_id) }}</b><span>{{ active(grant) ? '연결됨' : '만료 또는 해제됨' }} · {{ grant.scope?.includes('wiki:propose') ? '읽기 + 변경 제안' : '읽기 전용' }}</span><small>승인 {{ new Date(grant.created_at).toLocaleString('ko') }} · 만료 {{ new Date(grant.expires_at).toLocaleString('ko') }}</small></div><VaButton v-if="active(grant)" preset="secondary" color="danger" :disabled="busy" @click="revoke(grant)">연결 해제</VaButton></div>
        <p v-if="!data.grants.length" class="admin-empty">승인한 연결이 없습니다. ChatGPT에서 MCP 주소와 Client ID를 설정한 뒤 연결하세요.</p>
      </section>
    </template>
  </div>
</template>
