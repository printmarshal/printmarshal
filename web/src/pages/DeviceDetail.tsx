import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Alert, Consumable, Device, InstalledSupply, Sample, StockEvent, User } from '../types'
import { deviceName, makeModel } from '../types'
import { useEvents } from '../ws'
import { Badge, SeverityBadge, Spark, StateBadge, SupplyRow } from '../components/ui'
import { ConsumableLink, StockPill, fmtDays, statusOf } from '../components/consumables'

const transitions: Record<string, string[]> = {
  discovered: ['onboarded', 'active', 'decommissioned'],
  onboarded: ['active', 'needs_attention', 'decommissioned'],
  active: ['needs_attention', 'decommissioned'],
  needs_attention: ['active', 'decommissioned'],
  decommissioned: ['onboarded'],
}

export default function DeviceDetail({ user }: { user: User }) {
  const { id } = useParams()
  const [device, setDevice] = useState<Device | null>(null)
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [samples, setSamples] = useState<Sample[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    api.get<{ device: Device; alerts: Alert[] }>(`/api/devices/${id}`).then((r) => { setDevice(r.device); setAlerts(r.alerts) }).catch((e) => setError(e.message))
    api.get<Sample[]>(`/api/devices/${id}/telemetry?metric=counter.total`).then(setSamples).catch(() => {})
  }, [id])
  useEffect(() => { load() }, [load])
  useEvents((e) => {
    const d = e.data as { id?: string; deviceId?: string }
    if (d && (d.id === id || d.deviceId === id)) load()
  }, [id, load])

  const poll = async () => {
    setBusy(true); setNote('')
    try {
      const r = await api.post<{ device: Device; servedBy: Record<string, string>; errors: Record<string, string>; reachable: boolean }>(`/api/devices/${id}/poll`)
      setDevice(r.device)
      const errs = Object.entries(r.errors || {})
      const by = Object.values(r.servedBy || {}).filter((v, i, a) => a.indexOf(v) === i).join(', ')
      setNote(r.reachable ? `Polled via ${by}${errs.length ? ` · ${errs.map(([k, v]) => `${k}: ${v}`).join('; ')}` : ''}` : `Unreachable: ${errs.map(([, v]) => v).join('; ')}`)
      load()
    } catch (e) { setNote((e as Error).message) } finally { setBusy(false) }
  }
  const setState = async (state: string) => {
    try { setDevice(await api.post<Device>(`/api/devices/${id}/state`, { state })) } catch (e) { setNote((e as Error).message) }
  }
  const ack = async (a: Alert) => { await api.post(`/api/alerts/${a.id}/ack`); load() }

  if (error) return <div className="error">{error}</div>
  if (!device) return <div className="muted">Loading…</div>
  const d = device
  const inputs = d.trays.filter((t) => t.kind === 'input')
  const outputs = d.trays.filter((t) => t.kind === 'output')
  return (
    <>
      <div className="row between">
        <div>
          <h1 style={{ marginBottom: 4 }}>{deviceName(d)} <SeverityBadge d={d} /> <StateBadge state={d.state} /></h1>
          <div className="muted">{makeModel(d)} · {d.address}{d.serial ? ` · S/N ${d.serial}` : ''}{d.adminUrl && <> · <a href={d.adminUrl} target="_blank" rel="noreferrer">device web page</a></>}</div>
        </div>
        {canOperate && (
          <div className="row">
            <button className="primary" onClick={poll} disabled={busy}>{busy ? 'Polling…' : 'Poll now'}</button>
            <select value="" onChange={(e) => e.target.value && setState(e.target.value)}>
              <option value="">Change state…</option>
              {(transitions[d.state] || []).map((s) => <option key={s} value={s}>{s.replace('_', ' ')}</option>)}
            </select>
          </div>
        )}
      </div>
      {note && <div className="muted small" style={{ margin: '8px 0' }}>{note}</div>}
      {d.status && (d.status.message || (d.status.errors && d.status.errors.length > 0)) && (
        <div className="panel"><strong>Device says:</strong> {d.status.message || '—'}{d.status.errors && d.status.errors.length > 0 && <span className="muted"> · {d.status.errors.join(', ')}</span>}</div>
      )}
      <div className="grid cols-3">
        <SuppliesCard d={d} canOperate={canOperate} />
        <div className="panel">
          <h2>Trays</h2>
          {inputs.length === 0 && <div className="muted">No tray data.</div>}
          <table><tbody>
            {inputs.map((t) => (
              <tr key={`in-${t.index}`}><td>{t.name}<div className="muted small">{[t.mediaSize, t.mediaName, t.mediaType].filter(Boolean).join(' · ')}</div></td>
                <td><Badge sev={t.status === 'ok' ? 'ok' : t.status === 'empty' || t.status === 'unavailable' ? 'critical' : t.status === 'low' ? 'warning' : 'outline'}>{t.status}</Badge></td>
                <td className="small">{t.percent !== null ? `${t.percent}%` : t.level === -3 ? 'some' : ''}</td></tr>
            ))}
            {outputs.map((t) => (
              <tr key={`out-${t.index}`}><td>{t.name}<div className="muted small">output</div></td><td><Badge sev={t.status === 'ok' ? 'ok' : 'warning'}>{t.status}</Badge></td><td /></tr>
            ))}
          </tbody></table>
        </div>
        <div className="panel">
          <h2>Counters</h2>
          {d.counters ? (
            <dl className="kv">
              <dt>Total pages</dt><dd>{d.counters.total.toLocaleString()}</dd>
              {d.counters.mono !== undefined && <><dt>Mono</dt><dd>{d.counters.mono.toLocaleString()}</dd></>}
              {d.counters.color !== undefined && <><dt>Colour</dt><dd>{d.counters.color.toLocaleString()}</dd></>}
              {d.counters.powerOn !== undefined && <><dt>Since power-on</dt><dd>{d.counters.powerOn.toLocaleString()}</dd></>}
            </dl>
          ) : <div className="muted">No counter data.</div>}
          <h2 style={{ marginTop: 12 }}>Page count, last 7 days</h2>
          <Spark samples={samples} />
        </div>
      </div>
      <div className="grid cols-2">
        <OperatorFields d={d} canEdit={canOperate} onSaved={setDevice} />
        <div className="panel">
          <h2>Identity</h2>
          <dl className="kv">
            <dt>Hostname</dt><dd>{d.hostname || '—'}</dd>
            <dt>MAC</dt><dd>{d.mac || '—'}</dd>
            <dt>UUID</dt><dd>{d.uuid || '—'}</dd>
            <dt>Firmware</dt><dd>{d.firmware || '—'}</dd>
            <dt>Type</dt><dd>{d.deviceType || '—'}</dd>
            <dt>sysDescr</dt><dd className="small">{d.sysDescr || '—'}</dd>
            <dt>Enterprise</dt><dd>{d.enterpriseId || '—'}</dd>
            <dt>Found by</dt><dd>{d.discoveryMethods.join(', ') || '—'}</dd>
            <dt>Identified by</dt><dd>{d.identifyMethods.join(', ') || '—'}</dd>
            <dt>Capabilities</dt><dd className="small">{Object.entries(d.capabilities).map(([c, by]) => `${c} (${by})`).join(', ') || '—'}</dd>
            <dt>First seen</dt><dd>{fmtTime(d.firstSeen)}</dd>
            <dt>Last seen</dt><dd>{fmtTime(d.lastSeen)} ({ago(d.lastSeen)})</dd>
            <dt>Last polled</dt><dd>{fmtTime(d.lastPolled)}</dd>
            <dt>Uptime</dt><dd>{d.status?.uptimeSecs ? `${Math.floor(d.status.uptimeSecs / 86400)}d ${Math.floor((d.status.uptimeSecs % 86400) / 3600)}h` : '—'}</dd>
          </dl>
        </div>
      </div>
      <div className="panel">
        <h2>Active alerts</h2>
        {alerts.length === 0 && <div className="muted">None.</div>}
        <table><tbody>
          {alerts.map((a) => (
            <tr key={a.id}><td><Badge sev={a.severity}>{a.severity}</Badge></td><td>{a.description}<div className="muted small">{a.source} · raised {ago(a.raisedAt)}{a.ackedBy ? ` · acked by ${a.ackedBy}` : ''}</div></td>
              <td style={{ textAlign: 'right' }}>{canOperate && !a.ackedAt && <button onClick={() => ack(a)}>Acknowledge</button>}</td></tr>
          ))}
        </tbody></table>
        {d.status?.alerts && d.status.alerts.length > 0 && (
          <>
            <h2 style={{ marginTop: 12 }}>Device-reported alerts</h2>
            <table><tbody>
              {d.status.alerts.map((a) => <tr key={a.index}><td><Badge sev={a.severity}>{a.severity}</Badge></td><td>{a.description}<div className="muted small">{a.group}{a.code ? ` · ${a.code}` : ''}</div></td></tr>)}
            </tbody></table>
          </>
        )}
      </div>
      <div className="muted small"><Link to="/devices">← all devices</Link></div>
    </>
  )
}

