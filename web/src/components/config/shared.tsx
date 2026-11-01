import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../../api'
import type { Job, JobResult } from '../../types'
import { useEvents } from '../../ws'
import { Ic } from '../icons'
import { EmptyState, Pill, Seg, Tag } from '../ui'
import type { Catalog, CatalogDef, SettingResultStatus, WriteOutcome } from '../../types-config'

// --- catalogue ----------------------------------------------------------------

let catalogCache: Promise<Catalog> | null = null
/** The canonical setting catalogue, fetched once per page load. */
export function useCatalog(): { catalog: Catalog | null; byKey: Map<string, CatalogDef>; error: string } {
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    catalogCache ||= api.get<Catalog>('/api/config/catalog')
    catalogCache.then(setCatalog).catch((e) => { catalogCache = null; setError((e as Error).message) })
  }, [])
  const byKey = useMemo(() => new Map((catalog?.settings || []).map((d) => [d.key, d])), [catalog])
  return { catalog, byKey, error }
}

export function isSecret(def?: CatalogDef): boolean { return def?.type === 'secret' }

/** Human form of a setting value. Secret keys are never rendered. */
export function fmtValue(v: unknown, def?: CatalogDef): string {
  if (isSecret(def) || v === '<redacted>') return 'never displayed'
  if (v === undefined || v === null || v === '') return 'not set'
  // RFC 3805 prtInputDefaultIndex: -1 is no default input for the printer as a whole.
  if (def?.key === 'trays.defaultTray' && String(v) === '-1') return 'No default set on the printer'
  if (def?.key === 'trays.defaultTray' && String(v) === '-2') return 'Unknown'
  if (typeof v === 'boolean') return v ? 'On' : 'Off'
  if (Array.isArray(v)) return v.length ? v.map(String).join(', ') : 'empty list'
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

export function settingName(key: string, byKey: Map<string, CatalogDef>, fallback?: string): string {
  return byKey.get(key)?.name || fallback || key
}

// --- jobs -----------------------------------------------------------------------

export const jobFinished = (status?: string) => !!status && ['succeeded', 'partial', 'failed', 'cancelled'].includes(status)

/** Follows one job: live `job.updated` events plus polling while it runs. */
export function useJob(jobId: string | null): { job: Job | null; results: JobResult[]; done: boolean; error: string; reload: () => void } {
  const [job, setJob] = useState<Job | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [error, setError] = useState('')
  const seq = useRef(0)
  const reload = useCallback(() => {
    if (!jobId) { setJob(null); setResults([]); return }
    const my = ++seq.current
    api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${jobId}`).then((r) => { if (my === seq.current) { setJob(r.job); setResults(r.results) } }).catch((e) => setError((e as Error).message))
  }, [jobId])
  useEffect(() => { setJob(null); setResults([]); setError(''); reload() }, [reload])
  useEvents((e) => {
    if (e.type === 'job.updated' && (e.data as Job)?.id === jobId) reload()
    if (e.type === 'config.applied' && (e.data as { jobId?: string })?.jobId === jobId) reload()
  }, [jobId, reload])
  const running = !!jobId && !jobFinished(job?.status)
  useEffect(() => {
    if (!running) return
    const t = setInterval(reload, 1500)
    return () => clearInterval(t)
  }, [running, reload])
  return { job, results, done: !!job && jobFinished(job.status), error, reload }
}

/** Polls a job until it finishes (for fire-and-wait flows such as reverts). */
export async function waitForJob(jobId: string, timeoutMs = 180000): Promise<{ job: Job; results: JobResult[] }> {
  const t0 = Date.now()
  for (;;) {
    const r = await api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${jobId}`)
    if (jobFinished(r.job.status)) return r
    if (Date.now() - t0 > timeoutMs) throw new Error('timed out waiting for the job')
    await new Promise((res) => setTimeout(res, 1000))
  }
}

// --- small presentational pieces ----------------------------------------------

/** Step indicator for the apply flow (mockup: numbered circles joined by lines). */
export function Steps({ current, steps }: { current: number; steps: string[] }) {
  return (
    <ol aria-label="Apply steps" style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', listStyle: 'none', margin: 0, padding: 0 }}>
      {steps.map((label, i) => {
        const n = i + 1
        const state = n < current ? 'done' : n === current ? 'cur' : 'todo'
        const color = state === 'cur' ? 'var(--accent)' : state === 'done' ? 'var(--text-2)' : 'var(--text-3)'
        const circle: React.CSSProperties = state === 'cur' ? { background: 'var(--accent)', borderColor: 'var(--accent)', color: 'var(--on-accent)' } : state === 'done' ? { background: 'var(--ok-bg)', borderColor: 'transparent', color: 'var(--ok)' } : { background: 'var(--surface)', borderColor: 'var(--border-strong)' }
        return (
          <li key={label} style={{ display: 'contents' }}>
            <span aria-current={state === 'cur' ? 'step' : undefined} style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontWeight: 500, color }}>
              <span style={{ width: 22, height: 22, borderRadius: '50%', display: 'grid', placeItems: 'center', fontSize: 11, fontWeight: 600, border: '1px solid', ...circle }}>{state === 'done' ? <Ic.check size={12} style={{ strokeWidth: 2.5 }} /> : n}</span>{label}
            </span>
            {i < steps.length - 1 && <span aria-hidden="true" style={{ width: 32, height: 1, background: 'var(--border-strong)' }} />}
          </li>
        )
      })}
    </ol>
  )
}

