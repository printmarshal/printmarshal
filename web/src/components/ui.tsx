import { useMemo } from 'react'
import type { Device, Sample, Severity, Supply, Tray } from '../types'
import { Ic } from './icons'

type Tone = 'ok' | 'warn' | 'crit' | 'off' | 'unk' | 'info' | 'new'

const toneIcon: Record<Tone, (p: { size?: number }) => JSX.Element> = {
  ok: Ic.ok, warn: Ic.warn, crit: Ic.crit, off: Ic.off, unk: Ic.unknown, info: Ic.info, new: Ic.plus,
}

/** Status pill: colour + distinct shape + label, readable in grayscale. */
export function Pill({ tone, children, xs, noIcon, title }: { tone: Tone; children: React.ReactNode; xs?: boolean; noIcon?: boolean; title?: string }) {
  const I = toneIcon[tone]
  return <span className={`pill ${tone}${xs ? ' xs' : ''}`} title={title}>{!noIcon && <I />}{children}</span>
}

export function sevTone(sev?: Severity | string): Tone {
  switch (sev) { case 'critical': return 'crit'; case 'warning': return 'warn'; case 'ok': return 'ok'; default: return 'unk' }
}

/** Device reachability & health as a pill. */
export function DevicePill({ d, xs }: { d: Device; xs?: boolean }) {
  if (d.state === 'decommissioned') return <Pill tone="unk" xs={xs}>Not contacted</Pill>
  if (!d.status) return <Pill tone="unk" xs={xs}>Not polled</Pill>
  if (!d.status.online) return <Pill tone="off" xs={xs}>Offline</Pill>
  const sev = d.severity ?? d.status.severity
  if (sev === 'critical') return <Pill tone="crit" xs={xs}>Critical</Pill>
  if (sev === 'warning') return <Pill tone="warn" xs={xs}>Warning</Pill>
  return <Pill tone="ok" xs={xs}>{d.status.state === 'printing' ? 'Printing' : 'Online'}</Pill>
}

export function SevPill({ sev, xs, cleared }: { sev: Severity | string; xs?: boolean; cleared?: boolean }) {
  if (cleared) return <Pill tone="ok" xs={xs}>Cleared</Pill>
  const t = sevTone(sev)
  return <Pill tone={t} xs={xs}>{t === 'crit' ? 'Critical' : t === 'warn' ? 'Warning' : t === 'ok' ? 'OK' : 'Unknown'}</Pill>
}

export function Badge({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return <span className={`badge ${tone}`}>{children}</span>
}

export function Tag({ children, up, mono, title }: { children: React.ReactNode; up?: boolean; mono?: boolean; title?: string }) {
  return <span className={`tag${up ? ' up' : ''}${mono ? ' mono' : ''}`} title={title} style={mono ? { fontSize: 11 } : undefined}>{children}</span>
}

type BtnProps = React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'pri' | 'wr' | 'dng' | 'dngf' | 'ghost'; sm?: boolean; ic?: boolean }
export function Btn({ variant, sm, ic, className, children, type = 'button', ...rest }: BtnProps) {
  return <button type={type} className={`btn${variant ? ' ' + variant : ''}${sm ? ' sm' : ''}${ic ? ' ic' : ''} ${className || ''}`} {...rest}>{children}</button>
}

export function Seg<T extends string>({ value, onChange, options, label, sm }: { value: T; onChange: (v: T) => void; options: { v: T; label: React.ReactNode }[]; label: string; sm?: boolean }) {
  return (
    <div className={`seg${sm ? ' sm' : ''}`} role="group" aria-label={label}>
      {options.map((o) => <button key={o.v} type="button" aria-pressed={value === o.v} onClick={() => onChange(o.v)}>{o.label}</button>)}
    </div>
  )
}

/** Filter chip wrapping a select: "Label Value ▾", accent when set. */
export function ChipSelect({ label, value, onChange, options, any = 'Any' }: { label: string; value: string; onChange: (v: string) => void; options: { v: string; label: string }[]; any?: string }) {
  const set = value !== ''
  return (
    <label className={`chip${set ? ' set' : ''}`}>
      {label}
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        <option value="">{any}</option>
        {options.map((o) => <option key={o.v} value={o.v}>{o.label}</option>)}
      </select>
      {set ? <button type="button" className="link" aria-label={`Clear ${label}`} onClick={(e) => { e.preventDefault(); onChange('') }} style={{ display: 'inline-flex' }}><Ic.x size={12} /></button> : <Ic.chevronDown size={12} />}
    </label>
  )
}