function OperatorFields({ d, canEdit, onSaved }: { d: Device; canEdit: boolean; onSaved: (d: Device) => void }) {
  const [form, setForm] = useState({ displayName: d.displayName, location: d.location, assetTag: d.assetTag, notes: d.notes })
  const [msg, setMsg] = useState('')
  useEffect(() => setForm({ displayName: d.displayName, location: d.location, assetTag: d.assetTag, notes: d.notes }), [d.id, d.displayName, d.location, d.assetTag, d.notes])
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try { onSaved(await api.patch<Device>(`/api/devices/${d.id}`, form)); setMsg('Saved.') } catch (err) { setMsg((err as Error).message) }
  }
  return (
    <form className="panel" onSubmit={save}>
      <h2>Operator fields <span className="muted small">(never overwritten by scans)</span></h2>
      <div className="grid cols-2">
        <label className="field">Display name<input value={form.displayName} disabled={!canEdit} onChange={(e) => setForm({ ...form, displayName: e.target.value })} /></label>
        <label className="field">Asset tag<input value={form.assetTag} disabled={!canEdit} onChange={(e) => setForm({ ...form, assetTag: e.target.value })} /></label>
      </div>
      <label className="field" style={{ marginTop: 8 }}>Location<input value={form.location} disabled={!canEdit} onChange={(e) => setForm({ ...form, location: e.target.value })} /></label>
      <label className="field" style={{ marginTop: 8 }}>Notes<textarea rows={3} value={form.notes} disabled={!canEdit} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
      {canEdit && <div className="row" style={{ marginTop: 10 }}><button className="primary">Save</button><span className="muted small">{msg}</span></div>}
    </form>
  )
}

