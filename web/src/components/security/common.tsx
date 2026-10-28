// Shared building blocks for the security UI: severity markers, compliance
// meters, certificate expiry pills and timeline, target picker, the
// confirmation dialog with blast radius, and the live job hook.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { Device, Group, Job, JobResult } from '../../types'
import { deviceName } from '../../types'
import { useEvents } from '../../ws'
import { Ic } from '../icons'
import { Btn, Dialog, JobBar, JobPill, Pill, SearchBox, Seg, Tag } from '../ui'
import type { Assessment, ItemStatus, PolicyOperator, RemediationStatus, SecurityTarget } from '../../types-security'
import { daysLeft } from '../../types-security'

// --- severity ---------------------------------------------------------------------

/** Inline policy-severity marker (design 02): octagon Critical, diamond High, square Medium, ring Low. */
export function SevMark({ sev, noLabel, title }: { sev: string; noLabel?: boolean; title?: string }) {
  const label = sev.charAt(0).toUpperCase() + sev.slice(1)
  const color = sev === 'critical' || sev === 'high' ? 'var(--crit)' : sev === 'medium' ? 'var(--warn)' : 'var(--text-2)'
  let mark: React.ReactNode
  if (sev === 'critical') mark = <Ic.crit size={12} style={{ strokeWidth: 2.25 }} />
  else if (sev === 'high') mark = <i aria-hidden="true" style={{ width: 9, height: 9, background: 'var(--crit)', transform: 'rotate(45deg)', display: 'inline-block', borderRadius: 1, flex: 'none' }} />
  else if (sev === 'medium') mark = <i aria-hidden="true" style={{ width: 9, height: 9, background: 'var(--warn)', borderRadius: 2, display: 'inline-block', flex: 'none' }} />
  else mark = <i aria-hidden="true" style={{ width: 10, height: 10, border: '2px solid var(--text-3)', borderRadius: '50%', display: 'inline-block', flex: 'none' }} />
  return <span className="sev" title={title || `${label} severity`} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color, fontWeight: 500, fontSize: 12, whiteSpace: 'nowrap' }}>{mark}{!noLabel && label}</span>
}

export function ItemStatusPill({ status, xs }: { status: ItemStatus | string; xs?: boolean }) {
  if (status === 'pass') return <Pill tone="ok" xs={xs}>Pass</Pill>
  if (status === 'fail') return <span className={`pill crit${xs ? ' xs' : ''}`}><Ic.x />Fail</span>
  return <Pill tone="unk" xs={xs}>Unknown</Pill>
}

export function RemediationPill({ status, xs }: { status: RemediationStatus | string; xs?: boolean }) {
  switch (status) {
    case 'written': return <Pill tone="ok" xs={xs}>Written</Pill>
    case 'would_write': return <Pill tone="info" xs={xs} noIcon><Ic.upload />Would write</Pill>
    case 'unchanged': return <Pill tone="ok" xs={xs} noIcon><Ic.check />Unchanged</Pill>
    case 'failed': return <Pill tone="crit" xs={xs}>Failed</Pill>
    default: return <Pill tone="off" xs={xs}>Skipped</Pill>
  }
}

export const operatorLabel: Record<PolicyOperator, string> = { eq: 'equals', ne: 'is not', gte: '≥', lte: '≤', in: 'is one of', notIn: 'is none of', set: 'is set', unset: 'is unset' }

