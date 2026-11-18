<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'
import { Check, Copy, Moon, ShieldCheck, Sun } from '@lucide/vue'
import { getSupportSettings } from '../services/api'
import { useTheme } from '../composables/useTheme'
import type { SupportSettings } from '../types/api'
import WalletQRCode from '../components/WalletQRCode.vue'

const settings = ref<SupportSettings | null>(null)
const loading = ref(true)
const unavailable = ref(false)
const copied = ref('')
const { theme, toggleTheme } = useTheme()
let copiedTimer = 0

async function copyValue(key: string, value: string) {
  try {
    await navigator.clipboard.writeText(value)
    copied.value = key
    window.clearTimeout(copiedTimer)
    copiedTimer = window.setTimeout(() => { copied.value = '' }, 1800)
  } catch {
    copied.value = ''
  }
}

onMounted(async () => {
  try {
    settings.value = await getSupportSettings()
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => window.clearTimeout(copiedTimer))

watchEffect(() => {
  document.title = settings.value?.title ? `${settings.value.title} - TechPedia` : 'Support - TechPedia'
  document.documentElement.lang = 'en'
  document.documentElement.dir = 'ltr'
  document.documentElement.dataset.writingSystem = 'Latn'
})
</script>

<template>
  <main class="support-page">
    <button class="support-theme-toggle theme-toggle icon-button" type="button" title="Theme" aria-label="Theme" :aria-pressed="theme === 'dark'" @click="toggleTheme">
      <Sun v-if="theme === 'dark'" :size="19" />
      <Moon v-else :size="19" />
    </button>
    <RouterLink class="support-brand" to="/"><span aria-hidden="true">T</span><b>TechPedia</b></RouterLink>

    <div v-if="loading" class="support-status">Loading support information...</div>
    <section v-else-if="unavailable || !settings" class="support-unavailable">
      <h1>Support page unavailable</h1>
      <p>TechPedia is not currently accepting cryptocurrency support through this page.</p>
      <RouterLink to="/">Return to TechPedia</RouterLink>
    </section>
    <template v-else>
      <header class="support-header">
        <p>Support TechPedia</p>
        <h1>{{ settings.title }}</h1>
        <p v-if="settings.description">{{ settings.description }}</p>
      </header>

      <aside class="support-safety-note">
        <ShieldCheck :size="20" />
        <p><strong>Verify the asset and network before sending.</strong> TechPedia publishes receiving addresses only and will never request a private key or recovery phrase.</p>
      </aside>

      <div class="support-wallet-list">
        <section v-for="(wallet, index) in settings.wallets" :key="`${wallet.asset}:${wallet.network}:${wallet.address}`" class="support-wallet">
          <header><div><h2>{{ wallet.asset }}</h2><span>{{ wallet.network }}</span></div><span>Cryptocurrency</span></header>
          <div class="support-wallet-body">
            <div class="support-wallet-details">
              <div class="support-address-row">
                <div><span>Address</span><code>{{ wallet.address }}</code></div>
                <button type="button" :title="`Copy ${wallet.asset} address`" :aria-label="`Copy ${wallet.asset} address`" @click="copyValue(`address:${index}`, wallet.address)">
                  <Check v-if="copied === `address:${index}`" :size="18" /><Copy v-else :size="18" />
                </button>
              </div>
              <div v-if="wallet.memo" class="support-address-row memo">
                <div><span>Memo / tag</span><code>{{ wallet.memo }}</code></div>
                <button type="button" title="Copy memo or tag" aria-label="Copy memo or tag" @click="copyValue(`memo:${index}`, wallet.memo || '')">
                  <Check v-if="copied === `memo:${index}`" :size="18" /><Copy v-else :size="18" />
                </button>
              </div>
            </div>
            <WalletQRCode :value="wallet.address" :asset="wallet.asset" :network="wallet.network" />
          </div>
        </section>
      </div>
    </template>
  </main>
</template>
