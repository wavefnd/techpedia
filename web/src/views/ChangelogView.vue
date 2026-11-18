<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import changelogSource from '../../../CHANGELOG.md?raw'
import { setTrustedHTML } from '../services/trustedHTML'

const content = ref<HTMLElement | null>(null)

onMounted(() => {
  if (!content.value) return
  const rendered = marked.parse(changelogSource, { gfm: true, async: false })
  const sanitized = DOMPurify.sanitize(rendered, { USE_PROFILES: { html: true } })
  setTrustedHTML(content.value, sanitized)
})
</script>

<template>
  <article class="changelog-view">
    <header class="changelog-header">
      <p>TechPedia Engine</p>
      <span>루트 CHANGELOG.md 기준</span>
    </header>
    <div ref="content" class="changelog-content"></div>
  </article>
</template>
