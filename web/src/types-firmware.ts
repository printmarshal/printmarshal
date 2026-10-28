// Firmware management types (spec §8.6) mirroring internal/model/firmware.go,
// internal/firmware/{campaign,inventory}.go and internal/api/firmware.go.
// Pure helpers only: no React, no fetch.

import type { Device } from './types'

export interface FirmwarePackage {
  id: string; vendor: string; models: string[]; version: string; minVersion?: string
  fileName: string; size: number; sha256: string; notes?: string; createdBy: string; createdAt: string
}

export type CampaignStatus = 'draft' | 'scheduled' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled'
export type PauseReason = 'canary' | 'threshold' | 'operator' | 'interrupted' | ''

export interface MaintenanceWindow { days?: string[]; start?: string; end?: string; tz?: string }

export interface Campaign {
  id: string; name: string; description?: string; packageId: string; groupId?: string; deviceIds?: string[]
  canaryCount: number; concurrency: number; window?: MaintenanceWindow | null; failureThresholdPercent: number
  status: CampaignStatus; wave: number; confirmed: boolean
  scheduledAt?: string; startedAt?: string; finishedAt?: string
  createdBy: string; createdAt: string; updatedAt: string; error?: string
  dryRun: boolean; allowDowngrade: boolean; verifyTimeoutSecs?: number
  pauseReason?: PauseReason; jobId?: string; startedBy?: string
}

export interface CampaignSummary { total: number; pending: number; skipped: number; updating: number; verifying: number; succeeded: number; failed: number }

/** What the API returns for a campaign: the campaign plus its device summary and recovery guidance. */
export interface CampaignView extends Campaign { summary: CampaignSummary; recoveryNotes: string; packageVersion?: string }

export type CampaignDeviceStatus = 'pending' | 'skipped' | 'updating' | 'verifying' | 'succeeded' | 'failed'

export interface CampaignDevice {
  campaignId: string; deviceId: string; wave: number; status: CampaignDeviceStatus
  fromVersion?: string; toVersion?: string; message?: string; startedAt?: string; finishedAt?: string
}
/** Campaign device joined with the device's identity (GET /api/firmware/campaigns/{id}/devices). */
export interface CampaignDeviceRow extends CampaignDevice { name: string; address: string; model: string }

export interface FirmwareHistoryEntry extends CampaignDevice { campaignName: string; packageId: string; dryRun: boolean }

export interface InventoryEntry {
  make: string; model: string; version: string; count: number
  latest?: string; latestPackageId?: string; outdated: boolean; deviceIds: string[]
}
export interface Inventory { entries: InventoryEntry[]; devices: number; outdated: number; unknown: number }
export interface OutdatedDevice { deviceId: string; name: string; address: string; make: string; model: string; installed: string; latest: string; packageId: string }

/** Body for POST / PATCH /api/firmware/campaigns. Pointers on the server: omit a field to leave it alone. */
export interface CampaignRequest {
  name?: string; description?: string; packageId?: string; groupId?: string; deviceIds?: string[]
  canaryCount?: number; concurrency?: number; window?: MaintenanceWindow; failureThresholdPercent?: number
  scheduledAt?: string; clearScheduledAt?: boolean; verifyTimeoutSecs?: number; allowDowngrade?: boolean
}

export const campaignStatuses: CampaignStatus[] = ['draft', 'scheduled', 'running', 'paused', 'completed', 'failed', 'cancelled']
export const deviceStatuses: CampaignDeviceStatus[] = ['pending', 'updating', 'verifying', 'succeeded', 'failed', 'skipped']
export const weekDays = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const

export function isFinished(c: Pick<Campaign, 'status'>): boolean {
  return c.status === 'completed' || c.status === 'failed' || c.status === 'cancelled'
}
export function isActive(c: Pick<Campaign, 'status'>): boolean {
  return c.status === 'scheduled' || c.status === 'running' || c.status === 'paused'
}

/** Human label for a campaign status, including the pause reason. */
export function campaignStatusLabel(c: Pick<Campaign, 'status' | 'wave' | 'pauseReason'>): string {
  switch (c.status) {
    case 'draft': return 'Draft'
    case 'scheduled': return 'Scheduled'
    case 'running': return c.wave ? `Running · wave ${c.wave}` : 'Running'
    case 'paused':
      switch (c.pauseReason) {
        case 'canary': return 'Awaiting confirmation'
        case 'threshold': return 'Paused · failure threshold'
        case 'interrupted': return 'Paused · interrupted'
        default: return 'Paused by operator'
      }
    case 'completed': return 'Completed'
    case 'failed': return 'Failed'
    case 'cancelled': return 'Cancelled'
  }
}

