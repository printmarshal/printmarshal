import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Consumable, ConsumableType, ForecastSummary, StockStatus } from '../types'

export const typeLabels: Record<ConsumableType, string> = {
  toner: 'Toner', ink: 'Ink', drum: 'Drum', fuser: 'Fuser', maintenance_kit: 'Maintenance kit', waste: 'Waste', staples: 'Staples', other: 'Other',
}
export function typeLabel(t: string): string { return typeLabels[t as ConsumableType] || t }

/** Colour swatch hex for a consumable colour (C/M/Y/K and named colours). */
export function swatchFor(color?: string): string | undefined {
  const n = (color || '').toLowerCase()
  if (!n) return undefined
  if (n.includes('light cyan')) return '#7fdcf5'
  if (n.includes('light magenta')) return '#f58ac6'
  if (n.includes('cyan') || n === 'c') return '#00b7eb'
  if (n.includes('magenta') || n === 'm') return '#ec008c'
  if (n.includes('yellow') || n === 'y') return '#ffd400'
  if (n.includes('black') || n === 'k') return '#222'
  if (n.includes('gray') || n.includes('grey')) return '#8a8f94'
  if (n.includes('red')) return '#d9342b'
  if (n.includes('green')) return '#2c9f45'
  if (n.includes('blue')) return '#2962c9'
  if (n.includes('orange')) return '#f28c28'
  if (n.includes('white')) return '#f5f5f5'
  return '#b0b6bc'
}

export function ColorSwatch({ color, label }: { color?: string; label?: boolean }) {
  const hex = swatchFor(color)
  if (!hex) return label ? <span className="muted small">—</span> : null
  return <span className="swatch" title={color}><span className="dot" style={{ background: hex }} />{label && <span className="small">{color}</span>}</span>
}

const statusMeta: Record<StockStatus, { label: string; cls: string; icon: string }> = {
  ok: { label: 'In stock', cls: 'ok', icon: '●' }, low: { label: 'Low', cls: 'warning', icon: '▲' }, critical: { label: 'Critical', cls: 'critical', icon: '▲' }, out: { label: 'Out', cls: 'critical', icon: '✕' },
}
export function statusOf(c: Pick<Consumable, 'onHand' | 'warningAt' | 'criticalAt'>): StockStatus {
  if (c.onHand <= 0) return 'out'
  if (c.onHand <= c.criticalAt) return 'critical'
  if (c.onHand <= c.warningAt) return 'low'
  return 'ok'
}
export function StockPill({ status }: { status: StockStatus }) {
  const m = statusMeta[status] || statusMeta.ok
  return <span className={`badge ${m.cls}`}><span aria-hidden style={{ marginRight: 4, fontSize: 9 }}>{m.icon}</span>{m.label}</span>
}

export function ModelChips({ models, max = 3 }: { models: string[]; max?: number }) {
  if (!models || models.length === 0) return <span className="muted small">any model</span>
  const shown = models.slice(0, max)
  return (
    <span className="chips">
      {shown.map((m) => <span key={m} className="chip" title={m}>{m}</span>)}
      {models.length > max && <span className="chip muted" title={models.slice(max).join(', ')}>+{models.length - max}</span>}
    </span>
  )
}

export function fmtDays(d: number | null | undefined): string {
  if (d === null || d === undefined) return '—'
  if (d < 1) return '< 1 day'
  if (d < 14) return `~${Math.round(d)} days`
  if (d < 60) return `~${Math.round(d / 7)} weeks`
  return `~${Math.round(d / 30)} months`
}

/** Forecast / "days remaining" indicator with an explicit unknown state. */
export function ForecastCell({ f, compact }: { f: ForecastSummary; compact?: boolean }) {
  if (!f.known) return <span className="muted small" title={f.reason || 'not enough history yet'}>— <span className="forecast-unknown">unknown</span></span>
  if (f.reorderSoon) return <span className="forecast reorder" title={`stock lasts ${fmtDays(f.stockDays)}`}>Reorder soon{!compact && f.stockDays !== null && <span className="small"> · {fmtDays(f.stockDays)}</span>}</span>
  if (f.stockDays !== null) return <span className="forecast" title="projected days until on-hand stock is used up">{fmtDays(f.stockDays)}</span>
  if (f.nextNeedDays !== null) return <span className="forecast" title="next device replacement">next need {fmtDays(f.nextNeedDays)}</span>
  return <span className="muted small" title={f.reason}>{f.reason || '—'}</span>
}

