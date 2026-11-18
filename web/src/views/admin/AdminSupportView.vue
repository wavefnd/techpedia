<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ExternalLink, Plus, Trash2, WalletCards } from '@lucide/vue'
import { getAdminSupportSettings, saveAdminSupportSettings } from '../../services/api'
import { syncPublicSupportSettings } from '../../services/support'
import { LatestRequest } from '../../services/latestRequest'
import type { SupportSettings, SupportWallet } from '../../types/api'

const settings = ref<SupportSettings>({ enabled: false, title: '', description: '', wallets: [] })
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const saved = ref('')
const ready = ref(false)
const requests = new LatestRequest()
let mounted = true

function emptyWallet(): SupportWallet {
  return { asset: '', network: '', address: '', memo: '' }
}

const configuredWallets = computed(() => settings.value.wallets.filter((wallet) => wallet.asset.trim() || wallet.network.trim() || wallet.address.trim() || wallet.memo?.trim()))
const canPublish = computed(() => settings.value.title.trim() !== '' && configuredWallets.value.length > 0 && configuredWallets.value.every((wallet) => wallet.asset.trim() && wallet.network.trim() && wallet.address.trim() && !/\s/.test(wallet.address)))

function addWallet() {
  if (saving.value) return
  if (settings.value.wallets.length < 32) settings.value.wallets.push(emptyWallet())
}

function removeWallet(index: number) {
  if (saving.value) return
  settings.value.wallets.splice(index, 1)
  if (settings.value.wallets.length === 0) addWallet()
}

async function save() {
  if (!ready.value || loading.value || saving.value) return
  if (settings.value.enabled && !canPublish.value) {
    error.value = '공개하려면 페이지 제목과 자산·네트워크·주소가 완성된 지갑이 하나 이상 필요합니다.'
    return
  }
  saving.value = true
  error.value = ''
  saved.value = ''
  try {
    const updated = await saveAdminSupportSettings({
      enabled: settings.value.enabled,
      title: settings.value.title,
      description: settings.value.description,
      wallets: configuredWallets.value,
    })
    if (!mounted) return
    settings.value = updated
    syncPublicSupportSettings(settings.value)
    if (settings.value.wallets.length === 0) settings.value.wallets.push(emptyWallet())
    saved.value = settings.value.enabled ? '후원 페이지를 공개했습니다.' : '후원 페이지 설정을 비공개 상태로 저장했습니다.'
  } catch (reason) {
    if (!mounted) return
    error.value = reason instanceof Error ? reason.message : '후원 페이지 설정을 저장하지 못했습니다.'
  } finally {
    if (mounted) saving.value = false
  }
}

async function load() {
  if (saving.value) return
  loading.value = true; ready.value = false; error.value = ''
  await requests.run((signal) => getAdminSupportSettings(signal), {
    success: (value) => { settings.value = value; if (!settings.value.wallets.length) addWallet(); ready.value = true },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : '후원 페이지 설정을 불러오지 못했습니다.' },
    settled: () => { loading.value = false },
  })
}
onMounted(load)
onBeforeUnmount(() => { mounted = false; requests.dispose() })
</script>

<template>
  <div class="admin-view admin-support-view">
    <header class="admin-page-header">
      <div><p>사이트 운영</p><h1>후원 페이지</h1></div>
      <RouterLink v-if="settings.enabled" class="admin-support-public-link" to="/support" target="_blank"><ExternalLink :size="15" /> 공개 페이지 보기</RouterLink>
    </header>

    <VaAlert color="warning" outline>
      이 화면에는 공개 수신 주소만 입력하세요. 개인 키, 시드 문구, 복구 문구는 어떠한 경우에도 저장하지 마세요.
    </VaAlert>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaButton v-if="error && !ready" :disabled="loading" preset="secondary" @click="load">다시 불러오기</VaButton>
    <VaAlert v-if="saved" color="success">{{ saved }}</VaAlert>

    <div v-if="loading" class="admin-loading"><VaProgressCircle indeterminate /> 설정을 불러오는 중입니다.</div>
    <template v-else-if="ready">
      <VaCard>
        <VaCardTitle>공개 상태</VaCardTitle>
        <VaCardContent class="admin-support-status">
          <div><strong>{{ settings.enabled ? '공개' : '비공개' }}</strong><p>비공개 상태에서는 `/support`가 404를 반환하고 공개 링크와 사이트맵에도 나타나지 않습니다.</p></div>
          <VaSwitch v-model="settings.enabled" color="success" :disabled="saving" />
        </VaCardContent>
      </VaCard>

      <VaCard>
        <VaCardTitle>페이지 내용</VaCardTitle>
        <VaCardContent class="admin-support-copy">
          <VaInput v-model="settings.title" label="페이지 제목" input-aria-label="페이지 제목" :disabled="saving" placeholder="Support TechPedia" maxlength="120" />
          <label>안내문<textarea v-model="settings.description" :disabled="saving" maxlength="2000" placeholder="후원금이 TechPedia 운영에 어떻게 사용되는지 적습니다."></textarea></label>
        </VaCardContent>
      </VaCard>

      <VaCard>
        <VaCardTitle class="admin-support-wallet-title"><span><WalletCards :size="18" /> 암호화폐 주소</span><VaButton preset="secondary" size="small" :disabled="settings.wallets.length >= 32 || saving" @click="addWallet"><Plus :size="15" /> 주소 추가</VaButton></VaCardTitle>
        <VaCardContent class="admin-support-wallets">
          <p class="admin-support-qr-note">공개 페이지는 각 수신 주소만 담은 QR 코드를 자동으로 생성합니다. QR에는 금액과 메모·태그가 포함되지 않습니다.</p>
          <section v-for="(wallet, index) in settings.wallets" :key="index" class="admin-support-wallet">
            <header><strong>주소 {{ index + 1 }}</strong><button type="button" title="주소 삭제" aria-label="주소 삭제" :disabled="saving" @click="removeWallet(index)"><Trash2 :size="17" /></button></header>
            <div class="admin-support-wallet-fields">
              <VaInput v-model="wallet.asset" label="자산" input-aria-label="자산" :disabled="saving" placeholder="BTC, ETH, USDT" maxlength="32" />
              <VaInput v-model="wallet.network" label="네트워크" input-aria-label="네트워크" :disabled="saving" placeholder="Bitcoin, Ethereum, Tron" maxlength="64" />
              <VaInput v-model="wallet.address" class="wallet-address-input" label="수신 주소" input-aria-label="수신 주소" :disabled="saving" placeholder="공개 수신 주소" maxlength="256" />
              <VaInput v-model="wallet.memo" label="메모 / 태그 (선택)" input-aria-label="메모 / 태그 (선택)" :disabled="saving" placeholder="필요한 네트워크에서만 입력" maxlength="128" />
            </div>
          </section>
        </VaCardContent>
      </VaCard>

      <div class="admin-support-actions">
        <span v-if="settings.enabled && !canPublish">공개 조건을 먼저 완성해야 합니다.</span>
        <VaButton :loading="saving" :disabled="saving || (settings.enabled && !canPublish)" @click="save">설정 저장</VaButton>
      </div>
    </template>
  </div>
</template>
