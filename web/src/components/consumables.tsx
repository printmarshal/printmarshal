import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Consumable, ConsumableType, ForecastSummary, StockStatus } from '../types'
import { Ic } from './icons'
import { Bar, Btn, Pill, Seg, Sw } from './ui'

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
  if (n.includes('cyan') || n === 'c') return 'var(--sup-c)'
  if (n.includes('magenta') || n === 'm') return 'var(--sup-m)'
  if (n.includes('yellow') || n === 'y') return 'var(--sup-y)'
  if (n.includes('black') || n === 'k') return 'var(--sup-k)'
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
  return <span className="swatch" title={color}><Sw color={hex} none={!hex} />{label && <span className="cap">{color || '—'}</span>}</span>
}

const statusMeta: Record<StockStatus, { label: string; tone: 'ok' | 'warn' | 'crit' }> = {
  ok: { label: 'In stock', tone: 'ok' }, low: { label: 'Low', tone: 'warn' }, critical: { label: 'Critical', tone: 'crit' }, out: { label: 'Out', tone: 'crit' },
}
export function statusOf(c: Pick<Consumable, 'onHand' | 'warningAt' | 'criticalAt'>): StockStatus {
  if (c.onHand <= 0) return 'out'
  if (c.onHand <= c.criticalAt) return 'critical'
  if (c.onHand <= c.warningAt) return 'low'
  return 'ok'
}
export function StockPill({ status, xs }: { status: StockStatus; xs?: boolean }) {
  const m = statusMeta[status] || statusMeta.ok
  return <Pill tone={m.tone} xs={xs}>{m.label}</Pill>
}

export function ModelChips({ models, max = 3 }: { models: string[]; max?: number }) {
  if (!models || models.length === 0) return <span className="cap">any model</span>
  const shown = models.slice(0, max)
  return (
    <span className="chips">
      {shown.map((m) => <span key={m} className="tag mono" style={{ fontSize: 11 }} title={m}>{m}</span>)}
      {models.length > max && <span className="tag" title={models.slice(max).join(', ')}>+{models.length - max}</span>}
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
  if (!f.known) return <span className="cap" title={f.reason || 'not enough history yet'}>— <span className="forecast-unknown">not enough history</span></span>
  if (f.reorderSoon) return <span className="forecast reorder num" title={`stock lasts ${fmtDays(f.stockDays)}`}>{f.stockDays !== null ? `${fmtDays(f.stockDays)} · ` : ''}reorder soon{compact ? '' : ''}</span>
  if (f.stockDays !== null) return <span className="forecast num" style={{ color: 'var(--text-2)' }} title="projected days until on-hand stock is used up">{fmtDays(f.stockDays)}</span>
  if (f.nextNeedDays !== null) return <span className="forecast num" title="next device replacement">next need {fmtDays(f.nextNeedDays)}</span>
  return <span className="cap" title={f.reason}>{f.reason || '—'}</span>
}

/** Inline-editable on-hand quantity. Dirty state is highlighted until saved. */
export function QtyCell({ value, original, disabled, onChange }: { value: number; original: number; disabled?: boolean; onChange: (v: number) => void }) {
  const dirty = value !== original
  if (disabled) return <span className="num" style={{ fontWeight: 600 }}>{original}</span>
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <span className={`qty${dirty ? ' ed' : ''}`} title={dirty ? `unsaved (was ${original})` : 'operator-owned; never overwritten by scans'}>
        <button type="button" aria-label="decrease" onClick={() => onChange(Math.max(0, value - 1))}>−</button>
        <input type="number" min={0} value={value} aria-label="On hand quantity" onChange={(e) => onChange(Math.max(0, Number(e.target.value) || 0))} />
        <button type="button" aria-label="increase" onClick={() => onChange(value + 1)}>+</button>
      </span>
      {dirty && <span className="cap num">was {original}</span>}
    </span>
  )
}