/** Inline-editable on-hand quantity. Dirty state is highlighted until saved. */
export function QtyCell({ value, original, disabled, onChange }: { value: number; original: number; disabled?: boolean; onChange: (v: number) => void }) {
  const dirty = value !== original
  if (disabled) return <span>{original}</span>
  return (
    <span className={`qty ${dirty ? 'dirty' : ''}`} title={dirty ? `unsaved (was ${original})` : 'operator-owned; never overwritten by scans'}>
      <button type="button" className="step" aria-label="decrease" onClick={() => onChange(Math.max(0, value - 1))}>−</button>
      <input type="number" min={0} value={value} onChange={(e) => onChange(Math.max(0, Number(e.target.value) || 0))} />
      <button type="button" className="step" aria-label="increase" onClick={() => onChange(value + 1)}>+</button>
      {dirty && <span className="dirty-dot" aria-label="unsaved change" />}
    </span>
  )
}

export function BatchBar({ count, saving, onSave, onDiscard }: { count: number; saving: boolean; onSave: (reason: string) => void; onDiscard: () => void }) {
  const [reason, setReason] = useState('')
  useEffect(() => { if (count === 0) setReason('') }, [count])
  if (count === 0) return null
  return (
    <div className="batchbar" role="status">
      <strong>{count} unsaved {count === 1 ? 'quantity' : 'quantities'}</strong>
      <input placeholder="Reason (optional, e.g. stocktake)" value={reason} onChange={(e) => setReason(e.target.value)} style={{ minWidth: 240 }} />
      <button className="primary" disabled={saving} onClick={() => onSave(reason)}>{saving ? 'Saving…' : 'Save all'}</button>
      <button disabled={saving} onClick={onDiscard}>Discard</button>
    </div>
  )
}

/** Adjust-stock control: set / add / remove with a reason. */
export function AdjustStock({ onHand, onApply }: { onHand: number; onApply: (body: { set?: number; delta?: number; reason: string }) => Promise<void> }) {
  const [mode, setMode] = useState<'add' | 'remove' | 'set'>('add')
  const [n, setN] = useState(1)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const apply = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setMsg('')
    try {
      await onApply(mode === 'set' ? { set: n, reason } : { delta: mode === 'add' ? n : -n, reason })
      setMsg('Saved.'); setReason('')
    } catch (err) { setMsg((err as Error).message) } finally { setBusy(false) }
  }
  const preview = mode === 'set' ? n : mode === 'add' ? onHand + n : Math.max(0, onHand - n)
  return (
    <form className="row adjust" onSubmit={apply}>
      <select value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}>
        <option value="add">Add</option><option value="remove">Remove</option><option value="set">Set to</option>
      </select>
      <input type="number" min={0} value={n} onChange={(e) => setN(Math.max(0, Number(e.target.value) || 0))} style={{ width: 80 }} />
      <input placeholder="Reason" value={reason} onChange={(e) => setReason(e.target.value)} style={{ minWidth: 160 }} />
      <button className="primary" disabled={busy}>Apply</button>
      <span className="muted small">→ {preview} on hand</span>
      {msg && <span className="small">{msg}</span>}
    </form>
  )
}

export function ConsumableLink({ c }: { c: Pick<Consumable, 'id' | 'name' | 'color'> }) {
  return <Link to={`/consumables/${c.id}`} className="row" style={{ gap: 6, display: 'inline-flex' }}><ColorSwatch color={c.color} />{c.name}</Link>
}

/** Mini supply bar for an installed level. */
export function LevelBar({ percent, filled }: { percent: number | null; filled?: boolean }) {
  const p = percent === null ? null : filled ? 100 - percent : percent
  const cls = p === null ? '' : p <= 5 ? 'critical' : p <= 15 ? 'warning' : ''
  return <span className="row" style={{ gap: 6, display: 'inline-flex' }}><span className={`bar ${cls}`} style={{ width: 70, minWidth: 70 }}><div style={{ width: `${p ?? 0}%` }} /></span><span className="small" style={{ width: 36 }}>{percent === null ? '?' : `${percent}%`}</span></span>
}
