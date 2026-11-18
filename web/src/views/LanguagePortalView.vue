<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'
import { ArrowDownWideNarrow, ArrowUpNarrowWide, ChevronDown, Globe2, Moon, Search, Sun, X } from '@lucide/vue'
import { languageDisplayName } from '../i18n'
import { activeLanguageEditions } from '../services/editions'
import { publicSupportSettings, supportAvailable } from '../services/support'
import { useTheme } from '../composables/useTheme'
import type { CountryCatalogEntry, LanguageEdition } from '../types/api'

const query = ref('')
const countryCode = ref('')
const countryQuery = ref('')
const countryMenuOpen = ref(false)
const countryOptionIndex = ref(0)
const countryFilter = ref<HTMLElement | null>(null)
const sortAscending = ref(false)
const { theme, toggleTheme } = useTheme()
const englishRegionNames = new Intl.DisplayNames(['en'], { type: 'region' })

function countryName(code: string): string {
  try { return englishRegionNames.of(code) || code } catch { return code }
}

const availableCountries = computed(() => {
  const used = new Set(activeLanguageEditions.value.flatMap((edition) => edition.country_codes || []))
  return Array.from(used, (code): CountryCatalogEntry => ({ code, name: countryName(code) }))
    .sort((a, b) => a.name.localeCompare(b.name, 'en'))
})

const matchingCountries = computed(() => {
  const value = countryQuery.value.trim().toLocaleLowerCase('en')
  if (!value || countryCode.value) return availableCountries.value
  return availableCountries.value.filter((country) => [country.code, country.name, countryName(country.code)].some((item) => item.toLocaleLowerCase('en').includes(value)))
})

function compareEditions(left: LanguageEdition, right: LanguageEdition): number {
  const difference = sortAscending.value
    ? left.document_count - right.document_count
    : right.document_count - left.document_count
  return difference || left.tag.localeCompare(right.tag, 'en')
}

const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  const countryValue = countryQuery.value.trim()
  const matchingCountryCodes = new Set(matchingCountries.value.map((country) => country.code))
  return activeLanguageEditions.value
    .filter((edition) => {
      if (countryCode.value) return edition.country_codes?.includes(countryCode.value)
      if (!countryValue) return true
      return edition.country_codes?.some((code) => matchingCountryCodes.has(code))
    })
    .filter((edition) => !value || [edition.tag, edition.name, edition.native_name, languageDisplayName(edition.tag, 'en')].some((item) => item.toLocaleLowerCase().includes(value)))
    .slice()
    .sort(compareEditions)
})

function openCountryMenu() {
  countryMenuOpen.value = true
  countryOptionIndex.value = 0
}

function handleCountryInput() {
  countryCode.value = ''
  openCountryMenu()
}

function selectCountry(country: CountryCatalogEntry) {
  countryCode.value = country.code
  countryQuery.value = countryName(country.code)
  countryMenuOpen.value = false
}

function clearCountry() {
  countryCode.value = ''
  countryQuery.value = ''
  countryOptionIndex.value = 0
}

function handleCountryKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    countryMenuOpen.value = false
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Enter'].includes(event.key)) return
  event.preventDefault()
  if (!countryMenuOpen.value) {
    openCountryMenu()
    return
  }
  if (matchingCountries.value.length === 0) return
  if (event.key === 'ArrowDown') countryOptionIndex.value = Math.min(countryOptionIndex.value + 1, matchingCountries.value.length - 1)
  if (event.key === 'ArrowUp') countryOptionIndex.value = Math.max(countryOptionIndex.value - 1, 0)
  if (event.key === 'Enter') {
    const country = matchingCountries.value[countryOptionIndex.value]
    if (country) selectCountry(country)
  }
}

function closeCountryMenu(event: PointerEvent) {
  if (!countryFilter.value?.contains(event.target as Node)) countryMenuOpen.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', closeCountryMenu)
})

onBeforeUnmount(() => document.removeEventListener('pointerdown', closeCountryMenu))

watchEffect(() => {
  document.title = 'TechPedia - Technical knowledge wiki'
  document.documentElement.lang = 'en'
  document.documentElement.dir = 'ltr'
  document.documentElement.dataset.writingSystem = 'Latn'
})
</script>

