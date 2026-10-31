import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Alert, Consumable, Device, InstalledSupply, Sample, StockEvent, Tray, User } from '../types'
import { decommissionedReason, deviceName, isDecommissioned, makeModel } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Bar, Btn, Card, DevicePill, Mk, MiniSpark, Pill, SevPill, Sw, Tabs, Tag, fmtInt, remaining, since, supplyColor, trayTone } from '../components/ui'
import { ConsumableLink, fmtDays, statusOf } from '../components/consumables'
import DeviceConfigTab from '../components/config/DeviceConfigTab'
import SupportCard from '../components/catalog/SupportCard'
import DeviceFirmwareTab from '../components/firmware/DeviceFirmwareTab'
import DeviceSecurityTab from '../components/security/DeviceSecurityTab'
import type { DeviceSecurity } from '../types-security'

const transitions: Record<string, string[]> = {
  discovered: ['onboarded', 'active', 'decommissioned'], onboarded: ['active', 'needs_attention', 'decommissioned'],
  active: ['needs_attention', 'decommissioned'], needs_attention: ['active', 'decommissioned'], decommissioned: ['onboarded'],
}
type Tab = 'overview' | 'config' | 'firmware' | 'security' | 'activity'

/** Route wrapper: keyed by the route id so every piece of device-specific state (record, alerts, counters, error, note, tab...) is discarded
 *  when navigating from one device to another, and the loading state shows until data for the new id arrives. */
export default function DeviceDetail({ user }: { user: User }) {
  const { id } = useParams()
  if (!id) return <Banner tone="crit">No device selected.</Banner>
  return <DeviceDetailBody key={id} id={id} user={user} />
}

