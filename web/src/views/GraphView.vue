<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowUpRight, Maximize2, Minus, Pause, Play, Plus, Search, X } from '@lucide/vue'
import { MultiDirectedGraph } from 'graphology'
import Sigma from 'sigma'
import { getGraph } from '../services/api'
import { activeLanguageEditions, loadLanguageEditions } from '../services/editions'
import { canonicalLanguageTag, useWikiI18n } from '../i18n'
import { useTheme } from '../composables/useTheme'
import { graphInitialPosition, graphLabel, graphNodeRadius, graphPalette } from '../services/graphPresentation'
import { LatestRequest } from '../services/latestRequest'
import { GraphLayoutSession } from '../services/graphLayout'
import type { GraphLayoutState } from '../services/graphLayout'
import type { GraphNode, GraphResponse } from '../types/api'
import '../styles/graph.css'

const route = useRoute()
const router = useRouter()
const { locale, t } = useWikiI18n()
const { theme } = useTheme()
const container = ref<HTMLElement | null>(null)
// The immutable snapshot can be large; do not proxy every node/edge.
const payload = shallowRef<GraphResponse | null>(null)
const loading = ref(true)
const preparing = ref(false)
const error = ref('')
const query = ref('')
const selectedNode = ref('')
const searchOpen = ref(false)
const neighborLimit = ref(60)
const layoutState = ref<GraphLayoutState>('paused')

const request = new LatestRequest()
let renderer: Sigma | null = null
let layoutSession: GraphLayoutSession | null = null
let resumeOnVisible = false
let reloadOnVisible = false
let hoveredNode = ''
let focusedNodes = new Set<string>()
let neighborhoods = new Map<string, Set<string>>()
let palette = graphPalette(theme.value === 'dark')
let renderGeneration = 0
let nodeByID = new Map<string, GraphNode>()
const searchIndex = shallowRef<{ node: GraphNode; title: string }[]>([])

function graphFrameInterval(): number {
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory
  const cores = navigator.hardwareConcurrency || 2
  if (window.matchMedia('(max-width: 680px)').matches || (memory && memory <= 2) || cores <= 2) return 100
  if ((memory && memory <= 4) || cores <= 4) return 66
  return 33
}

const frameInterval = graphFrameInterval()
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

const routeLocale = computed(() => route.name === 'graph-language' ? canonicalLanguageTag(route.params.locale) : '')
const collator = computed(() => new Intl.Collator(locale.value))
const matches = computed(() => {
  const needle = query.value.trim().toLocaleLowerCase(locale.value)
  if (!needle || !payload.value) return []
  const top: typeof searchIndex.value = []
  const compare = (a: typeof searchIndex.value[number], b: typeof searchIndex.value[number]) => Number(b.title === needle) - Number(a.title === needle)
    || b.node.degree - a.node.degree || collator.value.compare(a.node.name, b.node.name)
  for (const entry of searchIndex.value) {
    if (!entry.title.includes(needle)) continue
    let index = 0
    while (index < top.length && compare(top[index]!, entry) <= 0) index++
    if (index < 8) { top.splice(index, 0, entry); if (top.length > 8) top.pop() }
  }
  return top.map(({ node }) => node)
})
const selected = computed(() => payload.value && nodeByID.get(selectedNode.value))
const neighbors = computed(() => {
  const ids = neighborhoods.get(selectedNode.value)
  return Array.from(ids || [], (id) => nodeByID.get(id)!)
    .sort((a, b) => collator.value.compare(a.name, b.name))
})
const visibleNeighbors = computed(() => neighbors.value.slice(0, neighborLimit.value))
const layoutActive = computed(() => layoutState.value === 'running' || layoutState.value === 'stable')
const layoutStatus = computed(() => t(layoutState.value === 'running' ? 'graphLayoutRunning' : layoutState.value === 'stable' ? 'graphLayoutStable' : layoutState.value === 'failed' ? 'graphLayoutFailed' : 'graphLayoutPaused'))