<template>
  <main class="language-portal">
    <button class="portal-theme-toggle theme-toggle icon-button" type="button" title="Theme" aria-label="Theme" :aria-pressed="theme === 'dark'" @click="toggleTheme">
      <Sun v-if="theme === 'dark'" :size="19" />
      <Moon v-else :size="19" />
    </button>
    <header class="portal-header">
      <div class="portal-mark" aria-hidden="true">T</div>
      <h1>TechPedia</h1>
      <p>Technical knowledge wiki</p>
    </header>

    <section class="portal-language-panel" aria-labelledby="portal-languages">
      <div class="portal-section-head">
        <h2 id="portal-languages">Languages</h2>
        <span>{{ filtered.length }} / {{ activeLanguageEditions.length }}</span>
      </div>
      <div class="portal-query-row">
        <label class="portal-search">
          <Search :size="18" aria-hidden="true" />
          <span class="sr-only">Find a language</span>
          <input v-model="query" type="search" placeholder="Find a language">
        </label>
        <div ref="countryFilter" class="portal-country-filter">
          <div class="portal-country-search">
            <Globe2 :size="18" aria-hidden="true" />
            <label class="sr-only" for="portal-country-query">Country or region</label>
            <input
              id="portal-country-query"
              v-model="countryQuery"
              type="search"
              role="combobox"
              autocomplete="off"
              placeholder="Country or region"
              aria-autocomplete="list"
              aria-controls="portal-country-options"
              :aria-expanded="countryMenuOpen"
              :aria-activedescendant="countryMenuOpen && matchingCountries[countryOptionIndex] ? `portal-country-${matchingCountries[countryOptionIndex].code}` : undefined"
              @focus="openCountryMenu"
              @input="handleCountryInput"
              @keydown="handleCountryKeydown"
            >
            <button v-if="countryQuery" type="button" title="Clear country filter" aria-label="Clear country filter" @click="clearCountry"><X :size="16" /></button>
            <ChevronDown v-else :size="16" aria-hidden="true" />
          </div>
          <div v-if="countryMenuOpen" id="portal-country-options" class="portal-country-options" role="listbox" aria-label="Countries and regions">
            <button
              v-for="(country, index) in matchingCountries"
              :id="`portal-country-${country.code}`"
              :key="country.code"
              type="button"
              role="option"
              :aria-selected="country.code === countryCode"
              :class="{ active: index === countryOptionIndex }"
              @mouseenter="countryOptionIndex = index"
              @mousedown.prevent
              @click="selectCountry(country)"
            ><span>{{ countryName(country.code) }}</span><code>{{ country.code }}</code></button>
            <p v-if="matchingCountries.length === 0">No matching country or region.</p>
          </div>
        </div>
        <button
          class="portal-sort-button"
          type="button"
          :title="sortAscending ? 'Sort by most documents first' : 'Sort by fewest documents first'"
          :aria-label="sortAscending ? 'Sort by most documents first' : 'Sort by fewest documents first'"
          :aria-pressed="sortAscending"
          @click="sortAscending = !sortAscending"
        >
          <ArrowUpNarrowWide v-if="sortAscending" :size="19" aria-hidden="true" />
          <ArrowDownWideNarrow v-else :size="19" aria-hidden="true" />
        </button>
      </div>
		<div class="portal-language-grid">
        <RouterLink v-for="edition in filtered" :key="edition.tag" :to="edition.main_page_url || '/'" :lang="edition.tag" :dir="edition.direction">
          <strong>{{ edition.native_name }}</strong>
          <span>{{ languageDisplayName(edition.tag, 'en') }}</span>
          <small>{{ edition.document_count.toLocaleString('en') }} {{ edition.document_count === 1 ? 'article' : 'articles' }}</small>
          <code>{{ edition.tag }}</code>
        </RouterLink>
      </div>
		<p v-if="activeLanguageEditions.length === 0" class="portal-empty">No language edition has been configured. <RouterLink to="/admin">Open the admin panel</RouterLink></p>
		<p v-else-if="filtered.length === 0" class="portal-empty">No matching language edition.</p>
    </section>

    <footer class="portal-footer">
      <span>Computer science and software engineering</span>
      <nav aria-label="Account"><RouterLink v-if="supportAvailable" to="/support">{{ publicSupportSettings?.title }}</RouterLink><RouterLink to="/login">Log in</RouterLink><RouterLink to="/signup">Create account</RouterLink></nav>
    </footer>
  </main>
</template>

<style src="../styles/portal.css"></style>
