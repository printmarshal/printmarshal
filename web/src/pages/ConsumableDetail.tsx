import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Consumable, InstalledSupply, ItemForecast, Sample, StockEvent, StockStatus, User } from '../types'
import { useEvents } from '../ws'
import { Spark } from '../components/ui'
import { AdjustStock, ColorSwatch, LevelBar, ModelChips, StockPill, fmtDays, typeLabel } from '../components/consumables'
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

  if (error) return <div className="error">{error}</div>
  if (!d) return <div className="muted">Loading…</div>
  const c = d.item
  const f = d.forecast
  const history: Sample[] = [...d.events].filter((e) => e.status === 'applied').reverse().map((e) => ({ deviceId: '', metric: 'stock', value: e.resulting, at: e.at }))
  const remove = async () => {
    if (!confirm(`Delete "${c.name}" and its stock history?`)) return
    await api.del(`/api/consumables/${c.id}`); navigate('/consumables')
  }
  return (
    <>
      <div className="row between">
        <div>
          <h1 style={{ marginBottom: 4 }} className="row" ><ColorSwatch color={c.color} />{c.name} <StockPill status={d.status} /></h1>
          <div className="muted">{typeLabel(c.type)}{c.color ? ` · ${c.color}` : ''}{c.partNumber ? ` · ${c.partNumber}` : ''} · <strong>{c.onHand}</strong> {c.unit} on hand{c.location ? ` · ${c.location}` : ''}</div>
        </div>
        {canEdit && <div className="row"><button onClick={() => setEditing(true)}>Edit</button><button className="danger" onClick={remove}>Delete</button></div>}
      </div>
      {canEdit && (
        <div className="panel">
          <h2>Adjust stock <span className="muted small">(audited; operator-owned)</span></h2>
          <AdjustStock onHand={c.onHand} onApply={async (body) => { await api.post(`/api/consumables/${c.id}/adjust-stock`, body); load() }} />
        </div>
      )}
      <div className="grid cols-3">
        <div className="panel">
          <h2>Item</h2>
          <dl className="kv">
            <dt>Compatible</dt><dd><ModelChips models={c.compatibleModels} max={8} /></dd>
            <dt>Thresholds</dt><dd>warning ≤ {c.warningAt} · critical ≤ {c.criticalAt}</dd>
            <dt>Unit</dt><dd>{c.unit}</dd>
            <dt>Location</dt><dd>{c.location || '—'}</dd>
            <dt>Notes</dt><dd className="small">{c.notes || '—'}</dd>
            <dt>Reorder</dt><dd>{c.reorderUrl ? <a href={c.reorderUrl} target="_blank" rel="noreferrer">{c.reorderUrl.replace(/^https?:\/\//, '').slice(0, 40)}</a> : '—'}</dd>
            {c.unitCost !== undefined && <><dt>Unit cost</dt><dd>{c.unitCost} <span className="muted small">(reference only)</span></dd></>}
            <dt>Updated</dt><dd>{fmtTime(c.updatedAt)}</dd>
          </dl>
        </div>
        <div className="panel">
          <h2>Consumption &amp; forecast</h2>
          {f.known ? (
            <>
              <div className="stat" style={{ marginBottom: 8 }}>
                <span className="value" style={{ color: f.reorderSoon ? 'var(--warn)' : undefined }}>{f.stockDays !== null ? fmtDays(f.stockDays) : '> 3 years'}</span>
                <span className="label">until on-hand stock is used up{f.reorderSoon ? ' · reorder soon' : ''}</span>
              </div>
              <dl className="kv">
                <dt>Demand</dt><dd>{f.unitsPerDay} units / day across {f.devices.filter((x) => x.rate.known).length} device{f.devices.filter((x) => x.rate.known).length === 1 ? '' : 's'}</dd>
                <dt>Next need</dt><dd>{f.nextNeedDays !== null ? fmtDays(f.nextNeedDays) : '—'}</dd>
              </dl>
              <ForecastChart f={f} />
            </>
          ) : (
            <div className="muted"><strong>Forecast unknown.</strong> {f.reason || 'Not enough history yet.'}{f.linkedDevices > 0 && f.reason !== 'no linked devices' && ' Levels need to be polled over a few days before a rate can be estimated.'}</div>
          )}
        </div>
        <div className="panel">
          <h2>Stock history</h2>
          {history.length >= 2 ? <Spark samples={history} min={0} /> : <div className="muted small">not enough history yet</div>}
          <table style={{ marginTop: 8 }}><tbody>
            {d.events.slice(0, 8).map((e) => (
              <tr key={e.id}><td className="small">{ago(e.at)}</td><td className="small">{e.kind}{e.status !== 'applied' && <span className="muted"> · {e.status}</span>}</td>
                <td className="small" style={{ textAlign: 'right' }}>{e.status === 'applied' ? <>{e.delta > 0 ? `+${e.delta}` : e.delta} → <strong>{e.resulting}</strong></> : '—'}</td>
                <td className="muted small">{e.actor}{e.reason ? ` · ${e.reason}` : ''}</td></tr>
            ))}
          </tbody></table>
        </div>
      </div>
      <div className="panel">
        <h2>Linked devices <span className="muted small">({d.devices.length})</span></h2>
        {d.devices.length === 0 && <div className="muted">No device uses this item yet. Links are made automatically by part number or type + colour + compatible model, or manually from a device's Supplies card.</div>}
        <table>
          {d.devices.length > 0 && <thead><tr><th>Device</th><th>Installed supply</th><th>Level</th><th>Rate</th><th>Days to empty</th><th>Link</th></tr></thead>}
          <tbody>
            {d.devices.map((s) => (
              <tr key={`${s.deviceId}:${s.supply.index}`}>
                <td><Link to={`/devices/${s.deviceId}`}>{s.deviceName}</Link><div className="muted small">{s.deviceModel} · {s.address}</div></td>
                <td>{s.supply.description}</td>
                <td><LevelBar percent={s.supply.percent} filled={s.supply.class === 'filled'} /></td>
                <td className="small">{s.forecast?.rate.known ? `${s.forecast.rate.percentPerDay.toFixed(2)} %/day` : <span className="muted" title={s.forecast?.rate.reason}>unknown</span>}</td>
                <td className="small">{s.forecast?.daysToEmpty != null ? fmtDays(s.forecast.daysToEmpty) : '—'}</td>
                <td className="small muted">{s.linkSource}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="muted small"><Link to="/consumables">← all consumables</Link></div>
      {editing && <ConsumableForm existing={c} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); load() }} />}
    </>
  )
}

/** Projected on-hand stock over time: a step down at each predicted replacement. */
function ForecastChart({ f }: { f: ItemForecast }) {
  const path = useMemo(() => {
    const events: number[] = []
    for (const d of f.devices) {
      if (!d.rate.known || d.daysToEmpty === null) continue
      const period = 100 / d.rate.percentPerDay
      for (let t = d.daysToEmpty; t <= 365 && events.length < 200; t += period) events.push(t)
    }
    events.sort((a, b) => a - b)
    if (events.length === 0) return null
    const horizon = Math.min(365, Math.max(30, (events[Math.min(events.length - 1, f.onHand)] || 30) * 1.2))
    const pts: [number, number][] = [[0, f.onHand]]
    let qty = f.onHand
    for (const t of events) { if (t > horizon) break; pts.push([t, qty]); qty = Math.max(0, qty - 1); pts.push([t, qty]) }
    pts.push([horizon, qty])
    const maxQ = Math.max(1, f.onHand)
    const d = pts.map(([t, q], i) => `${i === 0 ? 'M' : 'L'}${((t / horizon) * 300).toFixed(1)},${(56 - (q / maxQ) * 52).toFixed(1)}`).join(' ')
    return { d, horizon }
  }, [f])
  if (!path) return null
  return (
    <div>
      <svg className="spark" viewBox="0 0 300 60" preserveAspectRatio="none">
        <line x1="0" y1="56" x2="300" y2="56" stroke="var(--border)" />
        <path d={path.d} fill="none" stroke="var(--accent)" strokeWidth="2" vectorEffect="non-scaling-stroke" />
      </svg>
      <div className="row between muted small"><span>today</span><span>projected on-hand stock</span><span>{fmtDays(path.horizon)}</span></div>
    </div>
  )
}