function destroyGraph() {
  renderGeneration++
  layoutSession?.dispose()
  layoutSession = null
  layoutState.value = 'paused'
  resumeOnVisible = false
  renderer?.kill()
  renderer = null
  hoveredNode = ''
  focusedNodes.clear()
  nodeByID.clear()
  neighborhoods.clear()
  searchIndex.value = []
}

function refreshFocus() {
  const focused = selectedNode.value || hoveredNode
  focusedNodes = new Set(neighborhoods.get(focused))
  if (focused) focusedNodes.add(focused)
  renderer?.refresh()
}

function updateAppearance() {
  if (!renderer || !container.value) return
  palette = graphPalette(theme.value === 'dark')
  const style = getComputedStyle(container.value)
  const label = graphLabel(style.getPropertyValue('--ink').trim(), style.getPropertyValue('--paper').trim(), style.direction === 'rtl' ? 'rtl' : 'ltr')
  renderer.getGraph().updateEachNodeAttributes((_, attributes) => ({ ...attributes, color: palette.node }), { attributes: ['color'] })
  renderer.setSettings({ labelFont: style.fontFamily, defaultDrawNodeLabel: label, defaultDrawNodeHover: label })
}

async function renderGraph(value: GraphResponse) {
  destroyGraph()
  const generation = renderGeneration
  const current = () => generation === renderGeneration && payload.value === value
  if (!container.value || value.nodes.length === 0) return

  const graph = new MultiDirectedGraph()
  const positions = new Map<string, { x: number; y: number }>()
  const nextNodes = new Map<string, GraphNode>()
  const nextNeighbors = new Map<string, Set<string>>()
  const nextSearch: typeof searchIndex.value = []
  let sliceStart = performance.now()
  const yieldIfBusy = async () => {
    if (performance.now() - sliceStart < 8) return
    // A task boundary lets input, navigation and paint run during large imports.
    await new Promise<void>((resolve) => setTimeout(resolve, 0))
    sliceStart = performance.now()
  }
  for (let index = 0; index < value.nodes.length; index++) {
    if (index % 256 === 0) { await yieldIfBusy(); if (!current()) return }
    const node = value.nodes[index]!
    const position = graphInitialPosition(node.url, value.nodes.length)
    positions.set(node.id, position)
    nextNodes.set(node.id, node)
    nextNeighbors.set(node.id, new Set())
    nextSearch.push({ node, title: node.name.toLocaleLowerCase(node.locale) })
    graph.addNode(node.id, {
      ...position,
      label: node.name,
      url: node.url,
      locale: node.locale,
      size: graphNodeRadius(node.degree),
      color: palette.node,
    })
  }
  for (let index = 0; index < value.edges.length; index++) {
    if (index % 256 === 0) { await yieldIfBusy(); if (!current()) return }
    const edge = value.edges[index]!
    if (!graph.hasNode(edge.source) || !graph.hasNode(edge.target)) continue
    if (edge.source !== edge.target) {
      nextNeighbors.get(edge.source)!.add(edge.target)
      nextNeighbors.get(edge.target)!.add(edge.source)
    }
    graph.addDirectedEdgeWithKey(`${edge.kind}:${index}`, edge.source, edge.target, {
      kind: edge.kind,
      size: edge.kind === 'translation' ? 0.8 : 0.5,
      weight: 1,
    })
  }
  if (!current()) return
  nodeByID = nextNodes
  neighborhoods = nextNeighbors
  searchIndex.value = nextSearch

  renderer = new Sigma(graph, container.value, {
    allowInvalidContainer: false,
    hideEdgesOnMove: value.nodes.length > 900,
    hideLabelsOnMove: value.nodes.length > 1500,
    renderEdgeLabels: false,
    // Physics and rendering share radius units, including responsive overview scaling.
    itemSizesReference: 'positions',
    labelSize: 14,
    labelWeight: '600',
    labelDensity: 0.5,
    labelGridCellSize: 160,
    labelRenderedSizeThreshold: value.nodes.length > 1500 ? 8 : value.nodes.length > 300 ? 6 : 4,
    stagePadding: 48,
    // A fixed zoom floor made individual nodes unreachable in very large graphs.
    minCameraRatio: Math.min(0.03, 1 / Math.sqrt(value.nodes.length)),
    maxCameraRatio: 8,
    nodeReducer: (node, data) => {
      const focused = selectedNode.value || hoveredNode
      if (focused && !focusedNodes.has(node)) return { ...data, color: palette.faded, size: 2, label: '', zIndex: 0 }
      return { ...data, forceLabel: node === focused, highlighted: node === focused, zIndex: node === focused ? 2 : 1 }
    },
    edgeReducer: (edge, data) => {
      const color = data.kind === 'translation' ? palette.translation : palette.edge
      const focused = selectedNode.value || hoveredNode
      if (!focused) return { ...data, color }
      const [source, target] = graph.extremities(edge)
      return source === focused || target === focused
        ? { ...data, color: palette.focus, size: 1.2, zIndex: 1 }
        : { ...data, hidden: true }
    },
    zIndex: true,
  })
  // Sigma normally paints hovered circles over hover text, which hides inside labels.
  const hoverLabels = renderer.getCanvases().hovers
  if (hoverLabels) {
    hoverLabels.style.zIndex = '1'
    hoverLabels.style.pointerEvents = 'none'
  }
  updateAppearance()
  refreshFocus()
  renderer.on('clickNode', ({ node }) => {
    const target = nodeByID.get(node)
    if (target) focusNode(target, false)
  })
  renderer.on('clickStage', clearSelection)
  renderer.on('enterNode', ({ node }) => {
    hoveredNode = node
    if (container.value) container.value.style.cursor = 'pointer'
    if (!selectedNode.value) refreshFocus()
  })
  renderer.on('leaveNode', () => {
    hoveredNode = ''
    if (container.value) container.value.style.cursor = 'grab'
    if (!selectedNode.value) refreshFocus()
  })

  if (value.nodes.length > 1) {
    try {
      const ids = value.nodes.map((node) => node.id)
      const indexes = new Map(ids.map((id, index) => [id, index * 2]))
      layoutSession = new GraphLayoutSession({
        nodes: value.nodes.map((node) => ({ id: node.id, ...positions.get(node.id)!, radius: graphNodeRadius(node.degree) })),
        edges: value.edges,
        interval: Math.max(frameInterval, Math.ceil((value.nodes.length + value.edges.length) / 1000)),
      }, (coordinates) => {
        graph.updateEachNodeAttributes((id, attributes) => {
          const index = indexes.get(id)!
          attributes.x = coordinates[index]!
          attributes.y = coordinates[index + 1]!
          return attributes
        }, { attributes: ['x', 'y'] })
      }, (state) => { layoutState.value = state })
      if (!reducedMotion && !document.hidden) layoutSession.start()
      else if (document.hidden) resumeOnVisible = !reducedMotion
    } catch { layoutState.value = 'failed' }
  }
}

