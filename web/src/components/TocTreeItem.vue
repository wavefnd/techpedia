<script setup lang="ts">
import { ChevronDown, ChevronRight } from '@lucide/vue'
import type { TocNode } from '../types/toc'

defineOptions({ name: 'TocTreeItem' })
const props = defineProps<{
  node: TocNode
  collapsed: Set<string>
  collapseLabel: string
  expandLabel: string
}>()
const emit = defineEmits<{ toggle: [id: string]; navigate: [id: string] }>()
</script>

<template>
  <li class="toc-tree-item" :class="`level-${node.level}`">
    <div class="toc-tree-row">
      <button
        v-if="node.children.length"
        type="button"
        :title="collapsed.has(node.id) ? expandLabel : collapseLabel"
        :aria-label="`${node.label}: ${collapsed.has(node.id) ? expandLabel : collapseLabel}`"
        :aria-expanded="!collapsed.has(node.id)"
        @click="emit('toggle', node.id)"
      >
        <ChevronRight v-if="collapsed.has(node.id)" :size="14" />
        <ChevronDown v-else :size="14" />
      </button>
      <span v-else class="toc-tree-spacer" aria-hidden="true"></span>
      <a :href="`#${node.id}`" @click.prevent="emit('navigate', node.id)">{{ node.label }}</a>
    </div>
    <ul v-if="node.children.length && !collapsed.has(node.id)">
      <TocTreeItem
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :collapsed="collapsed"
        :collapse-label="collapseLabel"
        :expand-label="expandLabel"
        @toggle="emit('toggle', $event)"
        @navigate="emit('navigate', $event)"
      />
    </ul>
  </li>
</template>
