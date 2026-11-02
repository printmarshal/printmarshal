// Types for configuration management (spec §8.4). Mirrors internal/model/config.go
// and the catalogue in internal/model/settings_catalog.go.

export type SettingType = 'string' | 'int' | 'bool' | 'enum' | 'list' | 'secret'

/** One canonical catalogue key as served by GET /api/config/catalog. */
export interface CatalogDef { key: string; area: string; name: string; type: SettingType; options?: unknown[]; security: boolean; help?: string }
export interface Catalog { areas: string[]; settings: CatalogDef[] }

export interface SettingChange { key: string; value: unknown }

export interface Profile {
  id: string; name: string; description: string; settings: SettingChange[]
  createdBy: string; createdAt: string; updatedAt: string
}

/** One normalised setting inside a device's configuration document. */
export interface ConfigSetting {
  key: string; area: string; name: string; value: unknown; type: SettingType; options?: unknown[]
  writable: boolean; source: string; readOnlyReason?: string
}
export interface ConfigDocument { deviceId: string; settings: ConfigSetting[]; readAt: string; sources: string[] }
export interface ConfigSnapshot { id: string; deviceId: string; document: ConfigDocument | null; at: string }
export interface DeviceConfigResponse { document: ConfigDocument | null; snapshotId?: string; at?: string; fresh: boolean; contacted?: boolean; notContactedReason?: string }

export type SettingResultStatus = 'written' | 'unchanged' | 'would_write' | 'skipped' | 'failed'
/** Read-back verdict of a written setting after a real apply (audit A20): accepted is not the same as in effect. */
export type WriteOutcome = 'written' | 'verified' | 'diverged' | 'unverifiable'
export interface SettingResult { key: string; status: SettingResultStatus; before?: unknown; after?: unknown; error?: string; driver?: string; outcome?: WriteOutcome; readBack?: unknown }
export interface WriteResult { deviceId: string; dryRun: boolean; results: SettingResult[]; at: string; rollback?: SettingChange[] }
export interface WriteRecord {
  id: string; jobId?: string; profileId?: string; revertOf?: string; actor: string
  changes: SettingChange[]; result: WriteResult; written: number; failed: number; skipped: number; unchanged: number; verified?: number; diverged?: number
}

export interface DriftItem { key: string; name?: string; desired: unknown; current: unknown; missing: boolean }
export interface DriftReport { deviceId: string; profileId: string; items: DriftItem[]; drifted: number; matching: number; at: string }
export interface DriftResponse { reports: DriftReport[]; devices: number; drifted: number }

export interface ConfigDiffItem { key: string; name?: string; area?: string; a: unknown; b: unknown; onlyA?: boolean; onlyB?: boolean }
export interface ConfigDiff { a: string; b: string; items: ConfigDiffItem[]; same: number }

export interface ConfigTarget { deviceIds?: string[]; groupId?: string }

/** `data` of a config.apply job result (per device). */
export interface ApplyResultData { writeId?: string; dryRun?: boolean; results?: SettingResult[]; verified?: number; diverged?: number }

/** Payload of the `config.applied` live event. */
export interface ConfigAppliedEvent { deviceId: string; jobId: string; writeId: string; dryRun: boolean }

/** Display labels for catalogue areas, in the catalogue's order. */
export const areaLabels: Record<string, string> = {
  identity: 'Identity', network: 'Network & I/O', protocols: 'Protocols', security: 'Security', energy: 'Energy / sleep',
  trays: 'Trays & media', panel: 'Control panel', print: 'Print defaults', scan: 'Scan', other: 'Other',
}
export function areaLabel(area: string): string { return areaLabels[area] || area.charAt(0).toUpperCase() + area.slice(1) }

/** Loose value equality, matching model.ValuesEqual: numbers by value, lists element-wise. */
export function valuesEqual(a: unknown, b: unknown): boolean {
  if (typeof a === 'number' || typeof b === 'number') return Number(a) === Number(b) && a !== null && b !== null && a !== '' && b !== ''
  if (typeof a === 'string' || typeof b === 'string') return a === b
  if (typeof a === 'boolean' || typeof b === 'boolean') return a === b
  if (a === null || a === undefined || b === null || b === undefined) return (a ?? null) === (b ?? null)
  return JSON.stringify(a) === JSON.stringify(b)
}
