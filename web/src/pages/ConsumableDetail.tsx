import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Consumable, InstalledSupply, ItemForecast, Sample, StockEvent, StockStatus, User } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, Mk, Spark, Tag } from '../components/ui'
import { AdjustStock, LevelBar, ModelChips, StockPill, fmtDays, swatchFor, typeLabel } from '../components/consumables'
import ConsumableForm from '../components/ConsumableForm'

interface Detail { item: Consumable; status: StockStatus; devices: InstalledSupply[]; forecast: ItemForecast; events: StockEvent[] }

export default function ConsumableDetail({ user }: { user: User }) {
  const { id } = useParams()
  const navigate = useNavigate()
  const [d, setD] = useState<Detail | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const canEdit = user.role !== 'viewer'
  const load = useCallback(() => { api.get<Detail>(`/api/consumables/${id}`).then(setD).catch((e) => setError(e.message)) }, [id])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('consumable.') || e.type === 'device.updated') load() }, [load])

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!d) return <div className="cap">Loading…</div>
  const c = d.item
  const f = d.forecast
  const history: Sample[] = [...d.events].filter((e) => e.status === 'applied').reverse().map((e) => ({ deviceId: '', metric: 'stock', value: e.resulting, at: e.at }))
  const remove = async () => { if (!confirm(`Delete "${c.name}" and its stock history?`)) return; await api.del(`/api/consumables/${c.id}`); navigate('/consumables') }
  const lowDevices = d.devices.filter((s) => s.supply.percent !== null && (s.supply.class === 'filled' ? 100 - s.supply.percent : s.supply.percent) <= 20).length
  const days = f.known && f.stockDays !== null ? f.stockDays : null
  return (
    <>
      <nav aria-label="Breadcrumb" className="cap"><Link to="/consumables">Consumables</Link> / <span style={{ color: 'var(--text-2)' }}>{c.name}</span></nav>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 14 }}>
          <span style={{ width: 44, height: 44, borderRadius: 8, background: swatchFor(c.color) || 'repeating-linear-gradient(135deg,var(--surface-2) 0 3px,var(--border-strong) 3px 4px)', boxShadow: 'inset 0 0 0 1px rgba(0,0,0,.15)', flex: 'none' }} />
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10 }}><h1 className="h1">{c.name}</h1><StockPill status={d.status} /></div>
            <div className="cap" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 14px', color: 'var(--text-2)' }}><span>{typeLabel(c.type)}{c.color ? ` · ${c.color}` : ''}</span>{c.partNumber && <span className="mono">{c.partNumber}</span>}<span>unit: {c.unit}</span>{c.location && <span>{c.location}</span>}</div>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 18 }}>
          <div style={{ textAlign: 'right' }}><div className="cap">On hand</div><div className="num" style={{ fontSize: 32, fontWeight: 600, lineHeight: 1.1 }}>{c.onHand}</div></div>
          <div style={{ textAlign: 'right' }}><div className="cap">Days remaining</div><div className="num" style={{ fontSize: 32, fontWeight: 600, lineHeight: 1.1, color: f.reorderSoon ? 'var(--warn)' : undefined }}>{days === null ? '—' : days < 1 ? '< 1' : `~${Math.round(days)}`}</div></div>
          {canEdit && <div className="row"><Btn sm onClick={() => setEditing(true)}><Ic.pencil />Edit</Btn><Btn sm variant="dng" onClick={remove}>Delete</Btn></div>}
        </div>
      </div>
      {canEdit && (
        <Card title="Adjust stock" id="adj" sub="changes PrintMarshal's count only · logged with reason">
          <div className="cb"><AdjustStock onHand={c.onHand} onApply={async (body) => { await api.post(`/api/consumables/${c.id}/adjust-stock`, body); load() }} /></div>
        </Card>
      )}
      <div className="cols">
        <div className="stack" style={{ flex: '2 1 620px' }}>
          <Card title="Consumption & forecast" id="fc" sub={f.known ? `fleet uses ~${(f.unitsPerDay * 7).toFixed(1)} / week · ${f.devices.filter((x) => x.rate.known).length} device${f.devices.filter((x) => x.rate.known).length === 1 ? '' : 's'} with a known rate` : 'not enough history yet'}>
            <div className="cb">
              {f.known ? (<>
                <ForecastChart f={f} />
                {f.reorderSoon && <div className="banner warn" style={{ marginTop: 12 }}><Ic.warn style={{ color: 'var(--warn)' }} /><span>Stock runs out inside the reorder lead time. <b>Reorder now</b> to avoid days without spares.</span></div>}
              </>) : <div className="cap"><b>Forecast unknown.</b> {f.reason || 'Not enough history yet.'} Levels on linked devices need to be polled over a few days before a rate can be estimated.</div>}
            </div>
          </Card>
          <Card title="Stock history" id="hist" sub="every change, with who and why">
            {history.length >= 2 && <div className="cb" style={{ paddingBottom: 0 }}><Spark samples={history} min={0} dot /></div>}
            {d.events.length === 0 ? <div className="cb cap">No stock events yet.</div> : (
              <div className="tw"><table className="tbl" style={{ minWidth: 620 }}>
                <thead><tr><th>When</th><th className="r">Change</th><th className="r">On hand</th><th>Reason</th><th>By</th></tr></thead>
                <tbody>{d.events.map((e) => (
                  <tr key={e.id} className={e.status !== 'applied' ? 'dim' : ''}><td className="cap num" title={fmtTime(e.at)}>{ago(e.at)}</td>
                    <td className="num r" style={{ fontWeight: 600, color: e.delta > 0 ? 'var(--ok)' : e.kind === 'set' ? 'var(--accent)' : undefined }}>{e.status === 'applied' ? (e.kind === 'set' ? `= ${e.resulting}` : e.delta > 0 ? `+${e.delta}` : `−${Math.abs(e.delta)}`) : '—'}</td>
                    <td className="num r">{e.status === 'applied' ? e.resulting : '—'}</td><td>{e.reason || e.kind}{e.status !== 'applied' && <Tag>{e.status}</Tag>}{e.deviceId && <> · <Link to={`/devices/${e.deviceId}`}>device</Link></>}</td><td className="cap">{e.actor}</td></tr>
                ))}</tbody>
              </table></div>
            )}
          </Card>
        </div>
        <div className="stack" style={{ flex: '1 1 360px' }}>
          <Card title="Item details" id="inf" right={canEdit ? <Btn sm onClick={() => setEditing(true)}>Edit</Btn> : undefined}>
            <div style={{ padding: '6px 0' }}>
              <div className="frow" style={{ alignItems: 'start' }}><span className="cap" style={{ paddingTop: 2 }}>Compatible models</span><span><ModelChips models={c.compatibleModels} max={8} /></span><Mk op /></div>
              <div className="frow"><span className="cap">Warning at</span><span className="num">{c.warningAt} {c.unit}</span><Mk op /></div>
              <div className="frow"><span className="cap">Critical at</span><span className="num">{c.criticalAt} {c.unit}</span><Mk op /></div>
              <div className="frow"><span className="cap">Storeroom</span><span>{c.location || '—'}</span><Mk op /></div>
              <div className="frow"><span className="cap">Notes</span><span>{c.notes || '—'}</span><Mk op /></div>
              <div className="frow"><span className="cap">Reorder link</span><span>{c.reorderUrl ? <a href={c.reorderUrl} className="mono" target="_blank" rel="noreferrer" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'block' }}>{c.reorderUrl.replace(/^https?:\/\//, '')}</a> : '—'}</span><Mk op /></div>
              {c.unitCost !== undefined && <div className="frow"><span className="cap">Unit cost</span><span className="num">{c.unitCost} <span className="cap">(reference only)</span></span><Mk op /></div>}
              <div className="frow"><span className="cap">Updated</span><span className="cap">{fmtTime(c.updatedAt)}</span><span /></div>
            </div>
          </Card>
          <Card title="Linked devices" id="ld" sub={`${d.devices.length} · lowest installed level first`}>
            {lowDevices > c.onHand && <div style={{ padding: '10px 16px', display: 'flex', alignItems: 'center', gap: 8, borderBottom: '1px solid var(--border)', background: 'var(--surface-2)' }}><Ic.warn style={{ color: 'var(--warn)' }} /><span><b className="num">{lowDevices}</b> devices are at ≤ 20% — demand exceeds the <b className="num">{c.onHand}</b> on hand.</span></div>}
            {d.devices.length === 0 ? <div className="cb cap">No device uses this item yet. Links are made automatically by part number or type + colour + compatible model, or manually from a device's Supplies card.</div> : d.devices.slice().sort((a, b) => (a.supply.percent ?? 101) - (b.supply.percent ?? 101)).map((s) => (
              <div key={`${s.deviceId}:${s.supply.index}`} style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) auto', gap: 10, alignItems: 'center', padding: '8px 16px', borderBottom: '1px solid var(--border)' }}>
                <span style={{ minWidth: 0 }}><Link to={`/devices/${s.deviceId}`} style={{ fontWeight: 500 }}>{s.deviceName}</Link><span className="cap" style={{ display: 'block', fontSize: 11 }}>{s.deviceModel}{s.linkSource === 'manual' ? ' · manual link' : ''}{s.forecast?.daysToEmpty != null ? ` · empty in ${fmtDays(s.forecast.daysToEmpty)}` : ''}</span></span>
                <LevelBar percent={s.supply.percent} filled={s.supply.class === 'filled'} />
              </div>
            ))}
            <div className="cf cap"><Link to="/consumables?tab=fleet">See all in fleet supplies →</Link></div>
          </Card>
        </div>
      </div>
      {editing && <ConsumableForm existing={c} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); load() }} />}
    </>
  )
}

