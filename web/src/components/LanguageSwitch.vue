<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref } from 'vue'
import { X } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import type { LanguageLink } from '../types/api'
import { documentURL, languageDisplayName, useWikiI18n } from '../i18n'
import { activeLanguageEditions, loadLanguageEditions } from '../services/editions'
import { hasPermission, isLoggedIn } from '../services/auth'

const DocumentTechdataControl = defineAsyncComponent(() => import('./DocumentTechdataControl.vue'))

const props = withDefaults(defineProps<{
  current: string
  name: string
  languages: LanguageLink[]
  allowCreate?: boolean
}>(), { allowCreate: true })
const emit = defineEmits<{ techdataChanged: [] }>()
const { locale, t } = useWikiI18n()
const route = useRoute()
const router = useRouter()
const details = ref<HTMLDetailsElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const menuStyle = ref<Record<string, string>>({ visibility: 'hidden' })
const translationOpen = ref(false)
const targetLocale = ref('')
const targetName = ref('')
const linkedLocales = computed(() => new Set(props.languages.map((language) => language.locale.toLowerCase())))
const availableEditions = computed(() => activeLanguageEditions.value.filter((edition) => edition.tag.toLowerCase() !== props.current.toLowerCase() && !linkedLocales.value.has(edition.tag.toLowerCase())))
const targetURL = computed(() => targetLocale.value && targetName.value.trim() ? documentURL('wiki', targetLocale.value, targetName.value) : '')
let positionFrame = 0

onBeforeUnmount(() => stopPositionTracking())

function positionMenu() {
  window.cancelAnimationFrame(positionFrame)
  positionFrame = window.requestAnimationFrame(() => {
    if (!details.value?.open || !menu.value) return
    const anchor = details.value.getBoundingClientRect()
    const viewportWidth = document.documentElement.clientWidth
    const viewportHeight = window.innerHeight
    const edge = 12
    const gap = 6
    if (anchor.bottom < 0 || anchor.top > viewportHeight) {
      details.value.open = false
      return
    }
    const width = Math.min(320, viewportWidth - edge * 2)
    const rtl = getComputedStyle(details.value).direction === 'rtl'
    const preferredLeft = rtl ? anchor.right - width : anchor.left
    const left = Math.max(edge, Math.min(preferredLeft, viewportWidth - width - edge))
    const naturalHeight = menu.value.scrollHeight
    const roomBelow = viewportHeight - anchor.bottom - edge - gap
    const roomAbove = anchor.top - edge - gap
    const openAbove = roomBelow < Math.min(naturalHeight, 260) && roomAbove > roomBelow
    const availableHeight = Math.max(120, openAbove ? roomAbove : roomBelow)
    const height = Math.min(naturalHeight, availableHeight)
    const top = openAbove ? Math.max(edge, anchor.top - height - gap) : anchor.bottom + gap

    menuStyle.value = {
      left: `${Math.round(left)}px`,
      top: `${Math.round(top)}px`,
      width: `${Math.round(width)}px`,
      maxHeight: `${Math.round(availableHeight)}px`,
      visibility: 'visible',
    }
  })
}

function startPositionTracking() {
  window.addEventListener('resize', positionMenu)
  window.addEventListener('scroll', positionMenu, true)
}

function stopPositionTracking() {
  window.cancelAnimationFrame(positionFrame)
  window.removeEventListener('resize', positionMenu)
  window.removeEventListener('scroll', positionMenu, true)
}

async function handleToggle() {
  if (!details.value?.open) {
    stopPositionTracking()
    menuStyle.value = { visibility: 'hidden' }
    return
  }
  await nextTick()
  positionMenu()
  startPositionTracking()
}

async function openTranslation() {
  details.value && (details.value.open = false)
  if (!isLoggedIn.value) {
    await router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  if (!hasPermission('edit')) return
  await loadLanguageEditions()
  targetLocale.value = availableEditions.value[0]?.tag || ''
  targetName.value = ''
  translationOpen.value = true
}

function closeTranslation() {
  translationOpen.value = false
}

function closeLanguageMenu() {
  if (details.value) details.value.open = false
}

async function createTranslation() {
  const name = targetName.value.trim()
  if (!targetLocale.value || !name) return
  await router.push({
    path: documentURL('edit', targetLocale.value, name),
    query: { translation_from_locale: props.current, translation_from_name: props.name },
  })
  closeTranslation()
}
</script>

<template>
  <details ref="details" class="language-switch" @toggle="handleToggle">
    <summary>{{ t('languages') }} {{ languages.length }}</summary>
    <div ref="menu" :style="menuStyle">
      <RouterLink
        v-for="language in languages"
        :key="language.locale"
        :to="language.url"
        :class="{ active: language.locale === current, missing: !language.exists }"
      >
        <span>{{ language.locale }}</span>
        <strong>{{ languageDisplayName(language.locale, locale) }} · {{ language.name }}</strong>
      </RouterLink>
      <button v-if="allowCreate" class="language-add-link" type="button" :disabled="isLoggedIn && !hasPermission('edit')" @click="openTranslation"><span>+</span><strong>{{ isLoggedIn ? t('addLanguage') : t('login') }}</strong></button>
      <DocumentTechdataControl v-if="allowCreate && hasPermission('admin')" :locale="current" :name="name" @open="closeLanguageMenu" @changed="emit('techdataChanged')" />
    </div>
  </details>
  <Teleport to="body">
    <div v-if="translationOpen" class="translation-dialog-backdrop" @mousedown.self="closeTranslation">
      <form class="translation-dialog" @submit.prevent="createTranslation">
        <header><div><p>{{ t('languages') }}</p><h2>{{ t('addLanguage') }}</h2></div><button type="button" :aria-label="t('close')" :title="t('close')" @click="closeTranslation"><X :size="20" /></button></header>
        <label>{{ t('currentArticle') }}<strong>{{ current }} · {{ name }}</strong></label>
        <label>{{ t('languages') }}<select v-model="targetLocale" required><option v-for="edition in availableEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} · {{ edition.tag }}</option></select></label>
        <label>{{ t('documentName') }}<input v-model="targetName" type="text" maxlength="200" :placeholder="t('documentName')" required></label>
        <code v-if="targetURL">{{ targetURL }}</code>
        <p v-if="!availableEditions.length" class="translation-empty">{{ t('noResults') }}</p>
        <footer><button type="button" @click="closeTranslation">{{ t('cancel') }}</button><button type="submit" :disabled="!availableEditions.length || !targetName.trim()">{{ t('edit') }}</button></footer>
      </form>
    </div>
  </Teleport>
</template>
