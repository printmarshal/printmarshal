import { useMemo } from 'react'
import type { Device, Sample, Severity, Supply } from '../types'

export function SeverityBadge({ d }: { d: Device }) {
  if (!d.status) return <span className="badge outline">not polled</span>
  if (!d.status.online) return <span className="badge offline">offline</span>
  return <span className={`badge ${d.status.severity}`}>{d.status.severity === 'ok' ? d.status.state : `${d.status.severity} · ${d.status.state}`}</span>
}

export function Badge({ sev, children }: { sev: Severity | string; children: React.ReactNode }) {
  return <span className={`badge ${sev}`}>{children}</span>
}

export function StateBadge({ state }: { state: string }) {
  return <span className="badge outline">{state.replace('_', ' ')}</span>
}

function levelClass(p: number | null): string {
  if (p === null) return ''
  if (p <= 5) return 'critical'
  if (p <= 15) return 'warning'
  return ''
}

export function SupplyRow({ s }: { s: Supply }) {
  const p = s.class === 'filled' && s.percent !== null ? 100 - s.percent : s.percent
  const cls = levelClass(p)
  const color = colorFor(s.colorant || s.description)
  return (
    <div className="supply">
      <div>{color && <span className="dot" style={{ background: color }} />}{s.description || `${s.colorant || ''} ${s.type}`}<span className="muted small"> · {s.type}{s.class === 'filled' ? ' (fills)' : ''}</span></div>
      <div className={`bar ${cls}`}><div style={{ width: `${p ?? 0}%` }} /></div>
      <div className="small">{s.percent === null ? (s.level === -3 ? 'some' : '?') : `${s.percent}%`}</div>
    </div>
  )
}

export function colorFor(name?: string): string | undefined {
  const n = (name || '').toLowerCase()
  if (n.includes('cyan')) return '#00b7eb'
  if (n.includes('magenta')) return '#ec008c'
  if (n.includes('yellow')) return '#ffd400'
  if (n.includes('black')) return '#222'
  return undefined
}

export function Spark({ samples, min, max }: { samples: Sample[]; min?: number; max?: number }) {
  const path = useMemo(() => {
    if (samples.length < 2) return ''
    const vals = samples.map((s) => s.value)
    const lo = min ?? Math.min(...vals)
    const hi = max ?? Math.max(...vals)
    const span = hi - lo || 1
    const t0 = new Date(samples[0].at).getTime()
    const t1 = new Date(samples[samples.length - 1].at).getTime()
    const tspan = t1 - t0 || 1
    return samples.map((s, i) => {
      const x = ((new Date(s.at).getTime() - t0) / tspan) * 300
      const y = 56 - ((s.value - lo) / span) * 52
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    }).join(' ')
  }, [samples, min, max])
  if (!path) return <div className="muted small">not enough history yet</div>
  return (
    <svg className="spark" viewBox="0 0 300 60" preserveAspectRatio="none">
      <path d={path} fill="none" stroke="var(--accent)" strokeWidth="2" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

export function Progress({ done, total }: { done: number; total: number }) {
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0
  return <div className="progress" title={`${done}/${total}`}><div style={{ width: `${pct}%` }} /></div>
}

export function JobStatus({ status }: { status: string }) {
  const sev = status === 'succeeded' ? 'ok' : status === 'failed' ? 'critical' : status === 'partial' ? 'warning' : 'outline'
  return <span className={`badge ${sev}`}>{status}</span>
}
