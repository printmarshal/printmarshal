export type Severity = 'ok' | 'warning' | 'critical'

export interface Supply {
  index: number; description: string; type: string; class: string; colorant?: string
  level: number; maxCapacity: number; unit?: string; percent: number | null
}
export interface Tray {
  index: number; kind: 'input' | 'output'; name: string; type?: string; status: string
  level: number; maxCapacity: number; percent: number | null; mediaName?: string; mediaSize?: string; mediaType?: string; flags?: string[]
}
export interface DeviceAlert { index: number; severity: Severity; group?: string; groupIndex?: number; code?: string; description: string }
export interface Status {
  online: boolean; state: string; severity: Severity; message?: string; errors?: string[]; alerts?: DeviceAlert[]; uptimeSecs?: number; source?: string; at: string
}
export interface Counters { total: number; mono?: number; color?: number; duplex?: number; powerOn?: number; byMarker?: Record<string, number>; at: string }
export interface Device {
  id: string; address: string; hostname?: string; mac?: string; serial?: string; uuid?: string; make?: string; model?: string
  deviceType?: string; firmware?: string; sysDescr?: string; sysName?: string; enterpriseId?: number; adminUrl?: string
  webUrl?: string; webUrlProbed?: string; webUrlOverride?: string
  capabilities: Record<string, string>; discoveryMethods: string[]; identifyMethods: string[]
  displayName: string; location: string; assetTag: string; notes: string; state: string
  status?: Status; supplies: Supply[]; trays: Tray[]; counters?: Counters
  firstSeen: string; lastSeen: string; lastPolled?: string; createdAt: string; updatedAt: string
}
export interface Alert {
  id: string; deviceId: string; severity: Severity; source: string; key: string; description: string
  raisedAt: string; updatedAt: string; clearedAt?: string; ackedAt?: string; ackedBy?: string
}
export interface Job {
  id: string; type: string; target: Record<string, unknown>; params: Record<string, unknown>; status: string
  total: number; done: number; failed: number; error?: string; createdBy: string; createdAt: string; startedAt?: string; finishedAt?: string
}
export interface JobResult { id: number; jobId: string; deviceId?: string; address?: string; status: string; message?: string; data?: Record<string, unknown>; at: string }
export interface Group { id: string; name: string; description: string; kind: 'static' | 'rule'; rule?: GroupRule; memberIds: string[] }
export interface GroupRule { makeContains?: string; modelContains?: string; locationContains?: string; subnet?: string; deviceType?: string; state?: string; hasCapability?: string }
export interface Credential { id: string; name: string; type: string; scope: string; username?: string; priority: number; createdAt: string }
export interface User { id: string; username: string; role: 'viewer' | 'operator' | 'admin'; createdAt: string }

export type ConsumableType = 'toner' | 'ink' | 'drum' | 'fuser' | 'maintenance_kit' | 'waste' | 'staples' | 'other'
export const consumableTypes: ConsumableType[] = ['toner', 'ink', 'drum', 'fuser', 'maintenance_kit', 'waste', 'staples', 'other']
export type StockStatus = 'ok' | 'low' | 'critical' | 'out'
/** A stock-kept consumable. Every field is operator-owned; scans never write it. */
export interface Consumable {
  id: string; name: string; type: ConsumableType; color: string; partNumber: string; compatibleModels: string[]; unit: string
  onHand: number; warningAt: number; criticalAt: number; location: string; notes: string; reorderUrl: string; unitCost?: number
  createdAt: string; updatedAt: string
}
export interface ForecastSummary { known: boolean; stockDays: number | null; nextNeedDays: number | null; reorderSoon: boolean; reason?: string }
export interface ConsumableRow extends Consumable { status: StockStatus; linkedDevices: number; forecast: ForecastSummary }
export interface StockEvent {
  id: number; consumableId: string; actor: string; kind: string; delta: number; resulting: number; reason?: string
  deviceId?: string; supplyIndex?: number; status: string; at: string
}
export interface Rate { percentPerDay: number; samples: number; spanDays: number; known: boolean; reason?: string }
export interface DeviceForecast { deviceId: string; deviceName: string; supplyIndex: number; description: string; percent: number | null; rate: Rate; daysToEmpty: number | null }
export interface ItemForecast {
  consumableId: string; name: string; onHand: number; status: StockStatus; linkedDevices: number; unitsPerDay: number
  stockDays: number | null; reorderSoon: boolean; known: boolean; reason?: string; devices: DeviceForecast[]; nextNeedDays: number | null
}
/** One installed supply (device slot) decorated with its stock link. */
export interface InstalledSupply {
  deviceId: string; deviceName: string; deviceModel: string; address: string; supply: Supply
  consumable?: Consumable; linkSource?: 'auto' | 'manual' | ''; forecast?: DeviceForecast; suggestion?: Partial<Consumable>
}
export interface ConsumableSettings {
  defaultWarningAt: number; defaultCriticalAt: number; autoDecrement: 'off' | 'prompt' | 'automatic'
  replacementJumpPct: number; forecastMinDays: number; reorderLeadDays: number; autoSuggestFromScans: boolean
}
export interface Sample { deviceId: string; metric: string; label?: string; value: number; at: string }
export interface AuditEntry { id: number; actor: string; action: string; target: string; before?: Record<string, unknown>; after?: Record<string, unknown>; result: string; at: string }
export interface Dashboard {
  summary: { devices: number; byState: Record<string, number>; bySeverity: Record<string, number>; offline: number; activeAlerts: number; criticalAlerts: number; lowSupplies: number; lowStock: number; runningJobs: number }
  alerts: Alert[]; jobs: Job[]; attention: Device[]
  consumables: { lowStock: ConsumableRow[]; reorder: ConsumableRow[]; pendingReplacements: number }
}
export interface Event { type: string; at: string; data: unknown }

export function deviceName(d: Device): string {
  return d.displayName || d.sysName || d.hostname || d.model || d.address
}

/** Make + model without repeating the make when the model already starts with it. */
export function makeModel(d: Pick<Device, 'make' | 'model'>): string {
  const make = d.make || ''
  const model = d.model || ''
  if (!make) return model
  if (!model) return make
  return model.toLowerCase().startsWith(make.toLowerCase()) ? model : `${make} ${model}`
}