/** Human form of a setting value; secrets arrive already redacted as <set>/<unset>. */
export function fmtVal(v: unknown): string {
  if (v === undefined || v === null || v === '') return '—'
  if (Array.isArray(v)) return v.map(fmtVal).join(', ')
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

export function isMgmtPathSkip(msg?: string): boolean { return !!msg && /management-path/i.test(msg) }

export function MgmtPathTag({ path, xs }: { path?: string; xs?: boolean }) {
  if (!path) return null
  return <Tag title={`Remediating this item could remove the ${path.toUpperCase()} management path; the engine keeps at least one path open (spec §11)`}><Ic.warn size={xs ? 11 : 12} style={{ color: 'var(--warn)' }} />may cut {path.toUpperCase()} path</Tag>
}

// --- meters ---------------------------------------------------------------------------

export function scoreTone(a: Pick<Assessment, 'score' | 'compliant' | 'passed' | 'failed'>): 'ok' | 'warn' | 'crit' {
  if (a.compliant) return 'ok'
  return a.score >= 80 ? 'warn' : 'crit'
}
const toneVar = { ok: 'var(--ok)', warn: 'var(--warn)', crit: 'var(--crit)', off: 'var(--off)' } as const

/** Compliance meter ring (design 07): percentage in the middle, optional caption below. */
export function ComplianceRing({ pct, size = 104, caption, label, tone }: { pct: number | null; size?: number; caption?: string; label: string; tone?: 'ok' | 'warn' | 'crit' | 'off' }) {
  const p = pct === null ? 0 : Math.max(0, Math.min(100, pct))
  const circ = 2 * Math.PI * 42
  const t = tone || (pct === null ? 'off' : p >= 93 ? 'ok' : p >= 75 ? 'warn' : 'crit')
  return (
    <svg viewBox="0 0 100 100" width={size} height={size} role="img" aria-label={label}>
      <circle cx="50" cy="50" r="42" fill="none" style={{ stroke: 'var(--track)' }} strokeWidth="10" />
      {pct !== null && <circle cx="50" cy="50" r="42" fill="none" style={{ stroke: toneVar[t] }} strokeWidth="10" strokeDasharray={`${(p / 100) * circ} ${circ}`} strokeLinecap="butt" transform="rotate(-90 50 50)" />}
      <text x="50" y={caption ? 49 : 56} textAnchor="middle" fontSize={size >= 100 ? 19 : 20} fontWeight="600" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--text)' }}>{pct === null ? '—' : `${p.toFixed(size >= 100 ? 1 : 0)}%`}</text>
      {caption && <text x="50" y="64" textAnchor="middle" fontSize="9" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--text-3)' }}>{caption}</text>}
    </svg>
  )
}

/** Per-device meter: "12 / 14 items" with a bar in the device's compliance tone. */
export function ScoreBar({ a, width }: { a: Pick<Assessment, 'score' | 'compliant' | 'passed' | 'failed' | 'unknown'>; width?: number | string }) {
  const total = a.passed + a.failed
  const tone = scoreTone(a)
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, width }}>
      <div className="bar" style={{ flex: 1, height: 8, borderRadius: 4 }} role="img" aria-label={`${a.passed} of ${total} items passing, score ${a.score}`}><span style={{ width: `${total ? (a.passed / total) * 100 : 0}%`, background: toneVar[tone] }} /></div>
      <span className="num cap" style={{ width: 46, textAlign: 'right' }}>{a.passed}/{total}</span>
    </div>
  )
}

/** Item pass-rate bar: green pass share, red fail share, grey unknown remainder. */
export function PassRateBar({ pass, fail, unknown = 0 }: { pass: number; fail: number; unknown?: number }) {
  const n = pass + fail + unknown
  const pct = (x: number) => (n ? (x / n) * 100 : 0)
  const rate = pass + fail ? (pass / (pass + fail)) * 100 : null
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <div className="bar" style={{ flex: 1, height: 8, borderRadius: 4 }} role="img" aria-label={`${pass} pass, ${fail} fail, ${unknown} unknown`}>
        <span style={{ width: `${pct(pass)}%`, background: 'var(--ok)' }} /><span style={{ width: `${pct(fail)}%`, background: 'var(--crit)' }} />
      </div>
      <span className="num cap" style={{ width: 46, textAlign: 'right' }}>{rate === null ? '—' : `${rate.toFixed(1)}%`}</span>
    </div>
  )
}

// --- certificates ------------------------------------------------------------------------

