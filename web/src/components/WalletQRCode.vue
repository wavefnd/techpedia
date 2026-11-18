<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{
  value: string
  asset: string
  network: string
}>()

const root = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const failed = ref(false)
const ready = ref(false)
let observer: IntersectionObserver | null = null
let rendered = false

async function renderQRCode() {
  if (rendered || !canvas.value) return
  rendered = true
  try {
    const { default: QRCode } = await import('qrcode')
    await QRCode.toCanvas(canvas.value, props.value, {
      errorCorrectionLevel: 'M',
      margin: 3,
      width: 188,
      color: { dark: '#111827', light: '#ffffff' },
    })
    ready.value = true
  } catch {
    failed.value = true
  }
}

onMounted(() => {
  if (!root.value || !('IntersectionObserver' in window)) {
    void renderQRCode()
    return
  }
  observer = new IntersectionObserver((entries) => {
    if (!entries.some((entry) => entry.isIntersecting)) return
    observer?.disconnect()
    observer = null
    void renderQRCode()
  }, { rootMargin: '200px' })
  observer.observe(root.value)
})

onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <figure ref="root" class="support-wallet-qr">
    <div class="support-wallet-qr-brand"><span aria-hidden="true">T</span><b>TechPedia</b></div>
    <div class="support-wallet-qr-code" :class="{ loading: !ready && !failed }">
      <canvas
        v-show="ready && !failed"
        ref="canvas"
        role="img"
        :aria-label="`${asset} ${network} receiving address QR code`"
      >
        {{ asset }} receiving address QR code
      </canvas>
      <span v-if="!ready && !failed">Generating QR</span>
    </div>
    <div v-if="failed" class="support-wallet-qr-error">QR unavailable</div>
    <figcaption><strong>{{ asset }} · {{ network }}</strong><span>Address only · verify in Phantom</span></figcaption>
  </figure>
</template>