async function loadGraph() {
  request.invalidate()
  destroyGraph()
  preparing.value = false
  payload.value = null
  loading.value = true
  error.value = ''
  query.value = ''
  selectedNode.value = ''
  searchOpen.value = false
  if (document.hidden) { reloadOnVisible = true; return }
  await request.run((signal) => getGraph(routeLocale.value || undefined, signal), {
    success: (graph) => { payload.value = graph },
    error: (reason) => { error.value = reason instanceof Error ? reason.message : t('graphEmpty') },
    settled: () => { loading.value = false },
  })
}

function focusNode(node: GraphNode, moveCamera = true) {
  layoutSession?.pause()
  selectedNode.value = node.id
  neighborLimit.value = 60
  query.value = node.name
  searchOpen.value = false
  refreshFocus()
  const data = renderer?.getNodeDisplayData(node.id)
  if (data && moveCamera) void renderer?.getCamera().animate({ x: data.x, y: data.y, ratio: Math.min(0.65, 20 / Math.sqrt(payload.value?.nodes.length || 1)) }, { duration: reducedMotion ? 0 : 350 })
}

function resetView() {
  query.value = ''
  searchOpen.value = false
  clearSelection()
  void renderer?.getCamera().animatedReset({ duration: reducedMotion ? 0 : 400 })
}

