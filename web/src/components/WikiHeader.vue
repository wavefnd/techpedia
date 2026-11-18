<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Languages, Menu, Moon, Network, Sun } from '@lucide/vue'
import { account, hasPermission, loadSession, logout as endSession } from '../services/auth'
import { useWikiI18n } from '../i18n'
import { useTheme } from '../composables/useTheme'
import WikiSearch from './WikiSearch.vue'

const emit = defineEmits<{ 'toggle-navigation': [] }>()
const router = useRouter()
const { locale, mainPageName, t, wikiURL } = useWikiI18n()
const { theme, toggleTheme } = useTheme()
const graphURL = computed(() => `/${locale.value}/graph`)
async function logout() {
  await endSession()
  await router.push(wikiURL(mainPageName.value))
}

onMounted(() => { void loadSession().catch(() => {}) })
</script>

<template>
  <header class="site-header">
    <div class="desktop-site-header">
      <button class="desktop-nav-button icon-button" type="button" :title="t('menu')" :aria-label="t('menu')" @click="emit('toggle-navigation')">
        <Menu :size="20" />
      </button>
      <div class="desktop-brand">
        <RouterLink class="wordmark" :to="wikiURL(mainPageName)">
          <span class="wordmark-symbol">T</span>
          <span><strong>TechPedia</strong><small>{{ t('tagline') }}</small></span>
        </RouterLink>
        <RouterLink class="language-portal-toggle icon-button" to="/" :title="t('languages')" :aria-label="t('languages')">
          <Languages :size="18" />
        </RouterLink>
      </div>

      <WikiSearch />

      <nav class="header-actions" :aria-label="t('userActions')">
        <RouterLink class="graph-toggle icon-button" :to="graphURL" :title="t('graph')" :aria-label="t('graph')">
          <Network :size="18" />
        </RouterLink>
        <button class="theme-toggle icon-button" type="button" :title="t('theme')" :aria-label="t('theme')" :aria-pressed="theme === 'dark'" @click="toggleTheme">
          <Sun v-if="theme === 'dark'" :size="18" />
          <Moon v-else :size="18" />
        </button>
        <details v-if="account" class="account-menu">
          <summary>{{ account.display_name }}</summary>
          <div>
            <RouterLink to="/account">{{ t('account') }}</RouterLink>
            <RouterLink v-if="hasPermission('admin')" to="/admin">{{ t('adminPanel') }}</RouterLink>
            <button type="button" @click="logout">{{ t('logout') }}</button>
          </div>
        </details>
        <template v-else>
          <RouterLink to="/login">{{ t('login') }}</RouterLink>
          <RouterLink class="signup-link" to="/signup">{{ t('signup') }}</RouterLink>
        </template>
      </nav>
    </div>

    <div class="mobile-site-header">
      <div class="mobile-topbar">
        <button class="mobile-nav-button icon-button" type="button" :title="t('menu')" :aria-label="t('menu')" @click="emit('toggle-navigation')">
          <Menu :size="21" />
        </button>
        <div class="mobile-brand">
          <RouterLink class="mobile-wordmark" :to="wikiURL(mainPageName)" aria-label="TechPedia">
            <span class="wordmark-symbol">T</span><strong>TechPedia</strong>
          </RouterLink>
          <RouterLink class="language-portal-toggle icon-button" to="/" :title="t('languages')" :aria-label="t('languages')">
            <Languages :size="18" />
          </RouterLink>
        </div>
        <div class="mobile-header-actions">
          <RouterLink class="graph-toggle icon-button" :to="graphURL" :title="t('graph')" :aria-label="t('graph')"><Network :size="19" /></RouterLink>
          <button class="theme-toggle mobile-theme-toggle icon-button" type="button" :title="t('theme')" :aria-label="t('theme')" :aria-pressed="theme === 'dark'" @click="toggleTheme">
            <Sun v-if="theme === 'dark'" :size="19" />
            <Moon v-else :size="19" />
          </button>
        </div>
      </div>
      <WikiSearch mobile />
    </div>
  </header>
</template>
