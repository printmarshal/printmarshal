/** CSV import: the plan the server returns for a preview (POST /api/discovery/import, mode=preview). */
export type ImportOutcome = 'create' | 'update' | 'skip'

export interface ImportHeldBack { field: string; reason: string }

export interface ImportRow {
  row: number
  key: string
  outcome: ImportOutcome
  deviceId?: string
  fields: Record<string, string | boolean>
  heldBack: ImportHeldBack[]
  reason?: string
}

export interface PlanResult {
  separator: ',' | ';'
  ignoredColumns: string[]
  knownColumns: string[]
  totals: { create: number; update: number; skip: number }
  rows: ImportRow[]
}

/** Status of one row in an import job's results. */
export type ImportResultStatus = 'created' | 'updated' | 'skipped' | 'failed'

/** Known columns in template order, with the console's names for them. */
export const importColumns: { key: string; label: string }[] = [
  { key: 'address', label: 'Address' },
  { key: 'hostname', label: 'Hostname' },
  { key: 'serial', label: 'Serial' },
  { key: 'displayName', label: 'Display name' },
  { key: 'location', label: 'Location' },
  { key: 'assetTag', label: 'Asset tag' },
  { key: 'notes', label: 'Notes' },
  { key: 'webUrl', label: 'Web address' },
  { key: 'dnsSync', label: 'DNS sync' },
  { key: 'state', label: 'Lifecycle state' },
  { key: 'group', label: 'Group' },
]

/** Key columns identify a row; they cannot be marked "replace". */
export const importKeyColumns = ['address', 'hostname', 'serial']

export function importColumnLabel(key: string): string {
  return importColumns.find((c) => c.key === key)?.label ?? key
}