export type ExpiryTone = 'expired' | 'critical' | 'warning' | 'ok'
export function expiryTone(notAfter: string): ExpiryTone {
  const d = daysLeft(notAfter)
  if (d < 0) return 'expired'
  if (d <= 7) return 'critical'
  if (d <= 30) return 'warning'
  return 'ok'
}

/** Expiry pill (design 07): clock icon, critical when expired or ≤ 7 d, warning ≤ 30 d, else month and year. */
export function ExpiryPill({ notAfter, xs }: { notAfter: string; xs?: boolean }) {
  const d = daysLeft(notAfter)
  const t = expiryTone(notAfter)
  const cls = `pill ${t === 'ok' ? 'ok' : t === 'warning' ? 'warn' : 'crit'}${xs ? ' xs' : ''}`
  const when = new Date(notAfter)
  if (isNaN(when.getTime())) return <Pill tone="unk" xs={xs}>No expiry</Pill>
  if (t === 'expired') return <span className={cls} title={when.toLocaleString()}><Ic.clock />Expired {-d === 0 ? 'today' : `${-d} d ago`}</span>
  if (t !== 'ok') return <span className={cls} title={when.toLocaleString()}><Ic.clock />in {d} day{d === 1 ? '' : 's'}</span>
  return <span className={cls} title={when.toLocaleString()}><Ic.ok />{when.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })}</span>
}

/** Certificate expiry timeline for the next 90 days (design 07), built from real expiry dates. */
export function ExpiryTimeline({ dates }: { dates: string[] }) {
  const days = dates.map((d) => daysLeft(d)).filter((d) => !isNaN(d))
  const expired = days.filter((d) => d < 0).length
  const within7 = days.filter((d) => d >= 0 && d <= 7).length
  const within30 = days.filter((d) => d > 7 && d <= 30).length
  const within90 = days.filter((d) => d > 30 && d <= 90).length
  const W = 560
  const x = (d: number) => Math.max(0, Math.min(W, (d / 90) * W))
  const x7 = x(7), x30 = x(30)
  return (
    <svg viewBox={`0 0 ${W} 64`} width="100%" height="64" role="img" aria-label={`Certificate expiry timeline: ${expired} expired, ${within7} within 7 days, ${within30} within 30 days, ${within90} within 90 days`}>
      <rect x="0" y="26" width={W} height="6" rx="3" style={{ fill: 'var(--track)' }} />
      <rect x="0" y="26" width={x7} height="6" rx="3" style={{ fill: 'var(--crit)' }} />
      <rect x={x7} y="26" width={x30 - x7} height="6" style={{ fill: 'var(--warn)' }} />
      <g fontSize="10" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--text-3)' }}><text x="0" y="52">Today</text><text x={x7 - 6} y="52">7 d</text><text x={x30 - 8} y="52">30 d</text><text x={W - 26} y="52">90 d</text></g>
      {days.map((d, i) => {
        const cx = d < 0 ? 4 : x(Math.min(d, 90))
        const fill = d < 0 || d <= 7 ? 'var(--crit)' : d <= 30 ? 'var(--warn)' : 'var(--text-3)'
        const r = d < 0 ? 5 : d <= 30 ? 4 : 3.5
        return <circle key={i} cx={cx} cy="29" r={r} style={{ fill, opacity: d > 90 ? 0.35 : 1 }}><title>{d < 0 ? `expired ${-d} d ago` : `in ${d} d`}</title></circle>
      })}
      {expired > 0 && <text x="12" y="14" fontSize="11" fontWeight="600" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--crit)' }}>{expired} expired</text>}
      {within7 + within30 > 0 && <text x={Math.max(90, x7 + 10)} y="14" fontSize="11" fontWeight="600" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--warn)' }}>{within7 + within30} ≤ 30 d</text>}
      {days.length === 0 && <text x={W / 2} y="14" textAnchor="middle" fontSize="11" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--text-3)' }}>no certificates observed yet</text>}
    </svg>
  )
}