function clearSelection() {
  selectedNode.value = ''
  hoveredNode = ''
  refreshFocus()
}

function toggleLayout() {
  if (layoutActive.value) layoutSession?.pause()
  else if (!document.hidden) layoutSession?.start()
}
function visibilityChanged() {
  if (document.hidden) {
    if (loading.value) { request.invalidate(); reloadOnVisible = true }
    resumeOnVisible = layoutActive.value
    if (resumeOnVisible) layoutSession?.pause('hidden')
  } else if (reloadOnVisible) { reloadOnVisible = false; void loadGraph() }
  else if (resumeOnVisible) { resumeOnVisible = false; layoutSession?.start() }
}

function searchChanged() {
  clearSelection()
  searchOpen.value = true
}

function zoom(inward: boolean) {
  const camera = renderer?.getCamera()
  if (!camera) return
  void camera.animate({ ratio: camera.getBoundedRatio(camera.ratio * (inward ? 0.65 : 1.5)) }, { duration: reducedMotion ? 0 : 200 })
}

function chooseLanguage(event: Event) {
  const tag = (event.target as HTMLSelectElement).value
  if (tag) void router.push({ name: 'graph-language', params: { locale: tag } })
}

onMounted(() => {
  document.addEventListener('visibilitychange', visibilityChanged)
  document.fonts?.addEventListener('loadingdone', updateAppearance)
  void loadLanguageEditions()
  void loadGraph()
})
watch(routeLocale, () => void loadGraph(), { flush: 'sync' })
watch(payload, async (value) => {
  if (!value) return
  preparing.value = true
  await nextTick()
  if (payload.value !== value) return
  try { await renderGraph(value) } catch (reason) {
    if (payload.value !== value) return
    destroyGraph()
    error.value = reason instanceof Error ? reason.message : t('graphEmpty')
  } finally { if (payload.value === value) preparing.value = false }
})
watchEffect(() => {
  document.title = `${t('graph')} - TechPedia`
})
watch(theme, updateAppearance)
onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', visibilityChanged)
  document.fonts?.removeEventListener('loadingdone', updateAppearance)
  request.dispose()
  payload.value = null
  destroyGraph()
})
</script>

