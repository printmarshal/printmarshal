import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { Device, JobResult } from '../../types'
import { deviceName } from '../../types'
import { Ic } from '../icons'
import { Btn, Pill } from '../ui'
import type { ApplyResultData, CatalogDef, SettingChange, SettingResult } from '../../types-config'
import { areaLabel } from '../../types-config'
import { Diff, OutcomePill, ResultPill, fmtValue, settingName } from './shared'

export type RowKind = 'change' | 'match' | 'skipped' | 'failed' | 'unreachable'
export interface DeviceRow {
  deviceId: string; label: string; model: string; location: string; kind: RowKind; message: string
  writeId?: string; results: SettingResult[]; writes: number; failed: number; skipped: number; unchanged: number
  /** Read-back split of the written settings (real applies only, audit A20). */
  verified: number; diverged: number
}
export interface SettingRow { key: string; desired: unknown; befores: { value: string; n: number }[]; changing: number; skipped: number; failed: number; unchanged: number; reasons: string[] }
export interface Analysis { rows: DeviceRow[]; settings: SettingRow[]; changing: DeviceRow[]; matching: DeviceRow[]; skippedRows: DeviceRow[]; writes: number; touched: string[] }

/** Turns a config.apply job's per-device results into per-device and per-setting views. */
export function analyse(results: JobResult[], devices: Map<string, Device>, profileSettings: SettingChange[]): Analysis {
  const rows: DeviceRow[] = results.map((r) => {
    const data = (r.data || {}) as ApplyResultData
    const list = data.results || []
    const d = r.deviceId ? devices.get(r.deviceId) : undefined
    const writes = list.filter((x) => x.status === 'would_write' || x.status === 'written').length
    const failed = list.filter((x) => x.status === 'failed').length
    const skipped = list.filter((x) => x.status === 'skipped').length
    const unchanged = list.filter((x) => x.status === 'unchanged').length
    const verified = list.filter((x) => x.outcome === 'verified').length
    const diverged = list.filter((x) => x.outcome === 'diverged').length
    const kind: RowKind = list.length === 0 && r.status === 'error' ? 'unreachable' : failed > 0 ? 'failed' : writes > 0 ? 'change' : unchanged === 0 && skipped > 0 ? 'skipped' : 'match'
    return { deviceId: r.deviceId || '', label: d ? deviceName(d) : r.address || r.deviceId || '?', model: d?.model || '', location: d?.location || '', kind, message: r.message || '', writeId: data.writeId, results: list, writes, failed, skipped, unchanged, verified, diverged }
  })
  const order: Record<RowKind, number> = { failed: 0, unreachable: 1, change: 2, skipped: 3, match: 4 }
  rows.sort((a, b) => order[a.kind] - order[b.kind] || a.label.localeCompare(b.label))
  const settings: SettingRow[] = profileSettings.map((s) => {
    const befores = new Map<string, number>()
    let changing = 0, skipped = 0, failed = 0, unchanged = 0
    const reasons = new Set<string>()
    for (const row of rows) for (const x of row.results) {
      if (x.key !== s.key) continue
      if (x.status === 'would_write' || x.status === 'written') { changing++; const v = fmtValue(x.before); befores.set(v, (befores.get(v) || 0) + 1) }
      else if (x.status === 'skipped') { skipped++; if (x.error) reasons.add(x.error) }
      else if (x.status === 'failed') { failed++; if (x.error) reasons.add(x.error) }
      else if (x.status === 'unchanged') unchanged++
    }
    return { key: s.key, desired: s.value, befores: Array.from(befores, ([value, n]) => ({ value, n })).sort((a, b) => b.n - a.n), changing, skipped, failed, unchanged, reasons: Array.from(reasons) }
  })
  const changing = rows.filter((r) => r.kind === 'change' || r.kind === 'failed')
  return {
    rows, settings, changing, matching: rows.filter((r) => r.kind === 'match'), skippedRows: rows.filter((r) => r.kind === 'skipped' || r.kind === 'unreachable'),
    writes: rows.reduce((n, r) => n + r.writes, 0), touched: settings.filter((s) => s.changing > 0).map((s) => s.key),
  }
}

