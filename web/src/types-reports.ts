// Types for GET /api/reports/* (internal/reports). Every report echoes the
// resolved range and scope; tables are exported as CSV with ?format=csv&table=.

export type ReportName = 'utilisation' | 'fleet-health' | 'supplies' | 'alerts' | 'firmware' | 'compliance'

export interface ReportRange { from: string; to: string; days: number; group?: string; device?: string; devices: number }

export interface ReportIndex { available: boolean; maxRangeDays: number; reports: { name: ReportName; title: string; tables: string[] }[] }

// --- utilisation --------------------------------------------------------------
export interface UtilDay { day: string; pages: number; mono: number; color: number; devices: number }
export interface UtilDevice {
  deviceId: string; name: string; model: string; location: string; pages: number; mono: number; color: number
  perDay: number; activeDays: number; dataDays: number; resets: number; idle: boolean
}
export interface UtilGroup { groupId: string; name: string; devices: number; pages: number; perDay: number }
export interface UtilSummary {
  pages: number; mono: number; color: number; hasSplit: boolean; perDay: number; perDevicePerDay: number
  devicesWithData: number; devicesNoData: number; idleDevices: number; resets: number
  peakDay?: string; peakPages: number; busiestDevice?: string; busiestDeviceId?: string; busiestDevicePct: number
}
export interface UtilisationReport { range: ReportRange; summary: UtilSummary; days: UtilDay[]; devices: UtilDevice[]; top: UtilDevice[]; groups: UtilGroup[] }

// --- fleet health ---------------------------------------------------------------
export interface HealthDay { day: string; polled: number; reachable: number; unreachable: number; reachablePct: number; ok: number; warning: number; critical: number }
export interface HealthDevice {
  deviceId: string; name: string; model: string; location: string; polls: number; failed: number; availabilityPct: number
  offlineMinutes: number; incidents: number; meanOfflineMinutes: number; worstSeverity: string; daysCritical: number; daysWarning: number
}
export interface HealthSummary {
  devices: number; polled: number; availabilityPct: number; offlineMinutes: number; incidents: number; devicesOffline: number
  daysCritical: number; worstDay?: string; worstDayPct: number
}
export interface HealthReport { range: ReportRange; summary: HealthSummary; days: HealthDay[]; devices: HealthDevice[] }

// --- supplies --------------------------------------------------------------------
export interface SupplyRow {
  deviceId: string; deviceName: string; model: string; index: number; description: string
  startPct: number; nowPct: number; startAt: string; nowAt: string; consumed: number
  ratePerDay: number | null; daysToEmpty: number | null; emptyBy?: string; replacements: number; samples: number
}
export interface ReplacementRow {
  at: string; deviceId: string; deviceName: string; supplyIndex: number; consumableId: string; consumableName: string
  status: string; reason: string; actor: string
}
export interface ConsumptionRow { consumableId: string; name: string; unit: string; unitsOut: number; unitsIn: number; events: number }
export interface SuppliesSummary {
  supplies: number; declining: number; dueWithin30: number; dueWithin7: number; empty: number; replacements: number
  stockEvents: number; unitsOut: number; meanRate: number | null; meanConsumed: number; fastestSupply?: string
}
export interface SuppliesReport {
  range: ReportRange; summary: SuppliesSummary; supplies: SupplyRow[]; replacements: ReplacementRow[]; consumption: ConsumptionRow[]; consumptionScoped: boolean
}

// --- alerts ----------------------------------------------------------------------
export interface AlertDay { day: string; total: number; critical: number; warning: number; bySource: Record<string, number> }
export interface NameCount { name: string; count: number }
export interface AlertDevice {
  deviceId: string; name: string; model: string; location: string; alerts: number; critical: number; warning: number
  acked: number; cleared: number; meanAckMinutes: number | null; meanClearMinutes: number | null
}
export interface AlertsSummary {
  total: number; critical: number; warning: number; acked: number; cleared: number; perDay: number
  meanAckMinutes: number | null; meanClearMinutes: number | null; topSource?: string; peakDay?: string; peakCount: number; devicesAffected: number
}
export interface AlertsReport { range: ReportRange; summary: AlertsSummary; days: AlertDay[]; sources: NameCount[]; devices: AlertDevice[] }

// --- firmware --------------------------------------------------------------------
export type FirmwareStatus = 'current' | 'outdated' | 'unregistered' | 'unknown'
export interface VersionCount { version: string; count: number; outdated: boolean }
export interface FirmwareModel { make: string; model: string; devices: number; versions: VersionCount[]; latest?: string; outdated: number; unknown: number }
export interface FirmwareDevice { deviceId: string; name: string; make: string; model: string; location: string; installed: string; latest?: string; status: FirmwareStatus }
export interface FirmwareSummary { devices: number; versions: number; models: number; current: number; outdated: number; unregistered: number; unknown: number; packages: number }
export interface FirmwareReport { range: ReportRange; summary: FirmwareSummary; models: FirmwareModel[]; devices: FirmwareDevice[] }

// --- compliance (hook) -----------------------------------------------------------
export interface ComplianceReport { range: ReportRange; available: boolean; reason?: string }