export function deviceStatusLabel(s: CampaignDeviceStatus): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export function fmtBytes(n: number): string {
  if (!isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export function shortSha(s: string): string {
  return s.length > 12 ? `${s.slice(0, 4)}…${s.slice(-4)}` : s
}

export function windowLabel(w?: MaintenanceWindow | null): string {
  if (!w || (!w.days?.length && !w.start)) return 'Any time'
  const days = w.days?.length ? w.days.map((d) => d.charAt(0).toUpperCase() + d.slice(1)).join(', ') : 'Every day'
  const hours = w.start && w.end ? `${w.start}–${w.end}` : 'all day'
  return `${days} · ${hours}${w.tz ? ` (${w.tz})` : ''}`
}

/** "Package 3.0.0 for Acme LaserPrint" style label. */
export function packageLabel(p: Pick<FirmwarePackage, 'vendor' | 'version' | 'models'>): string {
  return `${p.vendor || 'any vendor'} · ${p.version}${p.models?.length ? ` · ${p.models.join(', ')}` : ''}`
}

/** Canary rule as the server applies it: explicit count, 0 = none, -1/undefined = default (3 or 10 %). */
export function canaryLabel(n: number | undefined, eligible?: number): string {
  if (n === 0) return 'No canary wave'
  if (n === undefined || n < 0) {
    if (eligible === undefined) return 'Default · 3 or 10 %'
    const d = Math.min(eligible, Math.max(3, Math.ceil(eligible / 10)))
    return `Default · ${d} device${d === 1 ? '' : 's'} (3 or 10 %)`
  }
  return `${n} device${n === 1 ? '' : 's'}`
}

// --- version comparison, mirroring internal/firmware/fwversion ---------------

const prefixRe = /^(?:version|firmware|ver|rev|fw|v|r)[\s.:_-]*/
const tokenRe = /\d+|[a-z]+/g
const dateRe = /^(\d{4})[.\-/]?(\d{2})[.\-/]?(\d{2})$/
const prerelease = new Set(['alpha', 'beta', 'rc', 'pre', 'dev', 'snapshot'])

export function normalizeVersion(v: string): string {
  return (v || '').toLowerCase().trim().replace(prefixRe, '').trim()
}

function dateValue(v: string): number | null {
  const m = dateRe.exec(v)
  if (!m) return null
  const y = Number(m[1]), mo = Number(m[2]), d = Number(m[3])
  if (y < 1990 || mo < 1 || mo > 12 || d < 1 || d > 31) return null
  return y * 10000 + mo * 100 + d
}
const isNumeric = (s: string) => /^\d+$/.test(s)
function tailRank(rest: string[]): number {
  for (const t of rest) {
    if (isNumeric(t)) { if (t.replace(/0/g, '') !== '') return 1; continue }
    return prerelease.has(t) ? -1 : 1
  }
  return 0
}
function cmpToken(a: string, b: string): number {
  const na = isNumeric(a), nb = isNumeric(b)
  if (na && nb) { const ia = Number(a), ib = Number(b); return ia < ib ? -1 : ia > ib ? 1 : 0 }
  if (na) return 1
  if (nb) return -1
  return a < b ? -1 : a > b ? 1 : 0
}

/** Compare two firmware versions: -1, 0 or 1. */
export function compareVersions(a: string, b: string): number {
  a = normalizeVersion(a); b = normalizeVersion(b)
  if (a === b) return 0
  if (a === '') return -1
  if (b === '') return 1
  const da = dateValue(a), db = dateValue(b)
  if (da !== null && db !== null) return da < db ? -1 : da > db ? 1 : 0
  const ta = a.match(tokenRe) || [], tb = b.match(tokenRe) || []
  const n = Math.max(ta.length, tb.length)
  for (let i = 0; i < n; i++) {
    if (i >= ta.length) return -tailRank(tb.slice(i))
    if (i >= tb.length) return tailRank(ta.slice(i))
    const c = cmpToken(ta[i], tb[i])
    if (c !== 0) return c
  }
  return 0
}

function fuzzyMatch(a: string, b: string): boolean {
  a = (a || '').toLowerCase().trim(); b = (b || '').toLowerCase().trim()
  if (!a || !b) return false
  return a.includes(b) || b.includes(a)
}
export function modelMatches(models: string[], deviceModel: string): boolean {
  const dm = (deviceModel || '').toLowerCase().trim()
  return models.some((m) => { const x = m.toLowerCase().trim(); return x !== '' && (x === '*' || dm.includes(x)) })
}

/** Does the package target this device's make and model at all (ignoring versions)? */
export function packageApplies(p: Pick<FirmwarePackage, 'vendor' | 'models'>, d: Pick<Device, 'make' | 'model'>): boolean {
  if (p.vendor && !fuzzyMatch(p.vendor, d.make || '')) return false
  return p.models.length === 0 || modelMatches(p.models, d.model || '')
}

export interface Verdict { compatible: boolean; current?: boolean; downgrade?: boolean; reason?: string }

/** Client-side preview of the server's compatibility verdict (fwversion.Check). The server decides at start. */
export function checkCompatibility(p: FirmwarePackage, d: Pick<Device, 'make' | 'model' | 'firmware'>, allowDowngrade: boolean): Verdict {
  if (p.vendor && !fuzzyMatch(p.vendor, d.make || '')) return { compatible: false, reason: `package is for vendor “${p.vendor}”, device make is “${d.make || 'unknown'}”` }
  if (p.models.length > 0 && !modelMatches(p.models, d.model || '')) return { compatible: false, reason: `package models ${p.models.join(', ')} do not match “${d.model || 'unknown'}”` }
  const fw = d.firmware || ''
  if (p.minVersion) {
    if (!fw) return { compatible: false, reason: `requires ${p.minVersion} or newer; installed version unknown` }
    if (compareVersions(fw, p.minVersion) < 0) return { compatible: false, reason: `installed ${fw} is older than the required minimum ${p.minVersion}` }
  }
  if (fw) {
    const c = compareVersions(fw, p.version)
    if (c === 0) return { compatible: true, current: true, reason: `already at ${p.version}` }
    if (c > 0 && !allowDowngrade) return { compatible: false, downgrade: true, reason: `installed ${fw} is newer than ${p.version} (downgrade not allowed)` }
    if (c > 0) return { compatible: true, downgrade: true }
  }
  return { compatible: true }
}

/** Latest registered package that applies to a device, if any. */
export function latestPackageFor(packages: FirmwarePackage[], d: Pick<Device, 'make' | 'model'>): FirmwarePackage | null {
  let best: FirmwarePackage | null = null
  for (const p of packages) if (packageApplies(p, d) && (!best || compareVersions(p.version, best.version) > 0)) best = p
  return best
}