/** Supplies card: installed levels linked to stock items, with "mark replaced". */
function SuppliesCard({ d, canOperate }: { d: Device; canOperate: boolean }) {
  const [rows, setRows] = useState<InstalledSupply[] | null>(null)
  const [pending, setPending] = useState<StockEvent[]>([])
  const [items, setItems] = useState<Consumable[]>([])
  const [msg, setMsg] = useState('')
  const load = useCallback(() => {
    api.get<{ supplies: InstalledSupply[]; pendingReplacements: StockEvent[] }>(`/api/devices/${d.id}/supplies`).then((r) => { setRows(r.supplies); setPending(r.pendingReplacements) }).catch(() => setRows([]))
    if (canOperate) api.get<{ items: Consumable[] }>('/api/consumables').then((r) => setItems(r.items)).catch(() => {})
  }, [d.id, canOperate])
  useEffect(() => { load() }, [load, d.lastPolled])
  useEvents((e) => { if (e.type.startsWith('consumable.')) load() }, [load])
  const markReplaced = async (r: InstalledSupply) => {
    const c = r.consumable
    const dec = c ? confirm(`Take one "${c.name}" out of stock? (${c.onHand} on hand now)\nCancel to record the replacement without changing stock.`) : false
    try { await api.post(`/api/devices/${d.id}/supplies/${r.supply.index}/mark-replaced`, { decrement: dec }); setMsg(dec ? 'Stock decremented.' : 'Replacement recorded.'); load() } catch (e) { setMsg((e as Error).message) }
  }
  const link = async (r: InstalledSupply, consumableId: string) => {
    try {
      if (consumableId) await api.put(`/api/devices/${d.id}/supplies/${r.supply.index}/link`, { consumableId })
      else await api.del(`/api/devices/${d.id}/supplies/${r.supply.index}/link`)
      load()
    } catch (e) { setMsg((e as Error).message) }
  }
  const act = async (ev: StockEvent, what: 'confirm' | 'dismiss') => { try { await api.post(`/api/consumables/replacements/${ev.id}/${what}`); load() } catch (e) { setMsg((e as Error).message) } }
  const other = d.supplies.filter((s) => s.class === 'other')
  return (
    <div className="panel">
      <h2>Supplies</h2>
      {pending.map((ev) => (
        <div className="note small" key={ev.id}>Replacement detected: {ev.reason}. {canOperate && <><button className="link" onClick={() => act(ev, 'confirm')}>Take 1 from stock</button> · <button className="link" onClick={() => act(ev, 'dismiss')}>Dismiss</button></>}</div>
      ))}
      {rows === null && <div className="muted">Loading…</div>}
      {rows !== null && rows.length === 0 && other.length === 0 && <div className="muted">No supply data.</div>}
      {rows?.map((r) => (
        <div className="supply-row" key={r.supply.index}>
          <SupplyRow s={r.supply} />
          <div className="actions">{canOperate && r.consumable && <button onClick={() => markReplaced(r)} title="Record that this supply was replaced and optionally decrement stock">Mark replaced</button>}</div>
          <div className="stock">
            {r.consumable ? (<>
              <ConsumableLink c={r.consumable} /><StockPill status={statusOf(r.consumable)} /><span>{r.consumable.onHand} in stock</span>
              {r.linkSource === 'manual' && <span>· manual link</span>}
              {r.forecast?.daysToEmpty != null && <span>· empty in {fmtDays(r.forecast.daysToEmpty)}</span>}
            </>) : <span>No stock item linked{canOperate ? '' : '.'}</span>}
            {canOperate && (
              <select value={r.linkSource === 'manual' ? r.consumable?.id || '' : ''} onChange={(e) => link(r, e.target.value)} title="Link this slot to a stock item (operator link; auto-matching never overrides it)">
                <option value="">{r.consumable ? 'auto-linked · change…' : 'link to…'}</option>
                {items.map((i) => <option key={i.id} value={i.id}>{i.name}{i.color ? ` (${i.color})` : ''}</option>)}
                {r.linkSource === 'manual' && <option value="">remove manual link</option>}
              </select>
            )}
            {!r.consumable && canOperate && <Link to="/consumables?tab=unmatched" className="small">create item →</Link>}
          </div>
        </div>
      ))}
      {other.map((s) => <SupplyRow key={`o-${s.index}`} s={s} />)}
      {msg && <div className="muted small" style={{ marginTop: 6 }}>{msg}</div>}
    </div>
  )
}
