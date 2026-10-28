import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { AuditEntry, Dashboard as DashboardData, InstalledSupply, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Av, Banner, Bar, Btn, Card, DevicePill, EmptyState, JobBar, JobPill, Kpi, MultiBar, Pill, SevPill, Sw, Tag, fmtInt, remaining, since, supplyColor } from '../components/ui'
import { StockPill, fmtDays, swatchFor } from '../components/consumables'

export default function Dashboard({ user }: { user: User }) {
  const [data, setData] = useState<DashboardData | null>(null)
  const [low, setLow] = useState<InstalledSupply[]>([])
  const [audit, setAudit] = useState<AuditEntry[]>([])
  const [error, setError] = useState('')
  const [dismissed, setDismissed] = useState(false)
  const [tick, setTick] = useState(0)
  const load = useCallback(() => {
    api.get<DashboardData>('/api/dashboard').then(setData).catch((e) => setError(e.message))
    api.get<InstalledSupply[]>('/api/consumables/installed?forecast=true').then((r) => setLow(r.filter((s) => remaining(s.supply) !== null && (remaining(s.supply) as number) <= 20).sort((a, b) => (remaining(a.supply) ?? 0) - (remaining(b.supply) ?? 0)).slice(0, 6))).catch(() => {})
    if (user.role !== 'viewer') api.get<AuditEntry[]>('/api/audit?limit=6').then(setAudit).catch(() => {})
  }, [user.role])
  useEffect(() => { load() }, [load])
  useEffect(() => { const t = setInterval(() => setTick((n) => n + 1), 30000); return () => clearInterval(t) }, [])
  useEffect(() => { if (tick) load() }, [tick, load])
  useEvents(() => load(), [load])

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!data) return <DashboardSkeleton />
  const s = data.summary
  const bySev = s.bySeverity
  const healthy = (bySev.ok || 0), warn = (bySev.warning || 0), crit = (bySev.critical || 0), off = s.offline
  const total = Math.max(1, s.devices)
  const pct = (n: number) => (n / total) * 100
  const newCrit = data.alerts.filter((a) => a.severity === 'critical' && !a.ackedAt)
  const lastPoll = data.jobs.find((j) => j.type === 'poll' && j.finishedAt)

  return (
    <>
      {newCrit.length > 0 && !dismissed && (
        <Banner tone="crit" role="alert" right={<><Link className="btn sm" to="/alerts">View alerts</Link><Btn sm variant="ghost" ic aria-label="Dismiss banner" onClick={() => setDismissed(true)}><Ic.x /></Btn></>}>
          <b style={{ color: 'var(--crit)' }}>{newCrit.length} unacknowledged critical alert{newCrit.length === 1 ? '' : 's'}</b> — {newCrit.slice(0, 2).map((a) => a.description).join(' · ')}{newCrit.length > 2 ? ' · …' : ''}
        </Banner>
      )}
      <div className="ph">
        <div><h1 className="h1">Fleet overview</h1><div className="cap num">{fmtInt(s.devices)} devices{lastPoll ? ` · last full poll ${ago(lastPoll.finishedAt)}` : ''}</div></div>
        <div className="row">
          <span className="cap num" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Ic.clock />Auto-refresh every 30 s</span>
          <Btn sm onClick={load}><Ic.refresh />Refresh</Btn>
        </div>
      </div>
      <div className="grid-auto">
        <Kpi label="Total devices" value={fmtInt(s.devices)} to="/devices" detail={s.byState.discovered ? `${s.byState.discovered} discovered, not onboarded` : 'all onboarded'} />
        <Kpi label="Online / offline" value={<>{fmtInt(s.devices - off)} <span style={{ fontSize: 15, color: 'var(--text-3)', fontWeight: 500 }}>/ {off}</span></>} to="/devices?severity=offline" detail={<><span className="dot" style={{ background: off === 0 ? 'var(--ok)' : 'var(--warn)' }} />{s.devices ? `${(((s.devices - off) / s.devices) * 100).toFixed(1)}% reachable` : 'no devices'}</>} />
        <Kpi label="Critical · warning" value={<><span style={{ color: 'var(--crit)' }}>{crit}</span> <span style={{ fontSize: 15, color: 'var(--text-3)', fontWeight: 500 }}>·</span> <span style={{ color: 'var(--warn)' }}>{warn}</span></>} to="/devices?sort=severity" detail={`${s.activeAlerts} active alerts`} />
        <Kpi label="Low / empty supplies" value={fmtInt(s.lowSupplies)} to="/consumables?tab=fleet" detail="installed levels at or below threshold" />
        <Kpi label="Consumables low on stock" value={fmtInt(s.lowStock || 0)} color={s.lowStock ? 'var(--warn)' : undefined} to="/consumables?status=low" detail="spares on the shelf" />
        <Kpi label="Jobs running" value={fmtInt(s.runningJobs)} to="/jobs" detail={data.jobs.some((j) => j.failed > 0 && j.status === 'running') ? 'some with failures' : 'discovery, polls'} />
      </div>

      <Card title="Fleet health" id="fh" right={<span className="cap">by device state · updated live</span>}>
        <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <MultiBar height={14} gap parts={[{ pct: pct(healthy), color: 'var(--ok)' }, { pct: pct(warn), color: 'var(--warn)' }, { pct: pct(crit), color: 'var(--crit)' }, { pct: pct(off), color: 'var(--off)' }]} label={`Healthy ${healthy}, warning ${warn}, critical ${crit}, offline ${off}`} />
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px 24px' }}>
            <Pill tone="ok">Healthy <b className="num">{healthy}</b></Pill><Pill tone="warn">Warning <b className="num">{warn}</b></Pill><Pill tone="crit">Critical <b className="num">{crit}</b></Pill><Pill tone="off">Offline <b className="num">{off}</b></Pill>
            <span className="cap" style={{ marginLeft: 'auto' }}>{Object.entries(s.byState).map(([k, v]) => `${v} ${k.replace('_', ' ')}`).join(' · ')}</span>
          </div>
        </div>
      </Card>

      <div className="grid-2">
        <Card title="Recent alerts" id="ra" right={<Link to="/alerts" className="cap" style={{ color: 'var(--accent)' }}>All alerts →</Link>}>
          {data.alerts.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title="All clear" text={`No active alerts across ${s.devices} devices.`} /></div> : (
            <div className="tw"><table className="tbl">
              <thead><tr><th>Severity</th><th>Device</th><th>Alert</th><th>Source</th><th className="r">Last seen</th></tr></thead>
              <tbody>{data.alerts.slice(0, 8).map((a) => {
                const dev = data.attention.find((d) => d.id === a.deviceId)
                return (
                  <tr key={a.id}><td><SevPill sev={a.severity} /></td>
                    <td>{a.deviceId ? <Link to={`/devices/${a.deviceId}`}>{dev ? deviceName(dev) : 'device'}</Link> : <Link to={a.source === 'consumable' ? '/consumables' : '/alerts'}>stock</Link>}</td>
                    <td className="wrap">{a.description}</td><td><Tag>{a.source}</Tag></td><td className="num cap r">{ago(a.updatedAt || a.raisedAt)}</td></tr>
                )
              })}</tbody>
            </table></div>
          )}
        </Card>
        <Card title="Devices needing attention" id="na" right={<Link to="/devices?sort=severity" className="cap" style={{ color: 'var(--accent)' }}>Open in inventory →</Link>}>
          {data.attention.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title="Nothing needs attention" text="All polled devices are healthy." /></div> : (
            <div className="tw"><table className="tbl">
              <thead><tr><th>Status</th><th>Device</th><th>Model</th><th>Location</th><th className="r">Since</th></tr></thead>
              <tbody>{data.attention.map((d) => (
                <tr key={d.id}><td><DevicePill d={d} /></td><td><Link to={`/devices/${d.id}`}>{deviceName(d)}</Link></td><td className="mono">{d.model}</td><td className="cap">{d.location || d.status?.message || '—'}</td><td className="num cap r">{since(d.status?.at)}</td></tr>
              ))}</tbody>
            </table></div>
          )}
        </Card>
      </div>

      <div className="grid-3">
        <Card title="Supplies running low" id="sl" right={<span className="cap">installed levels · forecast from usage</span>}>
          {low.length === 0 ? <div className="cb cap">No installed supply is at or below 20%.</div> : low.map((r) => {
            const p = remaining(r.supply) ?? 0
            return (
              <div className="lrow" key={`${r.deviceId}:${r.supply.index}`}>
                <Sw color={supplyColor(r.supply)} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                    <span><Link to={`/devices/${r.deviceId}`}>{r.deviceName}</Link> · {r.supply.colorant || r.supply.description}</span>
                    <span className="num" style={{ color: p === 0 ? 'var(--crit)' : p <= 10 ? 'var(--warn)' : undefined, fontWeight: p <= 10 ? 600 : 400 }}>{p === 0 ? 'Empty' : `${p}%`}{r.forecast?.daysToEmpty != null && <span className="cap"> · {fmtDays(r.forecast.daysToEmpty)}</span>}</span>
                  </div>
                  <Bar pct={p} color={supplyColor(r.supply)} />
                </div>
              </div>
            )
          })}
        </Card>
        <Card title="In-progress jobs" id="jp" right={<Link to="/jobs" className="cap" style={{ color: 'var(--accent)' }}>Jobs →</Link>}>
          {data.jobs.length === 0 ? <div className="cb"><EmptyState icon={<Ic.radar size={20} />} title="No jobs yet" text="Find printers on your network to start managing them." action={<Link className="btn sm pri" to="/discovery">Run discovery</Link>} /></div> : data.jobs.slice(0, 5).map((j) => (
            <div className="lrow" key={j.id} style={{ flexDirection: 'column', alignItems: 'stretch', gap: 6 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}><Link to={`/jobs/${j.id}`} style={{ fontWeight: 500 }}>{j.type === 'discovery' ? 'Discovery run' : j.type === 'poll' ? 'Poll' : j.type}</Link><span className="num cap">{j.done} / {j.total}</span></div>
              <JobBar done={j.done} failed={j.failed} total={j.total} running={j.status === 'running'} />
              <div className="cap" style={{ display: 'flex', gap: 8, alignItems: 'center' }}><JobPill status={j.status} xs />{j.failed ? `${j.failed} failed · ` : ''}{ago(j.createdAt)} by {j.createdBy}</div>
            </div>
          ))}
        </Card>
        <Card title="Recent activity" id="act" right={<Link to="/settings/audit" className="cap" style={{ color: 'var(--accent)' }}>Audit trail →</Link>}>
          {user.role === 'viewer' ? <div className="cb cap">The audit trail needs the operator role.</div> : audit.length === 0 ? <div className="cb cap">No changes recorded yet.</div> : audit.map((e) => (
            <div className="lrow" key={e.id} style={{ alignItems: 'flex-start' }}>
              <Av name={e.actor} sys={e.actor === 'scheduler' || e.actor === 'system'} />
              <div style={{ flex: 1, minWidth: 0 }}><div><b style={{ fontWeight: 500 }}>{e.actor}</b> {describeAudit(e)}</div><div className="cap">{ago(e.at)}</div></div>
            </div>
          ))}
        </Card>
      </div>

      <div className="grid-2">
        <Card title="Consumables low on stock" id="cls" right={<Link to="/consumables" className="cap" style={{ color: 'var(--accent)' }}>Consumables →</Link>}>
          {data.consumables.lowStock.length === 0 ? <div className="cb cap">Every stock item is above its warning threshold.</div> : (
            <div className="tw"><table className="tbl">
              <thead><tr><th>Item</th><th className="r">On hand</th><th>Stock</th><th className="r">Devices</th></tr></thead>
              <tbody>{data.consumables.lowStock.slice(0, 6).map((c) => (
                <tr key={c.id}><td><Link to={`/consumables/${c.id}`} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><Sw color={swatchFor(c.color)} none={!c.color} />{c.name}</Link></td><td className="num r" style={{ fontWeight: 600 }}>{c.onHand}</td><td><StockPill status={c.status} /></td><td className="num r">{c.linkedDevices}</td></tr>
              ))}</tbody>
            </table></div>
          )}
          <div className="cf cap">Stock on the shelf, separate from “Supplies running low”, which is about levels inside devices.{data.consumables.pendingReplacements > 0 && <> <Link to="/consumables?tab=unmatched">{data.consumables.pendingReplacements} replacement{data.consumables.pendingReplacements === 1 ? '' : 's'} awaiting confirmation</Link></>}</div>
        </Card>
        <Card title="Upcoming replacements & reorder forecast" id="rf" right={<span className="cap">from linked devices' consumption</span>}>
          {data.consumables.reorder.length === 0 ? <div className="cb cap">No forecast yet: levels need a few days of polling history on linked devices.</div> : (
            <div className="tw"><table className="tbl">
              <thead><tr><th>Item</th><th className="r">On hand</th><th>Runs out</th><th>Next need</th></tr></thead>
              <tbody>{data.consumables.reorder.slice(0, 6).map((c) => (
                <tr key={c.id}><td><Link to={`/consumables/${c.id}`}>{c.name}</Link></td><td className="num r">{c.onHand}</td>
                  <td className="num" style={{ color: c.forecast.reorderSoon ? 'var(--warn)' : undefined, fontWeight: c.forecast.reorderSoon ? 600 : 400 }}>{c.forecast.stockDays !== null ? fmtDays(c.forecast.stockDays) : '—'}</td>
                  <td>{c.forecast.reorderSoon ? <Pill tone="warn">Reorder soon</Pill> : <span className="cap">{c.forecast.nextNeedDays !== null ? fmtDays(c.forecast.nextNeedDays) : '—'}</span>}</td></tr>
              ))}</tbody>
            </table></div>
          )}
        </Card>
      </div>
    </>
  )
}

function describeAudit(e: AuditEntry): string {
  const t = e.target ? e.target.slice(0, 8) : ''
  switch (e.action) {
    case 'discovery.run': return 'started a discovery run'
    case 'device.patch': return `edited device ${t}`
    case 'device.state': return `changed device ${t} to ${String(e.after?.state || '').replace('_', ' ')}`
    case 'alert.ack': return 'acknowledged an alert'
    case 'alert.clear': return 'cleared an alert'
    case 'credential.create': return `added credential “${e.after?.name}”`
    case 'group.create': return `created group “${e.after?.name}”`
    case 'user.create': return `added user ${e.after?.username}`
    default: return `${e.action}${t ? ' · ' + t : ''}`
  }
}

function DashboardSkeleton() {
  return (
    <div className="grid-auto" aria-busy="true">
      {Array.from({ length: 6 }).map((_, i) => <div className="kpi skel" key={i} style={{ animation: 'none' }}><span className="skel" style={{ width: '60%', height: 10 }} /><span className="skel" style={{ width: '40%', height: 22 }} /><span className="skel" style={{ width: '70%', height: 10 }} /></div>)}
    </div>
  )
}