export function SearchBox({ value, onChange, placeholder, sm, filter, autoFocus }: { value: string; onChange: (v: string) => void; placeholder: string; sm?: boolean; filter?: boolean; autoFocus?: boolean }) {
  return (
    <label className={`search${sm ? ' sm' : ''}`} style={sm ? { flex: '1 1 220px', maxWidth: 320 } : undefined}>
      {filter ? <Ic.filter /> : <Ic.search />}
      <input type="search" aria-label={placeholder} placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} autoFocus={autoFocus} />
    </label>
  )
}

export function Card({ title, id, sub, right, children, className, style }: { title?: React.ReactNode; id?: string; sub?: React.ReactNode; right?: React.ReactNode; children?: React.ReactNode; className?: string; style?: React.CSSProperties }) {
  return (
    <section className={`card ${className || ''}`} aria-labelledby={id} style={style}>
      {(title || right) && (
        <div className="ch">
          <div>{title && <h2 className="h2" id={id}>{title}</h2>}{sub && <div className="cap">{sub}</div>}</div>
          {right}
        </div>
      )}
      {children}
    </section>
  )
}

export function Kpi({ label, value, detail, to, color, onClick }: { label: string; value: React.ReactNode; detail?: React.ReactNode; to?: string; color?: string; onClick?: () => void }) {
  const inner = <><span className="cap">{label}</span><span className="kv" style={color ? { color } : undefined}>{value}</span>{detail && <span className="kd">{detail}</span>}</>
  if (to) return <a className="kpi" href={to} onClick={(e) => { if (onClick) { e.preventDefault(); onClick() } }}>{inner}</a>
  return <div className="kpi">{inner}</div>
}

/** Single supply bar with the low-threshold tick. */
export function Bar({ pct, color, tick = 15, empty, lg, height }: { pct: number | null; color?: string; tick?: number | null; empty?: boolean; lg?: boolean; height?: number }) {
  return (
    <div className={`bar${lg ? ' lg' : ''}${empty ? ' empty' : ''}`} style={height ? { height } : undefined}>
      <span style={{ width: `${Math.max(pct ?? 0, pct === null ? 0 : 1)}%`, background: color || 'var(--ok)' }} />
      {lg && tick !== null && <i className="tick" style={{ left: `${tick}%` }} />}
    </div>
  )
}

/** Segmented bar (health, job progress). */
export function MultiBar({ parts, label, height = 6, gap }: { parts: { pct: number; color: string }[]; label: string; height?: number; gap?: boolean }) {
  return (
    <div className={`bar${gap ? ' seg' : ''}`} role="img" aria-label={label} style={{ height, borderRadius: height / 2 }}>
      {parts.filter((p) => p.pct > 0).map((p, i) => <span key={i} style={{ width: `${p.pct}%`, background: p.color }} />)}
    </div>
  )
}

export function supplyColor(s: Pick<Supply, 'colorant' | 'description' | 'class' | 'type'>): string {
  const n = `${s.colorant || ''} ${s.description || ''}`.toLowerCase()
  if (n.includes('cyan')) return 'var(--sup-c)'
  if (n.includes('magenta')) return 'var(--sup-m)'
  if (n.includes('yellow')) return 'var(--sup-y)'
  if (n.includes('black')) return 'var(--sup-k)'
  if (s.class === 'filled') return 'var(--warn)'
  return 'var(--text-3)'
}

/** Effective "remaining" percent: receptacles that fill up are inverted. */
export function remaining(s: Pick<Supply, 'percent' | 'class'>): number | null {
  if (s.percent === null) return null
  return s.class === 'filled' ? 100 - s.percent : s.percent
}

/** Grouped mini supply bars for table cells. */
export function Msb({ supplies }: { supplies: Supply[] }) {
  const list = supplies.filter((s) => s.class !== 'other' && s.percent !== null).slice(0, 6)
  if (list.length === 0) return <span className="cap">—</span>
  const min = Math.min(...list.map((s) => remaining(s) ?? 100))
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <span className="msb" role="img" aria-label={list.map((s) => `${s.description} ${s.percent}%`).join(', ')}>
        {list.map((s) => { const r = remaining(s) ?? 0; return <i key={s.index} className={r <= 10 ? 'low' : ''} style={{ ['--v' as string]: `${r}%`, ['--c' as string]: supplyColor(s) }} /> })}
      </span>
      {min <= 15 && <span className="cap num" style={{ color: min === 0 ? 'var(--crit)' : 'var(--warn)', fontWeight: 600 }}>{min === 0 ? 'empty' : 'low'}</span>}
    </span>
  )
}

