import { useEffect, useState } from 'react'
import { Ic } from '../icons'
import { Banner, Btn, Dialog, MultiBar, Pill } from '../ui'
import type { Campaign, CampaignDeviceStatus, CampaignSummary } from '../../types-firmware'
import { campaignStatusLabel, deviceStatusLabel } from '../../types-firmware'

/** Campaign lifecycle as a pill: colour + icon + label, pause reason included. */
export function CampaignPill({ c, xs }: { c: Pick<Campaign, 'status' | 'wave' | 'pauseReason' | 'dryRun'>; xs?: boolean }) {
  const label = campaignStatusLabel(c)
  const cls = xs ? ' xs' : ''
  switch (c.status) {
    case 'draft': return <Pill tone="unk" xs={xs} noIcon><Ic.pencil />{label}</Pill>
    case 'scheduled': return <span className={`pill off${cls}`}><Ic.clock />{label}</span>
    case 'running': return <span className={`pill info${cls}`}><Ic.refresh />{label}</span>
    case 'paused': return <span className={`pill ${c.pauseReason === 'canary' ? 'info' : 'warn'}${cls}`}>{c.pauseReason === 'canary' ? <Ic.shieldCheck /> : <Ic.pause />}{label}</span>
    case 'completed': return <Pill tone="ok" xs={xs}>{label}</Pill>
    case 'failed': return <Pill tone="crit" xs={xs}>{label}</Pill>
    case 'cancelled': return <span className={`pill off${cls}`}><Ic.x />{label}</span>
  }
}

/** Per-device progress inside a campaign. */
export function DeviceStatusPill({ status, xs }: { status: CampaignDeviceStatus; xs?: boolean }) {
  const label = deviceStatusLabel(status)
  const cls = xs ? ' xs' : ''
  switch (status) {
    case 'pending': return <span className={`pill off${cls}`}><Ic.clock />{label}</span>
    case 'updating': return <span className={`pill info${cls}`}><Ic.upload />{label}</span>
    case 'verifying': return <span className={`pill info${cls}`}><Ic.refresh />{label}</span>
    case 'succeeded': return <Pill tone="ok" xs={xs}>{label}</Pill>
    case 'failed': return <Pill tone="crit" xs={xs}>{label}</Pill>
    case 'skipped': return <span className={`pill off${cls}`}><Ic.chevronRight />{label}</span>
  }
}

export const statusColor: Record<CampaignDeviceStatus, string> = {
  succeeded: 'var(--ok)', failed: 'var(--crit)', updating: 'var(--accent)', verifying: 'var(--accent)', pending: 'var(--track)', skipped: 'var(--off)',
}

/** Segmented bar for a set of campaign devices: succeeded · failed · updating/verifying · skipped, pending stays as track. */
export function CampaignBar({ s, height = 8, label }: { s: CampaignSummary; height?: number; label?: string }) {
  const t = Math.max(1, s.total)
  const pct = (n: number) => (n / t) * 100
  const inflight = s.updating + s.verifying
  return (
    <MultiBar height={height} label={label || `${s.succeeded} succeeded, ${s.failed} failed, ${inflight} in progress, ${s.pending} pending, ${s.skipped} skipped of ${s.total}`}
      parts={[{ pct: pct(s.succeeded), color: 'var(--ok)' }, { pct: pct(s.failed), color: 'var(--crit)' }, { pct: pct(inflight), color: 'var(--accent)' }, { pct: pct(s.skipped), color: 'var(--off)' }]} />
  )
}

export function Legend({ s, compact }: { s: CampaignSummary; compact?: boolean }) {
  const inflight = s.updating + s.verifying
  const item = (color: string, n: number, what: string, outline?: boolean) => (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><span className="dot" style={{ background: color, boxShadow: outline ? 'inset 0 0 0 1px var(--border-strong)' : undefined }} /><span className="num">{n}</span> {what}</span>
  )
  return (
    <div className="row cap" style={{ gap: compact ? 10 : 14 }}>
      {item('var(--ok)', s.succeeded, 'succeeded')}{item('var(--crit)', s.failed, 'failed')}{item('var(--accent)', inflight, 'in progress')}{item('var(--track)', s.pending, 'pending', true)}{s.skipped > 0 && item('var(--off)', s.skipped, 'skipped')}
    </div>
  )
}

export function summarize(rows: { status: CampaignDeviceStatus }[]): CampaignSummary {
  const s: CampaignSummary = { total: 0, pending: 0, skipped: 0, updating: 0, verifying: 0, succeeded: 0, failed: 0 }
  for (const r of rows) { s.total++; s[r.status]++ }
  return s
}

