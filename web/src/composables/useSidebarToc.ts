import { inject, provide, shallowRef, type InjectionKey, type Ref } from 'vue'
import type { TocNode } from '../types/toc'

const sidebarTocKey: InjectionKey<Ref<TocNode[]>> = Symbol('sidebar-toc')

export function provideSidebarToc(): Ref<TocNode[]> {
  const sections = shallowRef<TocNode[]>([])
  provide(sidebarTocKey, sections)
  return sections
}

export function useSidebarToc(): Ref<TocNode[]> {
  const sections = inject(sidebarTocKey)
  if (!sections) throw new Error('Sidebar table of contents provider is missing.')
  return sections
}