export function Sw({ color, none, lg }: { color?: string; none?: boolean; lg?: boolean }) {
  return <span className={`sw${none || !color ? ' none' : ''}${lg ? ' lg' : ''}`} style={color && !none ? { background: color } : undefined} />
}

export function trayTone(t: Tray): Tone {
  switch (t.status) { case 'ok': return 'ok'; case 'low': case 'nearFull': return 'warn'; case 'empty': case 'full': return t.maxCapacity <= 1 ? 'off' : 'crit'; case 'unavailable': return 'off'; default: return 'unk' }
}

export function TrayChip({ t }: { t: Tray }) {
  const tone = trayTone(t)
  const level = t.percent === null ? (t.status === 'empty' ? 0 : t.level === -3 ? 2 : 4) : Math.ceil((t.percent / 100) * 4)
  const col = tone === 'ok' ? 'var(--ok)' : tone === 'warn' ? 'var(--warn)' : 'var(--crit)'
  return (
    <span className="trayc">
      <span className="stackg" aria-hidden="true" style={tone === 'crit' ? { borderColor: 'var(--crit)' } : undefined}>{Array.from({ length: Math.min(4, Math.max(0, level)) }).map((_, i) => <i key={i} style={{ background: col }} />)}</span>
      {t.name}{t.mediaSize || t.mediaName ? ` · ${t.mediaSize || t.mediaName}` : ''}
      <Pill tone={tone} xs noIcon>{t.status === 'ok' ? 'OK' : t.status.charAt(0).toUpperCase() + t.status.slice(1)}</Pill>
    </span>
  )
}

export function Av({ name, sys }: { name: string; sys?: boolean }) {
  const initials = name.split(/[\s._-]+/).filter(Boolean).slice(0, 2).map((s) => s[0]?.toUpperCase()).join('') || '?'
  return <span className={`av${sys ? ' sys' : ''}`}>{initials}</span>
}

export function Banner({ tone, children, right, role = 'status' }: { tone: 'crit' | 'warn' | 'info' | 'ok' | 'accent'; children: React.ReactNode; right?: React.ReactNode; role?: 'alert' | 'status' | 'note' }) {
  const I = tone === 'crit' ? Ic.crit : tone === 'warn' ? Ic.warn : tone === 'ok' ? Ic.ok : Ic.info
  const color = tone === 'accent' ? 'var(--accent)' : `var(--${tone})`
  return <div className={`banner ${tone}`} role={role}><I style={{ color }} /><div style={{ flex: '1 1 320px' }}>{children}</div>{right}</div>
}

export function EmptyState({ icon, title, text, action, tone = 'accent' }: { icon: React.ReactNode; title: string; text?: React.ReactNode; action?: React.ReactNode; tone?: 'accent' | 'crit' | 'ok' | 'muted' }) {
  const bg = tone === 'accent' ? 'var(--accent-weak)' : tone === 'crit' ? 'var(--crit-bg)' : tone === 'ok' ? 'var(--ok-bg)' : 'var(--surface-2)'
  const fg = tone === 'accent' ? 'var(--accent)' : tone === 'crit' ? 'var(--crit)' : tone === 'ok' ? 'var(--ok)' : 'var(--text-2)'
  return <div className="state"><span className="ring" style={{ background: bg, color: fg }}>{icon}</span><b>{title}</b>{text && <span className="cap">{text}</span>}{action}</div>
}

export function Skel({ w, h = 10, style }: { w: string | number; h?: number; style?: React.CSSProperties }) {
  return <span className="skel" style={{ width: w, height: h, ...style }} />
}

export function Spark({ samples, min, max, color = 'var(--accent)', height = 60, dot }: { samples: Sample[]; min?: number; max?: number; color?: string; height?: number; dot?: boolean }) {
  const d = useMemo(() => {
    if (samples.length < 2) return null
    const vals = samples.map((s) => s.value)
    const lo = min ?? Math.min(...vals)
    const hi = max ?? Math.max(...vals)
    const span = hi - lo || 1
    const t0 = new Date(samples[0].at).getTime()
    const t1 = new Date(samples[samples.length - 1].at).getTime()
    const tspan = t1 - t0 || 1
    const pts = samples.map((s) => [((new Date(s.at).getTime() - t0) / tspan) * 300, 56 - ((s.value - lo) / span) * 52] as [number, number])
    return { path: pts.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' '), last: pts[pts.length - 1] }
  }, [samples, min, max])
  if (!d) return <div className="cap">not enough history yet</div>
  return (
    <svg className="spark" viewBox="0 0 300 60" preserveAspectRatio="none" style={{ height }} aria-hidden="true">
      <path d={d.path} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
      {dot && <circle cx={d.last[0]} cy={d.last[1]} r="2.5" fill={color} />}
    </svg>
  )
}