export function DryRunBanner({ started }: { started?: boolean }) {
  return <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run</b> — nothing is delivered. {started ? 'Planning, pre-flight and driver validation run exactly as a real campaign, but every driver is told not to send anything and no device changes.' : 'This campaign will validate every step without sending an image.'}</Banner>
}

export interface ConfirmRow { label: string; value: React.ReactNode }

/**
 * Confirmation dialog that states the blast radius (design § 10). Above `typedAbove`
 * devices the operator must type `typed` to enable the confirm button; dry runs
 * never ask for typing because they change nothing.
 */
export function ConfirmDialog({ title, sub, tone = 'warn', rows, typed, typedAbove = 10, count = 0, dryRun, confirmLabel, confirmVariant, busy, error, children, onConfirm, onClose, confirmIcon }: {
  title: string; sub?: React.ReactNode; tone?: 'warn' | 'crit' | 'info' | 'ok'; rows?: ConfirmRow[]
  typed?: string; typedAbove?: number; count?: number; dryRun?: boolean
  confirmLabel: string; confirmVariant?: 'pri' | 'wr' | 'dng' | 'dngf'; confirmIcon?: React.ReactNode
  busy?: boolean; error?: string; children?: React.ReactNode; onConfirm: () => void; onClose: () => void
}) {
  const [typedValue, setTypedValue] = useState('')
  const needTyped = !!typed && !dryRun && count > typedAbove
  const ok = !needTyped || typedValue.trim().toUpperCase() === typed!.trim().toUpperCase()
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  const bg = tone === 'warn' ? 'var(--warn-bg)' : tone === 'crit' ? 'var(--crit-bg)' : tone === 'ok' ? 'var(--ok-bg)' : 'var(--info-bg)'
  const fg = tone === 'warn' ? 'var(--warn)' : tone === 'crit' ? 'var(--crit)' : tone === 'ok' ? 'var(--ok)' : 'var(--info)'
  const I = tone === 'crit' ? Ic.crit : tone === 'warn' ? Ic.upload : tone === 'ok' ? Ic.ok : Ic.info
  return (
    <Dialog title={title} sub={sub} onClose={onClose}
      icon={<span style={{ width: 32, height: 32, borderRadius: 8, background: bg, color: fg, display: 'grid', placeItems: 'center', flex: 'none' }}><I /></span>}
      footer={<><Btn onClick={onClose} disabled={busy}>Back</Btn><Btn variant={confirmVariant} onClick={onConfirm} disabled={!ok || busy}>{confirmIcon}{busy ? 'Working…' : confirmLabel}</Btn></>}>
      {rows && rows.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 1fr) 2fr', gap: '6px 16px', padding: '4px 0' }}>
          {rows.map((r) => <RowPair key={r.label} r={r} />)}
        </div>
      )}
      {children}
      {needTyped && (
        <div style={{ marginTop: 10 }}>
          <label className="cap" htmlFor="fw-cfm" style={{ display: 'block', marginBottom: 4 }}>Type <b className="mono" style={{ color: 'var(--text)' }}>{typed}</b> to confirm (required above {typedAbove} devices)</label>
          <input className="fin mono" id="fw-cfm" value={typedValue} onChange={(e) => setTypedValue(e.target.value)} autoFocus autoComplete="off" />
        </div>
      )}
      {error && <div className="error" style={{ marginTop: 10, fontSize: 12 }}>{error}</div>}
    </Dialog>
  )
}

function RowPair({ r }: { r: ConfirmRow }) {
  return <><span className="cap">{r.label}</span><b style={{ fontWeight: 500, minWidth: 0, overflowWrap: 'anywhere' }}>{r.value}</b></>
}

/** Multi-line recovery guidance as the API ships it (numbered steps + vendor note). */
export function RecoveryNotes({ text }: { text: string }) {
  const [vendorNote, ...rest] = text.split('\n\nVendor note: ').reverse()
  const body = rest.length ? rest.reverse().join('\n\n') : text
  return (
    <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={{ whiteSpace: 'pre-wrap', fontSize: 12, color: 'var(--text-2)', lineHeight: 1.5 }}>{body}</div>
      {rest.length > 0 && <div style={{ border: '1px solid var(--border-strong)', borderRadius: 6, padding: '8px 10px', background: 'var(--surface-2)', fontSize: 12 }}><b style={{ fontWeight: 600 }}>Vendor note</b> · {vendorNote}</div>}
    </div>
  )
}