export function BatchBar({ count, saving, onSave, onDiscard }: { count: number; saving: boolean; onSave: (reason: string) => void; onDiscard: () => void }) {
  const [reason, setReason] = useState('')
  useEffect(() => { if (count === 0) setReason('') }, [count])
  if (count === 0) return null
  return (
    <div className="savebar" role="region" aria-label="Unsaved changes">
      <Ic.pencil style={{ color: 'var(--accent)' }} />
      <b className="num" style={{ color: 'var(--accent)' }}>{count} unsaved quantity change{count === 1 ? '' : 's'}</b>
      <span className="cap" style={{ flex: '1 1 200px' }}>Edited cells are outlined. Saving writes to PrintMarshal's stock records only — nothing is sent to devices.</span>
      <input className="fin" placeholder="Reason (optional, e.g. stocktake)" value={reason} onChange={(e) => setReason(e.target.value)} style={{ width: 240 }} />
      <Btn sm variant="ghost" disabled={saving} onClick={onDiscard}>Discard</Btn>
      <Btn sm variant="pri" disabled={saving} onClick={() => onSave(reason)}><Ic.check />{saving ? 'Saving…' : 'Save all'}</Btn>
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
      const why = reason || (mode === 'add' ? 'Received delivery' : mode === 'set' ? 'Count correction' : 'Replaced in a device')
      await onApply(mode === 'set' ? { set: n, reason: why } : { delta: mode === 'add' ? n : -n, reason: why })
      setMsg('Saved.'); setReason('')
    } catch (err) { setMsg((err as Error).message) } finally { setBusy(false) }
  }
  const preview = mode === 'set' ? n : mode === 'add' ? onHand + n : Math.max(0, onHand - n)
  const defaultReason = mode === 'add' ? 'Received delivery' : mode === 'set' ? 'Count correction' : 'Replaced in a device'
  return (
    <form className="row" style={{ alignItems: 'flex-end', gap: 14 }} onSubmit={apply}>
      <div><div className="cap" style={{ marginBottom: 4 }}>Action</div><Seg label="Adjust action" value={mode} onChange={setMode} options={[{ v: 'set', label: 'Set to' }, { v: 'add', label: 'Add' }, { v: 'remove', label: 'Remove' }]} /></div>
      <div><div className="cap" style={{ marginBottom: 4 }}>Quantity</div><span className="qty" style={{ height: 32 }}><button type="button" aria-label="Decrease" onClick={() => setN(Math.max(0, n - 1))}>−</button><input type="number" min={0} value={n} aria-label="Adjustment quantity" onChange={(e) => setN(Math.max(0, Number(e.target.value) || 0))} /><button type="button" aria-label="Increase" onClick={() => setN(n + 1)}>+</button></span></div>
      <label style={{ flex: '1 1 200px' }}><span className="cap" style={{ display: 'block', marginBottom: 4 }}>Reason</span><input className="fin" list="pm-reasons" placeholder={defaultReason} value={reason} onChange={(e) => setReason(e.target.value)} /><datalist id="pm-reasons">{['Received delivery', 'Replaced in a device', 'Count correction', 'Damaged / returned'].map((r) => <option key={r} value={r} />)}</datalist></label>
      <div className="cap num" style={{ paddingBottom: 8 }}>{onHand} → <b style={{ color: 'var(--text)', fontSize: 14 }}>{preview}</b></div>
      <Btn type="submit" variant="pri" disabled={busy}>Apply</Btn>
      {msg && <span className="cap">{msg}</span>}
    </form>
  )
}

export function ConsumableLink({ c }: { c: Pick<Consumable, 'id' | 'name' | 'color'> }) {
  return <Link to={`/consumables/${c.id}`} className="row" style={{ gap: 6, display: 'inline-flex' }}><ColorSwatch color={c.color} />{c.name}</Link>
}

/** Mini supply bar for an installed level. */
export function LevelBar({ percent, filled }: { percent: number | null; filled?: boolean }) {
  const p = percent === null ? null : filled ? 100 - percent : percent
  const color = p === null ? 'var(--track)' : p === 0 ? 'var(--crit)' : p <= 15 ? 'var(--warn)' : 'var(--ok)'
  return <span style={{ gap: 6, display: 'inline-flex', alignItems: 'center' }}><span style={{ width: 70 }}><Bar pct={p} color={color} /></span><span className="num" style={{ width: 40, textAlign: 'right', color: p !== null && p <= 15 ? (p === 0 ? 'var(--crit)' : 'var(--warn)') : undefined, fontWeight: p !== null && p <= 15 ? 600 : 400 }}>{percent === null ? '?' : p === 0 ? 'empty' : `${percent}%`}</span></span>
}