export function StatTile({ label, value, detail, color }: { label: string; value: React.ReactNode; detail?: React.ReactNode; color?: string }) {
  return (
    <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: '12px 14px', display: 'flex', flexDirection: 'column', gap: 2 }}>
      <span className="cap">{label}</span><span className="num" style={{ fontSize: 22, fontWeight: 600, color }}>{value}</span>{detail && <span className="cap">{detail}</span>}
    </div>
  )
}

/** before → after, struck-through red then green (meaning survives without colour). */
export function Diff({ before, after, def }: { before: unknown; after: unknown; def?: CatalogDef }) {
  return (
    <span style={{ display: 'inline-flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
      <span className="old mono">{fmtValue(before, def)}</span><Ic.arrowRight size={14} style={{ color: 'var(--text-3)' }} /><span className="new-v mono">{fmtValue(after, def)}</span>
    </span>
  )
}

export function ResultPill({ status, xs }: { status: SettingResultStatus | string; xs?: boolean }) {
  switch (status) {
    case 'written': return <Pill tone="ok" xs={xs}>Written</Pill>
    case 'would_write': return <Pill tone="warn" xs={xs} noIcon><Ic.pencil />Would write</Pill>
    case 'unchanged': return <Pill tone="ok" xs={xs}>Already matches</Pill>
    case 'skipped': return <Pill tone="off" xs={xs}>Skipped</Pill>
    case 'failed': return <Pill tone="crit" xs={xs}>Failed</Pill>
    default: return <Pill tone="unk" xs={xs}>{status}</Pill>
  }
}

/** Read-back verdict of a written setting (audit A20): verified = in effect, written = accepted but not re-read, diverged = the device reports something else, unverifiable = no reader for the key. */
export function OutcomePill({ outcome, xs }: { outcome: WriteOutcome | string; xs?: boolean }) {
  switch (outcome) {
    case 'verified': return <Pill tone="ok" xs={xs} title="Read back from the device and equal to the desired value">Verified</Pill>
    case 'written': return <Pill tone="info" xs={xs} title="The device accepted the write; the read-back failed, so it is not verified">Accepted</Pill>
    case 'diverged': return <Pill tone="warn" xs={xs} title="The device accepted the write but reports a different value">Diverged</Pill>
    case 'unverifiable': return <Pill tone="off" xs={xs} title="No driver can read this setting back">Unverifiable</Pill>
    default: return <Pill tone="unk" xs={xs}>{outcome}</Pill>
  }
}

/** Permission state from the component library: write actions are removed, not disabled. */
export function ViewOnly({ what = 'Applying profiles' }: { what?: string }) {
  return <EmptyState tone="muted" icon={<Ic.lock size={20} />} title="View only" text={`Your role is Viewer. ${what} needs Operator — ask an admin.`} action={<Tag>Read-only · Viewer</Tag>} />
}

export function SourceTag({ source }: { source?: string }) {
  return source ? <Tag title={`read by the ${source} driver`}>{source}</Tag> : <span className="cap">—</span>
}

/** Typed editor for one catalogue setting (bool → on/off, enum → select, int → number, list → comma-separated). */
export function SettingInput({ def, value, onChange, id, disabled }: { def: CatalogDef; value: unknown; onChange: (v: unknown) => void; id?: string; disabled?: boolean }) {
  switch (def.type) {
    case 'bool':
      return <Seg sm label={def.name} value={value === true ? 'on' : 'off'} onChange={(v) => !disabled && onChange(v === 'on')} options={[{ v: 'on', label: 'On' }, { v: 'off', label: 'Off' }]} />
    case 'enum':
      return <select className="fin mono" id={id} value={value === undefined || value === null ? '' : String(value)} disabled={disabled} onChange={(e) => onChange(e.target.value)} aria-label={def.name}>
        <option value="" disabled>choose…</option>{(def.options || []).map((o) => <option key={String(o)} value={String(o)}>{String(o)}</option>)}
      </select>
    case 'int':
      return <input className="fin mono" id={id} type="number" step={1} value={value === undefined || value === null ? '' : String(value)} disabled={disabled} onChange={(e) => onChange(e.target.value === '' ? '' : Number(e.target.value))} aria-label={def.name} />
    case 'list':
      return <input className="fin mono" id={id} value={Array.isArray(value) ? value.join(', ') : String(value ?? '')} placeholder="comma-separated" disabled={disabled} onChange={(e) => onChange(e.target.value.split(',').map((s) => s.trim()).filter(Boolean))} aria-label={def.name} />
    default:
      return <input className="fin mono" id={id} value={String(value ?? '')} disabled={disabled} onChange={(e) => onChange(e.target.value)} aria-label={def.name} />
  }
}

/** True when a draft value is complete enough to save (numbers parsed, enum chosen). */
export function valueComplete(def: CatalogDef, v: unknown): boolean {
  if (def.type === 'bool') return typeof v === 'boolean'
  if (def.type === 'int') return typeof v === 'number' && Number.isInteger(v)
  if (def.type === 'enum') return v !== '' && v !== undefined && v !== null
  if (def.type === 'list') return Array.isArray(v)
  return typeof v === 'string'
}