function DeviceDetailBody({ id, user }: { id: string; user: User }) {
  const navigate = useNavigate()
  const [device, setDevice] = useState<Device | null>(null)
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [counters, setCounters] = useState<Sample[]>([])
  const [supplyHist, setSupplyHist] = useState<Record<string, Sample[]>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<{ tone: 'ok' | 'warn' | 'crit' | 'info'; text: string } | null>(null)
  const [tab, setTab] = useState<Tab>('overview')
  const [drifted, setDrifted] = useState<number | null>(null)
  const [secFailing, setSecFailing] = useState(0)
  const canOperate = user.role !== 'viewer'

  // Request generation: only responses belonging to the most recent load() of the mounted component are applied.
  const gen = useRef(0)
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false; gen.current++ } }, [])

  const load = useCallback(() => {
    const g = ++gen.current
    const cur = () => alive.current && g === gen.current
    api.get<{ device: Device; alerts: Alert[] }>(`/api/devices/${id}`).then((r) => { if (!cur()) return; setDevice(r.device); setAlerts(r.alerts); setError('') }).catch((e) => { if (cur()) setError(e.message) })
    api.get<DeviceSecurity>(`/api/devices/${id}/security`).then((r) => { if (cur()) setSecFailing(Math.max(0, ...r.assessments.map((a) => a.failed))) }).catch(() => {})
    api.get<Sample[]>(`/api/devices/${id}/telemetry?metric=counter.total&from=${new Date(Date.now() - 30 * 864e5).toISOString()}`).then((s) => { if (cur()) setCounters(s) }).catch(() => {})
    api.get<Sample[]>(`/api/devices/${id}/telemetry?metric=supply.percent&from=${new Date(Date.now() - 30 * 864e5).toISOString()}`).then((s) => {
      if (!cur()) return
      const m: Record<string, Sample[]> = {}
      for (const x of s) (m[(x.label || '').split(':')[0]] ||= []).push(x)
      setSupplyHist(m)
    }).catch(() => {})
  }, [id])
  useEffect(() => { load() }, [load])
  useEvents((e) => { const d = e.data as { id?: string; deviceId?: string }; if (d && (d.id === id || d.deviceId === id)) load() }, [id, load])

  // Actions target the device that is actually displayed (the loaded record), never the bare route parameter.
  const shownId = device?.id === id ? device.id : null
  const poll = async () => {
    if (!shownId) return
    setBusy(true); setNote(null)
    try {
      const r = await api.post<{ device: Device; servedBy: Record<string, string>; errors: Record<string, string>; reachable: boolean }>(`/api/devices/${shownId}/poll`)
      if (!alive.current) return
      setDevice(r.device)
      const errs = Object.entries(r.errors || {})
      const by = Object.values(r.servedBy || {}).filter((v, i, a) => a.indexOf(v) === i).join(', ')
      setNote(r.reachable ? { tone: errs.length ? 'warn' : 'ok', text: `Polled via ${by}${errs.length ? ` · ${errs.map(([k, v]) => `${k}: ${v}`).join('; ')}` : ''}` } : { tone: 'crit', text: `Unreachable: ${errs.map(([, v]) => v).join('; ')}` })
      load()
    } catch (e) { if (alive.current) setNote({ tone: 'crit', text: (e as Error).message }) } finally { if (alive.current) setBusy(false) }
  }
  const setState = async (state: string) => {
    if (!shownId) return
    try { const nd = await api.post<Device>(`/api/devices/${shownId}/state`, { state }); if (alive.current) setDevice(nd) } catch (e) { if (alive.current) setNote({ tone: 'crit', text: (e as Error).message }) }
  }
  const ack = async (a: Alert) => { await api.post(`/api/alerts/${a.id}/ack`); if (alive.current) load() }

  if (error && !device) return <Banner tone="crit">{error}</Banner>
  if (!device) return <div className="cap" aria-busy="true">Loading…</div>
  const d = device
  const inputs = d.trays.filter((t) => t.kind === 'input')
  const topAlert = alerts.find((a) => a.severity === 'critical' && !a.ackedAt) || alerts.find((a) => a.severity === 'critical')
  const typeTag = [d.deviceType === 'mfp' ? 'MFP' : 'Printer', d.supplies.some((s) => /cyan|magenta|yellow/i.test(`${s.colorant} ${s.description}`)) ? 'Colour' : 'Mono'].join(' · ')

  return (
    <>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <nav aria-label="Breadcrumb" className="cap"><Link to="/devices">Devices</Link> / <span style={{ color: 'var(--text-2)' }}>{deviceName(d)}</span></nav>
        <Btn sm variant="ghost" onClick={() => navigate(-1)}><Ic.chevronLeft />Back</Btn>
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10 }}><h1 className="h1">{deviceName(d)}</h1><DevicePill d={d} /><Tag>{typeTag}</Tag><Tag>{d.state.replace('_', ' ')}</Tag></div>
          <div className="cap" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 14px', color: 'var(--text-2)' }}>
            <span className="mono">{makeModel(d)}</span><span className="mono">{d.address}</span>{d.hostname && <span className="mono">{d.hostname}</span>}{d.serial && <span>SN <span className="mono">{d.serial}</span></span>}{d.location && <span>{d.location}</span>}<span>Polled {ago(d.lastPolled)}</span>
          </div>
        </div>
        {canOperate && (
          <div className="row" role="toolbar" aria-label="Device actions">
            <Btn sm onClick={poll} disabled={busy || !shownId || isDecommissioned(d)} title={isDecommissioned(d) ? decommissionedReason : undefined}><Ic.refresh />{busy ? 'Polling…' : 'Poll now'}</Btn>
            <a className="btn sm" href="#identity"><Ic.pencil />Edit</a>
            {d.webUrl && !isDecommissioned(d) && <a className="btn sm" href={d.webUrl} target="_blank" rel="noopener noreferrer"><Ic.external />Device web page</a>}
            <span className="vsep" />
            <label className="chip"><select aria-label="Change lifecycle state" value="" disabled={!shownId} onChange={(e) => e.target.value && setState(e.target.value)}><option value="">Change state…</option>{(transitions[d.state] || []).map((s) => <option key={s} value={s}>{s.replace('_', ' ')}</option>)}</select></label>
          </div>
        )}
      </div>
      {isDecommissioned(d) && <Banner tone="info"><b>This printer is decommissioned, and PrintMarshal does not contact it.</b> Everything here is the last stored data{d.lastPolled ? <>, polled {ago(d.lastPolled)}</> : ''}. Reads and writes stay off until its lifecycle state changes.</Banner>}
      {error && <Banner tone="crit">{error}</Banner>}
      {note && <Banner tone={note.tone}>{note.text}</Banner>}
      {topAlert && (
        <Banner tone="crit" role="alert" right={<>{canOperate && !topAlert.ackedAt && <Btn sm onClick={() => ack(topAlert)}>Acknowledge</Btn>}<Link className="btn sm ghost" to="/alerts">Open alerts</Link></>}>
          <b style={{ color: 'var(--crit)' }}>{topAlert.description}</b> — since {ago(topAlert.raisedAt)}{d.status?.message && <> · Panel: <span className="mono">{d.status.message}</span></>}
        </Banner>
      )}
      <Tabs label="Device sections" value={tab} onChange={setTab} tabs={[
        { v: 'overview', label: 'Overview' },
        { v: 'config', label: <>Configuration{drifted !== null && drifted > 0 && <span className="cnt warn">{drifted} drifted</span>}</> },
        { v: 'firmware', label: 'Firmware' },
        { v: 'security', label: <>Security{secFailing > 0 && <span className="cnt crit">{secFailing} failing</span>}</> },
        { v: 'activity', label: 'Activity' },
      ]} />
      <DeviceConfigTab device={d} canOperate={canOperate} hidden={tab !== 'config'} onDrift={setDrifted} />

      {tab === 'overview' && (
        <div className="cols">
          <div className="stack" style={{ flex: '2 1 620px' }}>
            <SuppliesCard d={d} canOperate={canOperate} hist={supplyHist} />
            <Card title="Counters" id="ctr" sub="lifetime · trend from polled totals">
              <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 12 }}>
                  <div><div className="cap">Total impressions</div><div className="num" style={{ fontSize: 20, fontWeight: 600 }}>{d.counters ? fmtInt(d.counters.total) : '—'}</div>{counters.length > 1 && <div className="cap num">+{fmtInt(counters[counters.length - 1].value - counters[0].value)} in {Math.max(1, Math.round((new Date(counters[counters.length - 1].at).getTime() - new Date(counters[0].at).getTime()) / 864e5))} days</div>}</div>
                  <div><div className="cap">Mono</div><div className="num" style={{ fontSize: 20, fontWeight: 600 }}>{d.counters?.mono !== undefined ? fmtInt(d.counters.mono) : <span className="cap">not reported</span>}</div></div>
                  <div><div className="cap">Colour</div><div className="num" style={{ fontSize: 20, fontWeight: 600 }}>{d.counters?.color !== undefined ? fmtInt(d.counters.color) : <span className="cap">not reported</span>}</div></div>
                  <div><div className="cap">Since power-on</div><div className="num" style={{ fontSize: 20, fontWeight: 600 }}>{d.counters?.powerOn !== undefined ? fmtInt(d.counters.powerOn) : '—'}</div><div className="cap">uptime {d.status?.uptimeSecs ? since(new Date(Date.now() - d.status.uptimeSecs * 1000).toISOString()) : '—'}</div></div>
                </div>
                <DailyChart samples={counters} />
              </div>
            </Card>
            <Card title="Trays & media" id="tr" sub={`${inputs.length} input${inputs.length === 1 ? '' : 's'}`}>
              {d.trays.length === 0 ? <div className="cb cap">No tray data.</div> : (
                <div className="cb" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 10 }}>
                  {d.trays.map((t) => <TrayCard key={`${t.kind}-${t.index}`} t={t} />)}
                </div>
              )}
            </Card>
          </div>
          <div className="stack" style={{ flex: '1 1 340px' }}>
            <Card title="Status & alerts" id="sa" right={<Link to="/alerts" className="cap" style={{ color: 'var(--accent)' }}>All alerts →</Link>}>
              <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {d.status?.message && <div style={{ border: '1px solid var(--border-strong)', borderRadius: 6, padding: '10px 12px', background: 'var(--surface-2)' }}><div className="cap">Control panel message</div><div className="mono" style={{ fontSize: 13, marginTop: 2 }}>{d.status.message}</div></div>}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px 12px' }}>
                  <div><div className="cap">Device state</div><div>{d.status ? `${d.status.state}${d.status.severity !== 'ok' ? ' · ' + d.status.severity : ''}` : '—'}</div></div>
                  <div><div className="cap">Uptime</div><div className="num">{d.status?.uptimeSecs ? `${Math.floor(d.status.uptimeSecs / 86400)} d ${Math.floor((d.status.uptimeSecs % 86400) / 3600)} h` : '—'}</div></div>
                  <div><div className="cap">Reachability</div><div>{d.identifyMethods.map((m) => m.toUpperCase()).join(' · ') || '—'}</div>
                    {Object.entries(d.identifyErrors || {}).map(([drv, why]) => <div key={drv} className="cap" style={{ color: 'var(--warn)', marginTop: 2, whiteSpace: 'normal' }} title={why}>{drv.toUpperCase()} did not answer on the last scan: {why}</div>)}
                  </div>
                  <div><div className="cap">Error flags</div><div>{d.status?.errors?.length ? d.status.errors.join(', ') : 'none'}</div></div>
                </div>
              </div>
              <div style={{ borderTop: '1px solid var(--border)' }}>
                {alerts.length === 0 && (d.status?.alerts?.length ?? 0) === 0 && <div className="lrow cap">No active alerts.</div>}
                {alerts.map((a) => <div className="lrow" key={a.id}><SevPill sev={a.severity} /><div style={{ flex: 1, minWidth: 0 }}>{a.description}<div className="cap">{a.source} · {ago(a.raisedAt)}{a.ackedBy ? ` · acked by ${a.ackedBy}` : ''}</div></div>{canOperate && !a.ackedAt && <Btn sm onClick={() => ack(a)}>Ack</Btn>}</div>)}
                {d.status?.alerts?.map((a) => <div className="lrow" key={`dev-${a.index}`}><SevPill sev={a.severity} /><div style={{ flex: 1, minWidth: 0 }}>{a.description}<div className="cap">device-reported · {a.group}{a.code ? ` · ${a.code}` : ''}</div></div></div>)}
              </div>
            </Card>
            <IdentityCard d={d} canEdit={canOperate} onSaved={setDevice} />
            <SupportCard deviceId={d.id} lastPolled={d.lastPolled} />
          </div>
        </div>
      )}
      {tab === 'firmware' && <DeviceFirmwareTab device={d} canOperate={canOperate} />}
      {tab === 'security' && <DeviceSecurityTab device={d} canOperate={canOperate} />}
      {tab === 'activity' && <ActivityCard deviceId={d.id} canSee={canOperate} />}
    </>
  )
}

