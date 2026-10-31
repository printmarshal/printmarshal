import type { User } from '../types'

/** How wide a widget is on the 12-column dashboard grid. */
export type WidgetSize = 'kpi' | 'third' | 'half' | 'full'

export interface WidgetDef {
  id: string
  title: string
  size: WidgetSize
  /** Lowest role that sees the widget at all (viewer when unset). */
  minRole?: User['role']
}

/**
 * The widget catalogue, in the built-in order. The server keeps a copy of the
 * ids (internal/api/dashboard.go) to validate stored layouts; keep them in step.
 */
export const CATALOGUE: WidgetDef[] = [
  { id: 'kpi.devices', title: 'Total devices', size: 'kpi' },
  { id: 'kpi.online', title: 'Online / offline', size: 'kpi' },
  { id: 'kpi.alerts', title: 'Critical · warning', size: 'kpi' },
  { id: 'kpi.supplies', title: 'Low / empty supplies', size: 'kpi' },
  { id: 'kpi.stock', title: 'Consumables low on stock', size: 'kpi' },
  { id: 'kpi.jobs', title: 'Jobs running', size: 'kpi' },
  { id: 'security.posture', title: 'Security posture', size: 'half' },
  { id: 'alerts.recent', title: 'Recent alerts', size: 'half' },
  { id: 'fleet.health', title: 'Fleet health', size: 'full' },
  { id: 'devices.attention', title: 'Devices needing attention', size: 'full' },
  { id: 'supplies.low', title: 'Supplies running low', size: 'third' },
  { id: 'jobs.inprogress', title: 'Recent jobs', size: 'third' },
  { id: 'activity.recent', title: 'Recent activity', size: 'third', minRole: 'operator' },
  { id: 'consumables.lowstock', title: 'Consumables low on stock', size: 'half' },
  { id: 'consumables.reorder', title: 'Upcoming replacements & reorder forecast', size: 'half' },
]

export const widgetById = new Map(CATALOGUE.map((w) => [w.id, w]))

const rank: Record<User['role'], number> = { viewer: 0, operator: 1, admin: 2 }

export function roleAllows(role: User['role'], w: WidgetDef): boolean {
  return rank[role] >= rank[w.minRole ?? 'viewer']
}
