import type { TocNode } from '../types/toc'

// Reuse the displayed article DOM; never build a second full document just
// to extract a few headings. The fallback IDs also belong to the real anchors.
export function collectArticleToc(container: ParentNode | null, sectionLabel: string): TocNode[] {
  if (!container) return []
  const roots: TocNode[] = [], stack: TocNode[] = []
  container.querySelectorAll<HTMLElement>('h2, h3, h4').forEach((heading, index) => {
    if (!heading.id) heading.id = `section-${index + 1}`
    const level = Number(heading.tagName.slice(1))
    const node: TocNode = {
      id: heading.id,
      label: heading.textContent?.trim() || `${sectionLabel} ${index + 1}`,
      level,
      children: [],
    }
    while (stack.length && stack[stack.length - 1].level >= level) stack.pop()
    if (stack.length) stack[stack.length - 1].children.push(node)
    else roots.push(node)
    stack.push(node)
  })
  return roots
}