function TrayCard({ t }: { t: Tray }) {
  const tone = trayTone(t)
  const col = tone === 'ok' ? 'var(--ok)' : tone === 'warn' ? 'var(--warn)' : tone === 'crit' ? 'var(--crit)' : 'var(--track)'
  const pct = t.percent ?? (t.status === 'ok' && t.level === -3 ? 60 : t.status === 'empty' ? 0 : 0)
  return (
    <div className="tray">
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 6 }}><b style={{ fontWeight: 600 }}>{t.name}</b><Pill tone={tone} xs>{t.status === 'ok' ? 'OK' : t.status.charAt(0).toUpperCase() + t.status.slice(1)}</Pill></div>
      <div className="cap">{[t.kind === 'output' ? 'output' : null, t.mediaSize, t.mediaName, t.mediaType].filter(Boolean).join(' · ') || t.type || '—'}</div>
      <Bar pct={pct} color={col} />
    </div>
  )
}

function DailyChart({ samples }: { samples: Sample[] }) {
  const days = useMemo(() => {
    if (samples.length < 2) return []
    const byDay = new Map<string, number>()
    for (const s of samples) byDay.set(s.at.slice(0, 10), s.value)
    const keys = Array.from(byDay.keys()).sort()
    const out: { day: string; pages: number }[] = []
    for (let i = 1; i < keys.length; i++) out.push({ day: keys[i], pages: Math.max(0, byDay.get(keys[i])! - byDay.get(keys[i - 1])!) })
    return out
  }, [samples])
  if (days.length < 2) return <div className="cap">Daily page trend appears after a couple of days of polling history.</div>
  const max = Math.max(1, ...days.map((d) => d.pages))
  const w = 640, h = 140, x0 = 36
  const pts = days.map((d, i) => `${x0 + (i / (days.length - 1)) * (w - x0 - 4)},${120 - (d.pages / max) * 110}`).join(' ')
  return (
    <>
      <div className="cap" style={{ display: 'flex', gap: 16 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 14, height: 3, background: 'var(--cat1)', borderRadius: 2 }} />Pages / day</span></div>
      <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label={`Pages per day over the last ${days.length} days, peak ${max}`}>
        <g style={{ stroke: 'var(--border)' }}><line x1={x0} y1="10" x2={w} y2="10" /><line x1={x0} y1="65" x2={w} y2="65" /><line x1={x0} y1="120" x2={w} y2="120" /></g>
        <g style={{ fill: 'var(--text-3)' }} fontSize="10" fontFamily="IBM Plex Sans, sans-serif"><text x="0" y="14">{fmtInt(max)}</text><text x="0" y="69">{fmtInt(Math.round(max / 2))}</text><text x="8" y="124">0</text><text x={x0} y="138">{days[0].day.slice(5)}</text><text x={w - 40} y="138">{days[days.length - 1].day.slice(5)}</text></g>
        <polyline fill="none" style={{ stroke: 'var(--cat1)' }} strokeWidth="2" points={pts} />
      </svg>
    </>
  )
}