export function kindPill(row: DeviceRow, dry: boolean) {
  switch (row.kind) {
    case 'change':
      if (dry) return <Pill tone="warn" noIcon><Ic.pencil />{row.writes} change{row.writes === 1 ? '' : 's'}</Pill>
      if (row.diverged > 0) return <Pill tone="warn" title="The device accepted the write but reports a different value">{row.diverged} diverged · {row.writes} written</Pill>
      if (row.verified === row.writes) return <Pill tone="ok" title="Read back from the device">{row.writes} verified</Pill>
      return <Pill tone="ok">{row.writes} written{row.verified ? ` · ${row.verified} verified` : ''}</Pill>
    case 'match': return <Pill tone="ok">Already matches</Pill>
    case 'skipped': return <Pill tone="off">Skipped</Pill>
    case 'unreachable': return <Pill tone="off">Unreachable · skip</Pill>
    case 'failed': return <Pill tone="crit">{row.failed} failed{row.writes ? ` · ${row.writes} ${dry ? 'would write' : 'written'}` : ''}</Pill>
  }
}

/** "Changes by setting": one row per profile setting with the current values found. */
export function SettingRows({ settings, byKey, dry }: { settings: SettingRow[]; byKey: Map<string, CatalogDef>; dry: boolean }) {
  const active = settings.filter((s) => s.changing > 0 || s.failed > 0)
  const quiet = settings.filter((s) => s.changing === 0 && s.failed === 0)
  return (
    <>
      {active.length === 0 ? <div className="cb cap">No setting would change — every reachable device already matches, or the settings are not writable on them.</div> : (
        <div className="tw"><table className="tbl" style={{ minWidth: 820 }}>
          <thead><tr><th>Area</th><th>Setting</th><th>Current values found</th><th /><th>Desired</th><th className="r">Devices {dry ? 'changing' : 'written'}</th></tr></thead>
          <tbody>{active.map((s) => { const def = byKey.get(s.key); return (
            <tr key={s.key}>
              <td className="cap">{areaLabel(def?.area || 'other')}</td>
              <td style={{ fontWeight: 500 }}>{settingName(s.key, byKey)}<div className="mono cap" style={{ fontSize: 10 }}>{s.key}</div></td>
              <td style={{ whiteSpace: 'normal' }}>{s.befores.map((b) => <span key={b.value} style={{ marginRight: 8 }}><span className="old mono">{b.value}</span> <span className="cap">×{b.n}</span></span>)}{s.failed > 0 && <div style={{ fontSize: 11, color: 'var(--crit)' }}>{s.failed} failed · {s.reasons.join('; ')}</div>}</td>
              <td><Ic.arrowRight style={{ color: 'var(--text-3)' }} /></td>
              <td><span className="new-v mono">{fmtValue(s.desired, def)}</span></td>
              <td className="num r">{s.changing}</td>
            </tr>) })}</tbody>
        </table></div>
      )}
      {quiet.length > 0 && <div className="cf cap" style={{ whiteSpace: 'normal' }}>{quiet.length} setting{quiet.length === 1 ? '' : 's'} with no writes: {quiet.map((s) => <span key={s.key} style={{ marginRight: 10 }}>{settingName(s.key, byKey)} <span className="num">({s.unchanged ? `${s.unchanged} match` : ''}{s.unchanged && s.skipped ? ', ' : ''}{s.skipped ? `${s.skipped} skipped${s.reasons.length ? ` — ${s.reasons[0]}` : ''}` : ''})</span></span>)}</div>}
    </>
  )
}

