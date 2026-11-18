export function revealCollapsibleSection(target: HTMLElement | null): HTMLElement | null {
  if (!target) return null
  const section = target.closest<HTMLDetailsElement>('details.techmark-section')
  if (section && !section.open) section.open = true
  return target
}

export function revealCurrentHash(root?: ParentNode | null): HTMLElement | null {
  const raw = window.location.hash.slice(1)
  if (!raw) return null
  let id = raw
  try {
    id = decodeURIComponent(raw)
  } catch {
    // Keep the raw fragment when it is not valid percent-encoding.
  }
  const target = window.document.getElementById(id)
  if (!target || (root && !root.contains(target))) return null
  revealCollapsibleSection(target)
  const scroll = () => {
    if (target.isConnected && window.location.hash.slice(1) === raw && (!root || root.contains(target))) {
      target.scrollIntoView({ block: 'start', behavior: 'instant' })
      return true
    }
    return false
  }
  window.requestAnimationFrame(() => {
    // Offscreen containment may materialize after the first scroll. Re-align
    // once on the following frame so Firefox keeps the heading below the header.
    if (scroll()) window.requestAnimationFrame(scroll)
  })
  return target
}
