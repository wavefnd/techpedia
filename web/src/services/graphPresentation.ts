import type { GraphResponse } from '../types/api'
import type { NodeLabelDrawingFunction } from 'sigma/rendering'

// Spread nodes without interpreting links, languages or subjects before physics runs.
export function graphInitialPositions(value: GraphResponse): Map<string, { x: number; y: number }> {
  const positions = new Map<string, { x: number; y: number }>()
  value.nodes.forEach((node) => {
    positions.set(node.id, graphInitialPosition(node.url, value.nodes.length))
  })
  return positions
}

export function graphInitialPosition(url: string, count: number): { x: number; y: number } {
  const extent = Math.max(40, Math.sqrt(count) * 18)
  return { x: (hash('x:' + url) / 4294967296 - 0.5) * extent * 2, y: (hash('y:' + url) / 4294967296 - 0.5) * extent * 2 }
}

function hash(value: string): number {
  let result = 2166136261
  for (const character of value) result = Math.imul(result ^ character.codePointAt(0)!, 16777619)
  result ^= result >>> 16
  result = Math.imul(result, 0x85ebca6b)
  result ^= result >>> 13
  result = Math.imul(result, 0xc2b2ae35)
  return (result ^ result >>> 16) >>> 0
}

// Build once per response, never scan all edges in a render-frame reducer.
export function graphNeighborhoods(value: GraphResponse): Map<string, Set<string>> {
  const neighbors = new Map(value.nodes.map((node) => [node.id, new Set<string>()]))
  for (const edge of value.edges) {
    if (edge.source === edge.target || !neighbors.has(edge.source) || !neighbors.has(edge.target)) continue
    neighbors.get(edge.source)!.add(edge.target)
    neighbors.get(edge.target)!.add(edge.source)
  }
  return neighbors
}

export function graphPalette(dark: boolean) {
  return dark
    ? { node: '#9aafc2', edge: '#3b4652', translation: '#3b4652', faded: '#39434e', focus: '#83b5de' }
    : { node: '#4f657a', edge: '#dce2e9', translation: '#dce2e9', faded: '#d2d8df', focus: '#477ead' }
}

// A soft ceiling keeps hubs distinct without letting one node fill the canvas.
export function graphNodeRadius(degree: number): number {
  const count = Number.isFinite(degree) ? Math.max(0, degree) : 0
  return 3 + 20 * -Math.expm1(-count / 65)
}

export function graphNodeTextColor(color: string, fallback: string): string {
  const match = /^#([\da-f]{6})$/i.exec(color)
  if (!match) return fallback
  const luminance = relativeLuminance(match[1]!)
  const dark = '#111827', light = '#f8fafc'
  const darkContrast = (luminance + 0.05) / (relativeLuminance(dark.slice(1)) + 0.05)
  const lightContrast = (relativeLuminance(light.slice(1)) + 0.05) / (luminance + 0.05)
  // Mid-tone custom colors can require pure black/white to retain 4.5:1.
  if (Math.max(darkContrast, lightContrast) < 4.5) return luminance > 0.179 ? '#000000' : '#ffffff'
  return darkContrast >= lightContrast ? dark : light
}

function relativeLuminance(hex: string): number {
  const channels = [0, 2, 4].map((offset) => {
    const channel = parseInt(hex.slice(offset, offset + 2), 16) / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  return channels[0]! * 0.2126 + channels[1]! * 0.7152 + channels[2]! * 0.0722
}

type InsideLabel = { lines: string[]; fontSize: number; lineHeight: number }

function wrapLabel(context: CanvasRenderingContext2D, tokens: string[], widths: number[]): string[] | null {
  const lines: string[] = []
  let cursor = 0
  for (const width of widths) {
    let end = cursor
    let lastSpace = -1
    let text = ''
    while (end < tokens.length) {
      const candidate = text + tokens[end]
      if (context.measureText(candidate).width > width) break
      text = candidate
      if (/^\s+$/u.test(tokens[end]!)) lastSpace = end
      end++
    }
    if (end === cursor) return null
    if (end < tokens.length && lastSpace > cursor) end = lastSpace
    lines.push(tokens.slice(cursor, end).join('').trim())
    cursor = end
    while (cursor < tokens.length && /^\s+$/u.test(tokens[cursor]!)) cursor++
    if (cursor === tokens.length) return lines
  }
  return null
}

function fitInsideLabel(context: CanvasRenderingContext2D, text: string, radius: number, weight: string, font: string, segmenter: Intl.Segmenter | null): InsideLabel | null {
  if (radius < 18 || !segmenter) return null
  const tokens = Array.from(segmenter.segment(text), (part) => part.segment)
  const safeRadius = radius - 4
  for (const fontSize of [14, 13, 12]) {
    context.font = `${weight} ${fontSize}px ${font}`
    const lineHeight = fontSize * 1.2
    for (let count = 1; count <= 3; count++) {
      const widths = Array.from({ length: count }, (_, line) => {
        const extent = Math.abs((line - (count - 1) / 2) * lineHeight) + lineHeight / 2
        return extent >= safeRadius ? 0 : 2 * Math.sqrt(safeRadius ** 2 - extent ** 2)
      })
      const lines = wrapLabel(context, tokens, widths)
      // Centering fewer lines would change their chord widths; try that count first.
      if (lines?.length === count) return { lines, fontSize, lineHeight }
    }
  }
  return null
}

export function graphLabel(ink: string, paper: string, direction: 'ltr' | 'rtl' = 'ltr'): NodeLabelDrawingFunction {
  const segmenter = typeof Intl.Segmenter === 'function' ? new Intl.Segmenter(undefined, { granularity: 'word' }) : null
  const cache = new Map<string, InsideLabel | null>()
  return (context, data, settings) => {
    if (!data.label) return
    context.save()
    context.direction = direction
    context.textBaseline = 'middle'
    // Round down so cached layouts always fit the actual screen-space circle.
    const radius = Math.floor(data.size * 2) / 2
    const key = `${radius}:${settings.labelWeight}:${settings.labelFont}:${data.label}`
    let inside = cache.get(key)
    if (inside === undefined) {
      inside = fitInsideLabel(context, data.label, radius, settings.labelWeight, settings.labelFont, segmenter)
      if (cache.size >= 256) cache.delete(cache.keys().next().value!)
      cache.set(key, inside)
    }
    if (inside) {
      context.font = `${settings.labelWeight} ${inside.fontSize}px ${settings.labelFont}`
      context.textAlign = 'center'
      context.fillStyle = graphNodeTextColor(data.color, ink)
      inside.lines.forEach((line, index) => {
        context.fillText(line, data.x, data.y + (index - (inside.lines.length - 1) / 2) * inside.lineHeight)
      })
      context.restore()
      return
    }
    context.font = `${settings.labelWeight} ${settings.labelSize}px ${settings.labelFont}`
    context.textAlign = direction === 'rtl' ? 'right' : 'left'
    context.lineJoin = 'round'
    context.lineWidth = 4
    context.strokeStyle = paper
    context.fillStyle = ink
    const x = data.x + (direction === 'rtl' ? -1 : 1) * (data.size + 6)
    context.strokeText(data.label, x, data.y)
    context.fillText(data.label, x, data.y)
    context.restore()
  }
}
