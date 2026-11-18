<script setup lang="ts">
import { onMounted } from 'vue'
import { Globe2, Moon, Sun } from '@lucide/vue'
import { useTheme } from '../composables/useTheme'

const { theme, toggleTheme } = useTheme()

onMounted(() => {
  document.title = 'Page not found - TechPedia'
  document.documentElement.lang = 'en'
  document.documentElement.dir = 'ltr'
  document.documentElement.dataset.writingSystem = 'Latn'
})
</script>

<template>
  <main class="not-found-page">
    <button class="not-found-theme-toggle theme-toggle icon-button" type="button" title="Theme" aria-label="Theme" :aria-pressed="theme === 'dark'" @click="toggleTheme">
      <Sun v-if="theme === 'dark'" :size="19" />
      <Moon v-else :size="19" />
    </button>

    <RouterLink class="not-found-brand" to="/" aria-label="TechPedia language portal">
      <span aria-hidden="true">T</span>
      <strong>TechPedia</strong>
    </RouterLink>

    <section class="not-found-content" aria-labelledby="not-found-title">
      <p class="not-found-code">404</p>
      <h1 id="not-found-title">Page not found</h1>
      <p>The requested page does not exist. Check the address or continue from the language portal.</p>
      <RouterLink class="not-found-portal-link" to="/">
        <Globe2 :size="19" aria-hidden="true" />
        Go to language portal
      </RouterLink>
    </section>

    <footer>Computer science and software engineering</footer>
  </main>
</template>
