<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { PanelLeftClose, PanelLeftOpen } from '@lucide/vue'
import TocTreeItem from './TocTreeItem.vue'
import { useWikiI18n } from '../i18n'
import { revealCollapsibleSection } from '../services/sections'
import type { TocNode } from '../types/toc'

const props = defineProps<{ sections: TocNode[] }>()
const emit = defineEmits<{ collapse: [collapsed: boolean] }>()
const { t, uiLocale } = useWikiI18n()
const collapsed = ref(typeof window !== 'undefined' && window.matchMedia('(max-width: 680px)').matches)
const collapsedBranches = ref<Set<string>>(new Set())

onMounted(() => {
  if (collapsed.value) emit('collapse', true)
})

function toggle() {
  collapsed.value = !collapsed.value
  emit('collapse', collapsed.value)
}

function toggleBranch(id: string) {
  const next = new Set(collapsedBranches.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsedBranches.value = next
}

function navigate(id: string) {
  const target = revealCollapsibleSection(window.document.getElementById(id))
  if (!target) return
  history.replaceState(history.state, '', `${location.pathname}${location.search}#${encodeURIComponent(id)}`)
  window.requestAnimationFrame(() => target.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

const branchCopy = computed(() => uiLocale.value === 'ko'
  ? { collapse: '하위 목차 접기', expand: '하위 목차 펼치기' }
  : { collapse: 'Collapse subsections', expand: 'Expand subsections' })

watch(() => props.sections, () => { collapsedBranches.value = new Set() })
</script>

<template>
  <nav v-if="sections.length" class="article-toc" :class="{ collapsed }" :aria-label="t('toc')">
    <div class="toc-heading">
      <strong>{{ t('toc') }}</strong>
      <button type="button" :title="t('toc')" :aria-expanded="!collapsed" @click="toggle">
        <PanelLeftOpen v-if="collapsed" :size="17" />
        <PanelLeftClose v-else :size="17" />
      </button>
    </div>
    <ul v-if="!collapsed" class="toc-tree">
      <TocTreeItem
        v-for="section in sections"
        :key="section.id"
        :node="section"
        :collapsed="collapsedBranches"
        :collapse-label="branchCopy.collapse"
        :expand-label="branchCopy.expand"
        @toggle="toggleBranch"
        @navigate="navigate"
      />
    </ul>
  </nav>
</template>