/** Projected on-hand stock over time: a step down at each predicted replacement. */
function ForecastChart({ f }: { f: ItemForecast }) {
  const path = useMemo(() => {
    const events: number[] = []
    for (const d of f.devices) { if (!d.rate.known || d.daysToEmpty === null) continue; const period = 100 / d.rate.percentPerDay; for (let t = d.daysToEmpty; t <= 365 && events.length < 200; t += period) events.push(t) }
    events.sort((a, b) => a - b)
    if (events.length === 0) return null
    const horizon = Math.min(365, Math.max(30, (events[Math.min(events.length - 1, f.onHand)] || 30) * 1.2))
    const pts: [number, number][] = [[0, f.onHand]]
    let qty = f.onHand
    for (const t of events) { if (t > horizon) break; pts.push([t, qty]); qty = Math.max(0, qty - 1); pts.push([t, qty]) }
    pts.push([horizon, qty])
    const maxQ = Math.max(1, f.onHand)
    const X = (t: number) => 30 + (t / horizon) * 600, Y = (q: number) => 160 - (q / maxQ) * 150
    const d = pts.map(([t, q], i) => `${i === 0 ? 'M' : 'L'}${X(t).toFixed(1)},${Y(q).toFixed(1)}`).join(' ')
    const zero = pts.find(([, q]) => q === 0)
    return { d, horizon, zeroAt: zero ? X(zero[0]) : null, zeroDays: zero ? zero[0] : null, maxQ }
  }, [f])
  if (!path) return <div className="cap">No projected replacements inside a year.</div>
  return (
    <>
      <svg viewBox="0 0 640 190" width="100%" height="190" role="img" aria-label={`Projected on-hand stock over ${Math.round(path.horizon)} days${path.zeroDays !== null ? `, reaching zero in about ${Math.round(path.zeroDays)} days` : ''}`}>
        <g style={{ stroke: 'var(--border)' }}><line x1="30" y1="10" x2="640" y2="10" /><line x1="30" y1="85" x2="640" y2="85" /><line x1="30" y1="160" x2="640" y2="160" /></g>
        <g fontSize="10" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--text-3)' }}><text x="4" y="14">{path.maxQ}</text><text x="4" y="89">{Math.round(path.maxQ / 2)}</text><text x="10" y="164">0</text><text x="30" y="180">Today</text><text x="590" y="180">+{Math.round(path.horizon)} d</text></g>
        <path d={path.d} fill="none" style={{ stroke: 'var(--accent)' }} strokeWidth="2" />
        {path.zeroAt !== null && <><circle cx={path.zeroAt} cy="160" r="4" style={{ fill: 'var(--crit)' }} /><text x={Math.min(path.zeroAt + 6, 560)} y="152" fontSize="11" fontWeight="600" fontFamily="IBM Plex Sans, sans-serif" style={{ fill: 'var(--crit)' }}>0 in ~{Math.round(path.zeroDays!)} d</text></>}
      </svg>
      <div className="cap" style={{ display: 'flex', gap: 16, marginTop: 6 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 16, height: 2, background: 'var(--accent)' }} />Projected on hand</span><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span className="dot" style={{ background: 'var(--crit)' }} />Runs out</span></div>
    </>
  )
}