<template>
  <section class="graph-view" @keydown.esc="searchOpen = false; clearSelection()">
    <header class="graph-toolbar">
      <div class="graph-title">
        <h1>{{ t('graph') }}</h1>
        <p v-if="payload"><b>{{ payload.stats.visible }}</b> {{ t('graphDocuments') }} <span aria-hidden="true">·</span> <b>{{ payload.stats.links }}</b> {{ t('graphLinks') }}</p>
      </div>

      <nav class="graph-scope" :aria-label="t('graph')">
        <RouterLink :class="{ active: route.name === 'graph-language' }" :to="{ name: 'graph-language', params: { locale } }">{{ t('graphLanguage') }}</RouterLink>
        <RouterLink :class="{ active: route.name === 'graph-all' }" :to="{ name: 'graph-all' }">{{ t('graphAll') }}</RouterLink>
      </nav>

      <label class="graph-language-picker">
        <span class="sr-only">{{ t('languages') }}</span>
        <select :value="routeLocale || locale" @change="chooseLanguage">
          <option v-for="edition in activeLanguageEditions" :key="edition.tag" :value="edition.tag">{{ edition.native_name }} ({{ edition.tag }})</option>
        </select>
      </label>

      <div class="graph-search">
        <Search :size="17" aria-hidden="true" />
        <input v-model="query" type="search" :placeholder="t('graphSearch')" :aria-label="t('graphSearch')" :aria-expanded="searchOpen && !!query.trim()" aria-controls="graph-search-results" @focus="searchOpen = true" @input="searchChanged" @keydown.enter.prevent="matches[0] && focusNode(matches[0])" @keydown.esc.stop="searchOpen = false">
        <div v-if="searchOpen && query.trim()" id="graph-search-results" class="graph-search-results">
          <button v-for="node in matches" :key="node.id" type="button" @click="focusNode(node)"><span>{{ node.name }}</span><small>{{ node.locale }}</small></button>
          <p v-if="matches.length === 0">{{ t('noResults') }}</p>
        </div>
      </div>

      <button class="graph-reset icon-button" type="button" :title="t('graphReset')" :aria-label="t('graphReset')" @click="resetView"><Maximize2 :size="18" /></button>
    </header>

    <div class="graph-stage" :class="{ 'has-selection': selected }">
      <div class="graph-viewport">
      <div ref="container" class="graph-canvas" aria-hidden="true"></div>
      <div v-if="loading || preparing" class="graph-state">{{ t('graphLoading') }}</div>
      <div v-else-if="error" class="graph-state error">{{ error }}</div>
      <div v-else-if="payload?.nodes.length === 0" class="graph-state">{{ t('graphEmpty') }}</div>
      <div v-if="payload?.nodes.length && !loading && !preparing" class="graph-exploration">
        <div class="graph-layout-controls">
          <button type="button" :disabled="payload.nodes.length < 2 || layoutState === 'failed'" :aria-label="t(layoutActive ? 'graphLayoutPause' : 'graphLayoutStart')" @click="toggleLayout"><Pause v-if="layoutActive" :size="16" /><Play v-else :size="16" /></button>
          <span>{{ layoutStatus }}</span>
        </div>
        <p v-if="payload.stats.truncated">{{ t('graphTruncated') }}</p>
      </div>
      <div v-if="payload?.nodes.length && !loading && !preparing" class="graph-zoom">
        <button type="button" :aria-label="`${t('graph')} +`" @click="zoom(true)"><Plus :size="18" /></button>
        <button type="button" :aria-label="`${t('graph')} −`" @click="zoom(false)"><Minus :size="18" /></button>
      </div>
      </div>
      <aside v-if="selected" class="graph-detail" :aria-label="selected.name">
        <header>
          <div><small>{{ selected.locale.toUpperCase() }}</small><h2 :title="selected.name">{{ selected.name }}</h2></div>
          <button type="button" class="icon-button" :aria-label="t('close')" @click="clearSelection"><X :size="18" /></button>
        </header>
        <RouterLink class="graph-open-document" :to="selected.url">{{ t('read') }}<ArrowUpRight :size="16" aria-hidden="true" /></RouterLink>
        <h3>{{ t('graphLinks') }} <span>{{ neighbors.length }}</span></h3>
        <ul class="graph-neighbors">
          <li v-for="node in visibleNeighbors" :key="node.id">
            <button type="button" @click="focusNode(node)"><span>{{ node.name }}</span><small>{{ node.locale }}</small></button>
            <RouterLink :to="node.url" :aria-label="`${node.name} · ${t('read')}`"><ArrowUpRight :size="16" /></RouterLink>
          </li>
          <li v-if="visibleNeighbors.length < neighbors.length"><button type="button" @click="neighborLimit += 60">+ {{ Math.min(60, neighbors.length - neighborLimit) }} {{ t('graphDocuments') }}</button></li>
        </ul>
      </aside>
    </div>
  </section>
</template>