/** Supplies card: installed levels linked to stock items, with "mark replaced" (consumables feature). */
function SuppliesCard({ d, canOperate, hist }: { d: Device; canOperate: boolean; hist: Record<string, Sample[]> }) {
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
    try { if (consumableId) await api.put(`/api/devices/${d.id}/supplies/${r.supply.index}/link`, { consumableId }); else await api.del(`/api/devices/${d.id}/supplies/${r.supply.index}/link`); load() } catch (e) { setMsg((e as Error).message) }
  }
  const act = async (ev: StockEvent, what: 'confirm' | 'dismiss') => { try { await api.post(`/api/consumables/replacements/${ev.id}/${what}`); load() } catch (e) { setMsg((e as Error).message) } }
  const list = rows ?? d.supplies.map((s) => ({ deviceId: d.id, deviceName: deviceName(d), deviceModel: d.model || '', address: d.address, supply: s } as InstalledSupply))
  return (
    <Card title="Supplies" id="sup" sub="tick = low threshold (15%) · sparkline = last 30 days · right: spares on hand">
      {pending.map((ev) => <div className="banner accent" key={ev.id} style={{ borderRadius: 0, borderLeft: 0, borderRight: 0 }}><Ic.info style={{ color: 'var(--accent)' }} /><div style={{ flex: 1 }}>Replacement detected: {ev.reason}.</div>{canOperate && <><Btn sm variant="pri" onClick={() => act(ev, 'confirm')}>Take 1 from stock</Btn><Btn sm variant="ghost" onClick={() => act(ev, 'dismiss')}>Dismiss</Btn></>}</div>)}
      {rows === null && <div className="cb cap" aria-busy="true">Loading…</div>}
      {rows !== null && list.length === 0 && <div className="cb cap">No supply data.</div>}
      <div className="tw"><div style={{ minWidth: 640 }}>
        {list.map((r) => {
          const s = r.supply
          const rem = remaining(s)
          const col = supplyColor(s)
          const low = rem !== null && rem <= 15
          const empty = rem === 0
          const label = s.description || `${s.colorant || ''} ${s.type}`.trim()
          return (
            <div className={`srow${empty ? ' crit' : ''}`} key={s.index}>
              <Sw color={col} />
              <div style={{ minWidth: 0 }}><div style={{ fontWeight: 500, color: empty ? 'var(--crit)' : undefined }}>{label}</div><div className="cap">{s.class === 'filled' ? 'fills up' : s.type}{r.forecast?.daysToEmpty != null ? ` · ~${fmtDays(r.forecast.daysToEmpty).replace('~', '')} left` : ''}{s.percent === null && s.level === -3 ? ' · some remaining' : ''}</div></div>
              <Bar lg pct={rem} color={col} tick={s.class === 'filled' ? 90 : 15} empty={empty} />
              <b className="num" style={{ textAlign: 'right', fontWeight: low ? 600 : 500, color: empty ? 'var(--crit)' : low ? 'var(--warn)' : undefined }}>{s.percent === null ? (s.level === -3 ? 'some' : '?') : `${s.percent}%`}</b>
              <MiniSpark samples={hist[String(s.index)] || []} color={col} />
              <div className="stk">
                {r.consumable ? (<>
                  <span className="num" style={{ color: statusOf(r.consumable) !== 'ok' ? 'var(--warn)' : undefined, fontWeight: statusOf(r.consumable) !== 'ok' ? 600 : 400 }}><ConsumableLink c={r.consumable} /> · {r.consumable.onHand} in stock{statusOf(r.consumable) !== 'ok' ? ` · ${statusOf(r.consumable)}` : ''}</span>
                  {canOperate && <button type="button" onClick={() => markReplaced(r)} title="Record that this supply was replaced and optionally decrement stock">Mark replaced</button>}
                </>) : (<>
                  <span className="cap">not stocked</span>
                  {canOperate && <Link to="/consumables?tab=unmatched" className="cap">Add to stock</Link>}
                </>)}
                {canOperate && items.length > 0 && (
                  <select className="fin" style={{ height: 22, fontSize: 11, padding: '0 4px', width: 'auto' }} value={r.linkSource === 'manual' ? r.consumable?.id || '' : ''} onChange={(e) => link(r, e.target.value)} aria-label="Link to stock item">
                    <option value="">{r.consumable ? 'auto-linked · change…' : 'link to…'}</option>
                    {items.map((i) => <option key={i.id} value={i.id}>{i.name}{i.color ? ` (${i.color})` : ''}</option>)}
                    {r.linkSource === 'manual' && <option value="">remove manual link</option>}
                  </select>
                )}
              </div>
            </div>
          )
        })}
      </div></div>
      {rows !== null && list.some((r) => r.consumable) && <div className="cf cap" style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'var(--surface-2)' }}><Ic.consumables style={{ color: 'var(--accent)' }} /><span style={{ flex: 1 }}><b style={{ fontWeight: 600, color: 'var(--text)' }}>Mark replaced</b> logs the swap on this device and offers to take 1 from stock.</span><Link to="/consumables">Consumables →</Link></div>}
      {msg && <div className="cf cap">{msg}</div>}
    </Card>
  )
}