/** "Changes by device": expandable rows with per-setting before → after. */
export function DeviceRows({ rows, byKey, dry, initialOpen = 3 }: { rows: DeviceRow[]; byKey: Map<string, CatalogDef>; dry: boolean; initialOpen?: number }) {
  const [open, setOpen] = useState<Record<string, boolean>>(() => Object.fromEntries(rows.filter((r) => r.kind !== 'match').slice(0, initialOpen).map((r) => [r.deviceId, true])))
  const [showAll, setShowAll] = useState(rows.length <= 25)
  const shown = showAll ? rows : rows.slice(0, 25)
  return (
    <>
      <div className="ch" style={{ paddingTop: 8, paddingBottom: 8 }}><span className="cap num">{rows.length} device{rows.length === 1 ? '' : 's'} · failures first</span><div className="row"><Btn sm variant="ghost" onClick={() => setOpen(Object.fromEntries(rows.map((r) => [r.deviceId, true])))}>Expand all</Btn><Btn sm variant="ghost" onClick={() => setOpen({})}>Collapse all</Btn></div></div>
      {shown.map((r) => {
        const isOpen = !!open[r.deviceId]
        return (
          <div key={r.deviceId || r.label}>
            <button type="button" aria-expanded={isOpen} onClick={() => setOpen((o) => ({ ...o, [r.deviceId]: !isOpen }))} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12, width: '100%', padding: '10px 16px', border: 0, borderBottom: '1px solid var(--border)', background: 'transparent', font: 'inherit', color: 'var(--text)', textAlign: 'left', cursor: 'pointer' }}>
              <Ic.chevronRight style={{ color: 'var(--text-3)', transform: isOpen ? 'rotate(90deg)' : undefined, transition: 'transform .12s' }} />
              <span style={{ fontWeight: 500, minWidth: 120 }}>{r.label}</span>
              <span className="mono cap" style={{ minWidth: 110 }}>{r.model}</span>
              <span className="cap" style={{ flex: '1 1 160px' }}>{r.location}</span>
              {kindPill(r, dry)}
            </button>
            {isOpen && (
              <div style={{ padding: '4px 16px 12px 48px', borderBottom: '1px solid var(--border)', background: 'var(--surface-2)' }}>
                {r.results.map((x) => (
                  <div key={x.key} style={{ display: 'grid', gridTemplateColumns: 'minmax(160px, 220px) minmax(0, 1fr) auto', gap: 12, alignItems: 'center', padding: '6px 0', borderBottom: '1px dashed var(--border)' }}>
                    <span className="cap" style={{ color: 'var(--text-2)' }} title={x.key}>{settingName(x.key, byKey)}</span>
                    <span style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
                      {x.status === 'would_write' || x.status === 'written' ? <Diff before={x.before} after={x.after} def={byKey.get(x.key)} /> : x.status === 'unchanged' ? <span className="mono">{fmtValue(x.before ?? x.after, byKey.get(x.key))}</span> : <span className="cap">{x.error || (x.status === 'skipped' ? 'not supported by any driver on this device' : x.status)}</span>}
                      {x.driver && <span className="tag" title="driver">{x.driver}</span>}
                      {x.outcome === 'diverged' && <span className="cap" style={{ color: 'var(--warn)' }}>device reports <span className="mono">{fmtValue(x.readBack, byKey.get(x.key))}</span></span>}
                    </span>
                    <span style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                      <ResultPill status={x.status} xs />
                      {x.outcome && <OutcomePill outcome={x.outcome} xs />}
                    </span>
                  </div>
                ))}
                {r.kind === 'unreachable' && <div className="cap" style={{ padding: '6px 0' }}>{r.message || 'Could not read the device.'} It is listed as skipped; re-run the preview when it is back, or check it from the drift report.</div>}
                {r.kind === 'match' && <div className="cap" style={{ padding: '6px 0' }}>All {r.unchanged} profile setting{r.unchanged === 1 ? '' : 's'} already match. No writes.</div>}
                {r.deviceId && <div style={{ paddingTop: 6 }}><Link to={`/devices/${r.deviceId}`} className="cap" style={{ color: 'var(--accent)' }}>Open device →</Link></div>}
              </div>
            )}
          </div>
        )
      })}
      {!showAll && <div className="cb cap num">+ {rows.length - 25} more devices · <button className="link" onClick={() => setShowAll(true)}>Show all {rows.length}</button></div>}
    </>
  )
}