// --- secrets -----------------------------------------------------------------------------

/** Secret row (design 12): a stored secret is only ever described, never shown. */
export function SecretRow({ label, detail, replaceTo, right }: { label: string; detail?: string; replaceTo?: string; right?: React.ReactNode }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 160px) minmax(0, 1fr) auto', gap: '8px 12px', alignItems: 'center' }}>
      <span className="cap">{label}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, height: 32, padding: '0 10px', border: '1px solid var(--border)', borderRadius: 6, background: 'var(--surface-2)', minWidth: 0 }}><Ic.lock style={{ color: 'var(--text-3)' }} /><span className="cap" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>Set{detail ? ` · ${detail}` : ''} · never displayed</span></div>
      {right ?? (replaceTo ? <Link className="btn sm" to={replaceTo}>Replace…</Link> : <span />)}
    </div>
  )
}

// --- jobs -----------------------------------------------------------------------------------

export const running = (j?: Job | null) => !!j && (j.status === 'queued' || j.status === 'running')

/** Live view of one job: reloads on job.updated and polls every 2 s while it runs. */
export function useJob(id: string | null | undefined) {
  const [job, setJob] = useState<Job | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [error, setError] = useState('')
  const load = useCallback(() => {
    if (!id) return
    api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${id}`).then((r) => { setJob(r.job); setResults(r.results); setError('') }).catch((e) => setError((e as Error).message))
  }, [id])
  useEffect(() => { setJob(null); setResults([]); load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated' && (e.data as Job)?.id === id) load() }, [id, load])
  const active = running(job)
  useEffect(() => { if (!active) return; const t = setInterval(load, 2000); return () => clearInterval(t) }, [active, load])
  return { job, results, error, reload: load, done: !!job && !active }
}

export function JobProgress({ job, children }: { job: Job; children?: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><JobPill status={job.status} />{children}</span>
        <span className="num" style={{ fontWeight: 600 }}>{job.done} <span className="cap" style={{ fontWeight: 400 }}>/ {job.total}</span>{job.failed > 0 && <span style={{ color: 'var(--crit)' }}> · {job.failed} failed</span>}</span>
      </div>
      <JobBar done={job.done} failed={job.failed} total={job.total} running={job.status === 'running'} />
      {job.error && <div className="error small">{job.error}</div>}
    </div>
  )
}

// --- targets -------------------------------------------------------------------------------

export function targetLabel(t: SecurityTarget, groups: Group[], devices: Device[]): string {
  if (t.groupId) return `group ${groups.find((g) => g.id === t.groupId)?.name || t.groupId.slice(0, 8)}`
  const n = t.deviceIds?.length || 0
  if (n === 1) { const d = devices.find((x) => x.id === t.deviceIds![0]); return d ? deviceName(d) : '1 device' }
  return `${n} device${n === 1 ? '' : 's'}`
}

/** Resolve a target to device objects on the client (rule groups are matched by the server; we use memberIds for static, and fall back to the group's listed devices). */
export function targetDevices(t: SecurityTarget, groups: Group[], devices: Device[], groupMembers?: Record<string, string[]>): Device[] {
  if (t.groupId) {
    const ids = groupMembers?.[t.groupId] || groups.find((g) => g.id === t.groupId)?.memberIds || []
    return devices.filter((d) => ids.includes(d.id))
  }
  return devices.filter((d) => t.deviceIds?.includes(d.id))
}

/** Pick a group or a set of devices as the target of an assess / remediate / rotate job. */
export function TargetPicker({ value, onChange, devices, groups, hint }: { value: SecurityTarget; onChange: (t: SecurityTarget) => void; devices: Device[]; groups: Group[]; hint?: React.ReactNode }) {
  const mode: 'group' | 'devices' = value.groupId !== undefined && !value.deviceIds ? 'group' : 'devices'
  const [q, setQ] = useState('')
  const sel = value.deviceIds || []
  const shown = useMemo(() => devices.filter((d) => d.state !== 'decommissioned').filter((d) => !q || `${deviceName(d)} ${d.address} ${d.model || ''} ${d.location}`.toLowerCase().includes(q.toLowerCase())), [devices, q])
  const toggle = (id: string) => onChange({ deviceIds: sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id] })
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div className="row">
        <Seg sm label="Target kind" value={mode} onChange={(m) => onChange(m === 'group' ? { groupId: groups[0]?.id || '' } : { deviceIds: [] })} options={[{ v: 'group', label: <><Ic.layers size={13} />Group</> }, { v: 'devices', label: <><Ic.printer size={13} />Devices</> }]} />
        {mode === 'group' && (
          <label className="chip set"><Ic.layers size={13} />Group
            <select aria-label="Target group" value={value.groupId || ''} onChange={(e) => onChange({ groupId: e.target.value })}>
              <option value="">choose…</option>{groups.map((g) => <option key={g.id} value={g.id}>{g.name}{g.kind === 'static' ? ` (${g.memberIds.length})` : ''}</option>)}
            </select>
          </label>
        )}
        {mode === 'devices' && <span className="cap num">{sel.length} selected</span>}
        {mode === 'devices' && shown.length > 0 && <Btn sm variant="ghost" onClick={() => onChange({ deviceIds: sel.length === shown.length ? [] : shown.map((d) => d.id) })}>{sel.length === shown.length ? 'Clear' : `Select all ${shown.length}`}</Btn>}
      </div>
      {hint && <div className="cap">{hint}</div>}
      {mode === 'devices' && (
        <div style={{ border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
          <div style={{ padding: 8, borderBottom: '1px solid var(--border)' }}><SearchBox sm filter value={q} onChange={setQ} placeholder="Filter devices…" /></div>
          <div style={{ maxHeight: 220, overflow: 'auto' }}>
            {shown.length === 0 && <div className="cb cap">No devices match.</div>}
            {shown.map((d) => (
              <label key={d.id} className="lrow" style={{ padding: '6px 12px', cursor: 'pointer', gap: 10 }}>
                <input className="cbx" type="checkbox" checked={sel.includes(d.id)} onChange={() => toggle(d.id)} aria-label={`Target ${deviceName(d)}`} />
                <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}><b style={{ fontWeight: 500 }}>{deviceName(d)}</b> <span className="cap mono">{d.address}</span></span>
                <span className="cap">{d.model || ''}</span>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// --- confirmation --------------------------------------------------------------------------

/** Confirmation dialog (design 10): states the blast radius; above 10 devices a typed phrase is required. */
export function ConfirmWriteDialog({ title, sub, rows, deviceCount, confirmWord, confirmLabel, onBack, onConfirm, busy, backLabel = 'Back to preview', icon }: {
  title: string; sub?: React.ReactNode; rows: [string, React.ReactNode][]; deviceCount: number; confirmWord: string; confirmLabel: string
  onBack: () => void; onConfirm: () => void; busy?: boolean; backLabel?: string; icon?: React.ReactNode
}) {
  const [typed, setTyped] = useState('')
  const needsTyping = deviceCount > 10
  const ok = !needsTyping || typed.trim().toUpperCase() === confirmWord.toUpperCase()
  return (
    <Dialog title={title} sub={sub} onClose={onBack}
      icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--warn-bg)', color: 'var(--warn)', display: 'grid', placeItems: 'center', flex: 'none' }}>{icon || <Ic.upload />}</span>}
      footer={<><Btn onClick={onBack} disabled={busy}>{backLabel}</Btn><Btn variant="dngf" onClick={onConfirm} disabled={!ok || busy || deviceCount === 0}>{busy ? 'Starting…' : confirmLabel}</Btn></>}>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px 16px', padding: '4px 0 10px' }}>
        {rows.map(([k, v]) => <span key={k} style={{ display: 'contents' }}><span className="cap">{k}</span><b className="num" style={{ fontWeight: 500, minWidth: 0, overflowWrap: 'anywhere' }}>{v}</b></span>)}
      </div>
      {needsTyping ? (
        <div>
          <label className="cap" htmlFor="sec-cfm" style={{ display: 'block', marginBottom: 4 }}>Type <b className="mono" style={{ color: 'var(--text)' }}>{confirmWord.toUpperCase()}</b> to confirm (required above 10 devices)</label>
          <input className="fin mono" id="sec-cfm" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck={false} autoFocus />
        </div>
      ) : <div className="cap">Fewer than 11 devices — no typed confirmation needed.</div>}
    </Dialog>
  )
}

/** Compliance history as an SVG line chart (same construction as DailyChart in DeviceDetail). */
export function HistoryChart({ points }: { points: { date: string; assessed: number; compliant: number; avgScore: number }[] }) {
  const days = points.filter((p) => p.assessed > 0)
  if (days.length < 2) return <div className="cap">The compliance trend appears after assessments on two or more days.</div>
  const w = 640, h = 140, x0 = 36
  const X = (i: number) => x0 + (i / (days.length - 1)) * (w - x0 - 4)
  const Y = (v: number) => 120 - (v / 100) * 110
  const score = days.map((d, i) => `${X(i).toFixed(1)},${Y(d.avgScore).toFixed(1)}`).join(' ')
  const rate = days.map((d, i) => `${X(i).toFixed(1)},${Y(d.assessed ? (d.compliant / d.assessed) * 100 : 0).toFixed(1)}`).join(' ')
  const last = days[days.length - 1]
  return (
    <>
      <div className="cap" style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 14, height: 3, background: 'var(--cat1)', borderRadius: 2 }} />Average score</span>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 14, height: 0, borderTop: '2px dashed var(--ok)' }} />Devices compliant</span>
        <span style={{ marginLeft: 'auto' }} className="num">latest {last.avgScore.toFixed(1)} · {last.compliant}/{last.assessed} compliant</span>
      </div>
      <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label={`Compliance over ${days.length} days: average score from ${days[0].avgScore} to ${last.avgScore}`}>
        <g style={{ stroke: 'var(--border)' }}><line x1={x0} y1="10" x2={w} y2="10" /><line x1={x0} y1="65" x2={w} y2="65" /><line x1={x0} y1="120" x2={w} y2="120" /></g>
        <g style={{ fill: 'var(--text-3)' }} fontSize="10" fontFamily="IBM Plex Sans, sans-serif"><text x="0" y="14">100</text><text x="4" y="69">50</text><text x="10" y="124">0</text><text x={x0} y="138">{days[0].date.slice(5)}</text><text x={w - 40} y="138">{last.date.slice(5)}</text></g>
        <polyline fill="none" style={{ stroke: 'var(--ok)' }} strokeWidth="2" strokeDasharray="5 3" points={rate} />
        <polyline fill="none" style={{ stroke: 'var(--cat1)' }} strokeWidth="2" points={score} />
        <circle cx={X(days.length - 1)} cy={Y(last.avgScore)} r="2.5" style={{ fill: 'var(--cat1)' }} />
      </svg>
    </>
  )
}

/** Failing-item chips with severity markers, worst first. */
export function FailingChips({ items, max = 4 }: { items: { itemId: string; name: string; severity: string }[]; max?: number }) {
  const order: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 }
  const sorted = [...items].sort((a, b) => (order[a.severity] ?? 9) - (order[b.severity] ?? 9))
  if (sorted.length === 0) return <span className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 4, color: 'var(--ok)' }}><Ic.check size={14} />none</span>
  return (
    <span style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 4 }}>
      {sorted.slice(0, max).map((it) => <Tag key={it.itemId} title={`${it.name} · ${it.severity}`}><SevMark sev={it.severity} noLabel />{it.name}</Tag>)}
      {sorted.length > max && <Tag>+{sorted.length - max} more</Tag>}
    </span>
  )
}