function IdentityCard({ d, canEdit, onSaved }: { d: Device; canEdit: boolean; onSaved: (d: Device) => void }) {
  const [form, setForm] = useState({ displayName: d.displayName, dnsSync: !!d.dnsSync, location: d.location, assetTag: d.assetTag, notes: d.notes, webUrl: d.webUrlOverride || '' })
  const [rdnsOff, setRdnsOff] = useState(false)
  useEffect(() => { api.get<{ runtime?: { reverseDns?: { enabled?: boolean } } }>('/api/settings').then((st) => setRdnsOff(st.runtime?.reverseDns?.enabled === false)).catch(() => {}) }, [])
  const [msg, setMsg] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const [usingDNS, setUsingDNS] = useState(false)
  const [groups, setGroups] = useState<{ id: string; name: string }[]>([])
  useEffect(() => setForm({ displayName: d.displayName, dnsSync: !!d.dnsSync, location: d.location, assetTag: d.assetTag, notes: d.notes, webUrl: d.webUrlOverride || '' }), [d.id, d.displayName, d.dnsSync, d.location, d.assetTag, d.notes, d.webUrlOverride])
  useEffect(() => { api.get<{ id: string; name: string; memberIds: string[] }[]>('/api/groups').then((gs) => setGroups(gs.filter((g) => g.memberIds.includes(d.id)))).catch(() => {}) }, [d.id])
  const dirty = form.displayName !== d.displayName || form.dnsSync !== !!d.dnsSync || form.location !== d.location || form.assetTag !== d.assetTag || form.notes !== d.notes || form.webUrl !== (d.webUrlOverride || '')
  const save = async (e: React.FormEvent) => { e.preventDefault(); try { onSaved(await api.patch<Device>(`/api/devices/${d.id}`, form)); setMsg('Saved.') } catch (err) { setMsg((err as Error).message) } }
  const refreshHostname = async () => {
    setRefreshing(true); setMsg('')
    try { onSaved(await api.post<Device>(`/api/devices/${d.id}/refresh-hostname`)) } catch (err) { setMsg((err as Error).message) } finally { setRefreshing(false) }
  }
  const useDNSName = async () => {
    setUsingDNS(true); setMsg('')
    try {
      await api.post('/api/devices/rename', { deviceIds: [d.id], source: 'dns', dryRun: false })
      const r = await api.get<{ device: Device }>(`/api/devices/${d.id}`)
      onSaved(r.device)
      setMsg('Display name updated from DNS.')
    } catch (err) { setMsg((err as Error).message) } finally { setUsingDNS(false) }
  }
  const Row = ({ label, children, op }: { label: string; children: React.ReactNode; op?: boolean }) => <div className="frow"><span className="cap">{label}</span><span style={{ minWidth: 0, overflowWrap: 'anywhere' }}>{children}</span>{op === undefined ? <span /> : <Mk op={op} />}</div>
  return (
    <form className="card" id="identity" onSubmit={save} aria-labelledby="ida">
      <div className="ch"><h2 className="h2" id="ida">Identity & asset</h2>{canEdit && <Btn type="submit" sm variant={dirty ? 'pri' : undefined} disabled={!dirty}>Save</Btn>}</div>
      <div className="cap" style={{ display: 'flex', flexWrap: 'wrap', gap: 12, padding: '10px 16px', borderBottom: '1px solid var(--border)' }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Mk op />Set by operator — never overwritten</span><span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Mk op={false} />Discovered from device</span>
      </div>
      <div style={{ padding: '6px 0' }}>
        <div className="frow">
          <label className="cap" htmlFor="f-name">Display name</label>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <input className="fin" id="f-name" value={form.displayName} disabled={!canEdit} placeholder={d.sysName || d.hostname || d.model} onChange={(e) => setForm({ ...form, displayName: e.target.value })} />
            {d.displayNameSource && <span className="cap" style={{ fontSize: 11 }}>Set from {d.displayNameSource}</span>}
            {d.dnsNameChanged && (
              <div style={{ display: 'inline-flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
                <Pill tone="warn" xs>DNS name changed</Pill>
                {canEdit && d.dnsName && !d.dnsNameSuspect && <Btn sm variant="ghost" onClick={useDNSName} disabled={usingDNS} style={{ height: 22, padding: '0 6px', fontSize: 11 }}>{usingDNS ? 'Updating…' : 'Use DNS name'}</Btn>}
              </div>
            )}
          </div>
          <Mk op />
        </div>
        <div className="frow">
          <span className="cap">DNS sync</span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
              <input className="cbx" type="checkbox" checked={form.dnsSync} disabled={!canEdit} onChange={(e) => setForm({ ...form, dnsSync: e.target.checked })} />
              Keep display name synced with DNS name
            </label>
            {rdnsOff && <span className="cap" style={{ fontSize: 11 }}>Reverse DNS is off, so names are looked up only when you press Refresh. {canEdit ? <Link to="/settings">Turn it on in Settings</Link> : 'An administrator can turn it on in Settings'}.</span>}
          </div>
          <Mk op />
        </div>
        <div className="frow"><label className="cap" htmlFor="f-loc">Location</label><input className="fin" id="f-loc" value={form.location} disabled={!canEdit} onChange={(e) => setForm({ ...form, location: e.target.value })} /><Mk op /></div>
        <div className="frow"><label className="cap" htmlFor="f-web">Web page</label><input className="fin mono" id="f-web" type="url" value={form.webUrl} disabled={!canEdit} placeholder={d.webUrl ? `${d.webUrl} (detected)` : 'https://… (none detected)'} onChange={(e) => setForm({ ...form, webUrl: e.target.value })} /><Mk op /></div>
        <div className="frow"><label className="cap" htmlFor="f-tag">Asset tag</label><input className="fin mono" id="f-tag" value={form.assetTag} disabled={!canEdit} onChange={(e) => setForm({ ...form, assetTag: e.target.value })} /><Mk op /></div>
        <div className="frow" style={{ alignItems: 'start' }}><label className="cap" htmlFor="f-notes" style={{ paddingTop: 6 }}>Notes</label><textarea className="fin" id="f-notes" rows={2} value={form.notes} disabled={!canEdit} onChange={(e) => setForm({ ...form, notes: e.target.value })} /><Mk op /></div>
        {msg && <div className="cap" style={{ margin: '0 16px 4px 146px' }}>{msg}</div>}
        <div style={{ height: 1, background: 'var(--border)', margin: '6px 16px' }} />
        <Row label="Make / model" op={false}><span className="mono">{makeModel(d)}</span></Row>
        <Row label="Serial" op={false}><span className="mono">{d.serial || '—'}</span></Row>
        <Row label="MAC" op={false}><span className="mono">{d.mac || '—'}</span></Row>
        <Row label="UUID" op={false}><span className="mono" style={{ fontSize: 11 }}>{d.uuid || '—'}</span></Row>
        <Row label="Firmware" op={false}><span className="mono">{d.firmware || '—'}</span></Row>
        <Row label="Hostname" op={false}><span className="mono">{d.hostname || '—'}</span> <span className="cap">· mDNS / advertised</span></Row>
        <Row label="sysName" op={false}><span className="mono">{d.sysName || '—'}</span> <span className="cap">· advertised</span></Row>
        <Row label="DNS name" op={false}>
          <span style={{ display: 'inline-flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
            <span className="mono">{d.dnsName || '—'}</span>
            {d.dnsName && d.dnsNameSuspect && <Pill tone="warn" xs title="Invalid characters, over length, or auto-generated from IP">Suspect</Pill>}
            {d.dnsCheckedAt && <span className="cap">· checked {ago(d.dnsCheckedAt)}</span>}
            {canEdit && d.dnsName && !d.dnsNameSuspect && d.displayName !== d.dnsName && (
              <Btn sm variant="ghost" onClick={useDNSName} disabled={usingDNS} style={{ height: 22, padding: '0 6px', fontSize: 11 }}>
                {usingDNS ? 'Applying…' : 'Use DNS name'}
              </Btn>
            )}
            {canEdit && <Btn sm variant="ghost" onClick={refreshHostname} disabled={refreshing} style={{ height: 22, padding: '0 6px', fontSize: 11 }}>{refreshing ? 'Refreshing…' : 'Refresh'}</Btn>}
          </span>
        </Row>
        <Row label="sysDescr" op={false}><span className="cap" style={{ fontSize: 11 }}>{d.sysDescr || '—'}</span></Row>
        <Row label="Enterprise" op={false}><span className="num">{d.enterpriseId || '—'}</span></Row>
        <Row label="Found by" op={false}>{d.discoveryMethods.map((m) => <Tag key={m}>{m}</Tag>)}</Row>
        <Row label="Capabilities" op={false}><span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{Object.entries(d.capabilities).map(([c, by]) => <Tag key={c} title={`served by ${by}`}>{c.replace('_', ' ')} <span className="cap">· {by}</span></Tag>)}</span></Row>
        <Row label="Groups">{groups.length ? <span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{groups.map((g) => <Link key={g.id} to={`/devices?group=${g.id}`} className="tag">{g.name}</Link>)}</span> : <span className="cap">none</span>}</Row>
        <Row label="First seen">{fmtTime(d.firstSeen)}</Row>
        <Row label="Last seen">{fmtTime(d.lastSeen)} <span className="cap">({ago(d.lastSeen)})</span></Row>
      </div>
    </form>
  )
}

function ActivityCard({ deviceId, canSee }: { deviceId: string; canSee: boolean }) {
  const [entries, setEntries] = useState<{ id: number; actor: string; action: string; target: string; at: string; result: string; after?: Record<string, unknown> }[]>([])
  const [jobs, setJobs] = useState<{ id: number; jobId: string; status: string; message?: string; at: string }[]>([])
  useEffect(() => {
    if (canSee) api.get<typeof entries>('/api/audit?limit=500').then((es) => setEntries(es.filter((e) => e.target === deviceId))).catch(() => {})
    api.get<{ id: string; type: string }[]>('/api/jobs?limit=50').then(() => {}).catch(() => {})
    setJobs([])
  }, [deviceId, canSee])
  return (
    <Card title="Activity for this device" id="actv" sub="operator changes recorded in the audit trail">
      {!canSee ? <div className="cb cap">The audit trail needs the operator role.</div> : entries.length === 0 && jobs.length === 0 ? <div className="cb cap">No recorded changes yet.</div> : (
        <div className="tw"><table className="tbl" style={{ minWidth: 640 }}><thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Outcome</th></tr></thead><tbody>
          {entries.map((e) => <tr key={e.id}><td className="num cap">{fmtTime(e.at)}</td><td>{e.actor}</td><td>{e.action}{e.after ? <span className="cap"> · {JSON.stringify(e.after).slice(0, 80)}</span> : null}</td><td><Pill tone={e.result === 'ok' ? 'ok' : 'warn'} xs>{e.result}</Pill></td></tr>)}
        </tbody></table></div>
      )}
    </Card>
  )
}
