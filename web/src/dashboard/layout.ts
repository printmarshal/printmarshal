import { CATALOGUE, widgetById } from './catalogue'
import type { DashboardLayout, LayoutEntry } from '../types'

export const LAYOUT_VERSION = 1

/**
 * Turns a stored layout (or none) into the full ordered list of entries:
 * listed widgets in their order, then every catalogue widget the layout omits,
 * with defaults. Unknown ids are dropped; repeats keep their first position.
 */
export function effectiveEntries(layout: DashboardLayout | null | undefined): LayoutEntry[] {
  const out: LayoutEntry[] = []
  const seen = new Set<string>()
  for (const e of layout?.widgets ?? []) {
    if (!widgetById.has(e.id) || seen.has(e.id)) continue
    seen.add(e.id)
    out.push({ id: e.id, hidden: !!e.hidden, collapsed: !!e.collapsed })
  }
  for (const w of CATALOGUE) {
    if (!seen.has(w.id)) out.push({ id: w.id, hidden: false, collapsed: false })
  }
  return out
}

export function toLayout(entries: LayoutEntry[]): DashboardLayout {
  return { version: LAYOUT_VERSION, widgets: entries.map((e) => ({ id: e.id, ...(e.hidden ? { hidden: true } : {}), ...(e.collapsed ? { collapsed: true } : {}) })) }
}

export function move(entries: LayoutEntry[], id: string, dir: -1 | 1): LayoutEntry[] {
  const i = entries.findIndex((e) => e.id === id)
  const j = i + dir
  if (i < 0 || j < 0 || j >= entries.length) return entries
  const next = entries.slice()
  ;[next[i], next[j]] = [next[j], next[i]]
  return next
}

export function setFlag(entries: LayoutEntry[], id: string, flag: 'hidden' | 'collapsed', value: boolean): LayoutEntry[] {
  return entries.map((e) => (e.id === id ? { ...e, [flag]: value } : e))
}

export function sameLayout(a: LayoutEntry[], b: LayoutEntry[]): boolean {
  return a.length === b.length && a.every((e, i) => e.id === b[i].id && !!e.hidden === !!b[i].hidden && !!e.collapsed === !!b[i].collapsed)
}