/** Tiny inline sparkline (64×20) for supply rows. */
export function MiniSpark({ samples, color }: { samples: Sample[]; color: string }) {
  if (samples.length < 2) return <span className="cap" style={{ fontSize: 11 }}>—</span>
  const vals = samples.map((s) => s.value)
  const lo = Math.min(...vals, 0), hi = Math.max(...vals, 100)
  const pts = samples.map((s, i) => `${((i / (samples.length - 1)) * 64).toFixed(1)},${(19 - ((s.value - lo) / (hi - lo || 1)) * 17).toFixed(1)}`).join(' ')
  return <svg viewBox="0 0 64 20" width="64" height="20" aria-hidden="true"><polyline points={pts} fill="none" style={{ stroke: color }} strokeWidth="1.5" /></svg>
}

export function JobPill({ status, xs }: { status: string; xs?: boolean }) {
  const tone: Tone = status === 'succeeded' ? 'ok' : status === 'failed' ? 'crit' : status === 'partial' ? 'warn' : status === 'running' ? 'info' : 'unk'
  const label = status.charAt(0).toUpperCase() + status.slice(1)
  if (status === 'running') return <span className={`pill info${xs ? ' xs' : ''}`}><Ic.refresh />Running</span>
  return <Pill tone={tone} xs={xs}>{label}</Pill>
}

export function JobBar({ done, failed, total, running }: { done: number; failed: number; total: number; running?: boolean }) {
  const ok = Math.max(0, done - failed)
  const pct = (n: number) => (total > 0 ? (n / total) * 100 : 0)
  return <MultiBar parts={[{ pct: pct(ok), color: 'var(--ok)' }, { pct: pct(failed), color: 'var(--crit)' }, { pct: running && total > done ? Math.min(100 - pct(done), 8) : 0, color: 'var(--accent)' }]} label={`${ok} succeeded, ${failed} failed, ${total - done} remaining`} />
}

export function Tabs<T extends string>({ value, onChange, tabs, label }: { value: T; onChange: (v: T) => void; tabs: { v: T; label: React.ReactNode; disabled?: boolean; title?: string }[]; label: string }) {
  return (
    <div className="tabs" role="tablist" aria-label={label}>
      {tabs.map((t) => <button key={t.v} role="tab" aria-selected={value === t.v} disabled={t.disabled} title={t.title} onClick={() => onChange(t.v)} style={t.disabled ? { opacity: .5, cursor: 'default' } : undefined}>{t.label}</button>)}
    </div>
  )
}

/** Provenance marker: operator-set vs discovered. */
export function Mk({ op }: { op: boolean }) {
  return <span className={`mk ${op ? 'op' : 'dc'}`} title={op ? 'Set by operator — never overwritten' : 'Discovered from device'}>{op ? <Ic.user /> : <Ic.discovered />}</span>
}

export function Dialog({ title, sub, icon, children, footer, onClose }: { title: string; sub?: React.ReactNode; icon?: React.ReactNode; children?: React.ReactNode; footer?: React.ReactNode; onClose: () => void }) {
  return (
    <div className="drawer-backdrop" style={{ justifyContent: 'center', alignItems: 'center', padding: 16 }} onClick={onClose}>
      <div className="dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <div style={{ padding: '18px 20px 6px', display: 'flex', gap: 12 }}>
          {icon}
          <div><h3 style={{ fontSize: 16, fontWeight: 600 }}>{title}</h3>{sub && <p className="cap" style={{ margin: '4px 0 0' }}>{sub}</p>}</div>
        </div>
        {children && <div style={{ padding: '6px 20px 14px' }}>{children}</div>}
        {footer && <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, padding: '12px 20px', borderTop: '1px solid var(--border)' }}>{footer}</div>}
      </div>
    </div>
  )
}

export function fmtInt(n: number | undefined | null): string { return n === undefined || n === null ? '—' : n.toLocaleString() }

/** Short duration since a timestamp, e.g. "29 min", "2 d 4 h". */
export function since(s?: string | null): string {
  if (!s) return '—'
  const sec = Math.max(0, Math.floor((Date.now() - new Date(s).getTime()) / 1000))
  if (sec < 60) return `${sec} s`
  if (sec < 3600) return `${Math.floor(sec / 60)} min`
  if (sec < 86400) return `${Math.floor(sec / 3600)} h ${Math.floor((sec % 3600) / 60)} min`
  return `${Math.floor(sec / 86400)} d ${Math.floor((sec % 86400) / 3600)} h`
}
