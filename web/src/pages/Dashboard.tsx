import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { Dashboard as DashboardData } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Badge, JobStatus, Progress, SeverityBadge } from '../components/ui'
import { ColorSwatch, ForecastCell, StockPill, fmtDays } from '../components/consumables'

export default function Dashboard() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState('')
  const load = useCallback(() => { api.get<DashboardData>('/api/dashboard').then(setData).catch((e) => setError(e.message)) }, [])
  useEffect(() => { load() }, [load])
  useEvents(() => load())

  if (error) return <div className="error">{error}</div>
  if (!data) return <div className="muted">Loading…</div>
  const s = data.summary
  return (
    <>
      <h1>Dashboard</h1>
      <div className="grid cols-4">
        <Stat label="Devices" value={s.devices} to="/devices" />
        <Stat label="Critical" value={s.bySeverity.critical || 0} to="/devices?severity=critical" cls="crit" />
        <Stat label="Warning" value={s.bySeverity.warning || 0} to="/devices?severity=warning" cls="warn" />
        <Stat label="Offline" value={s.offline} to="/devices?severity=offline" cls="offline" />
      </div>
      <div className="grid cols-4" style={{ marginTop: 16 }}>
        <Stat label="Active alerts" value={s.activeAlerts} to="/alerts" />
        <Stat label="Low supplies (device levels)" value={s.lowSupplies} to="/consumables?tab=fleet" />
        <Stat label="Consumables low on stock" value={s.lowStock} to="/consumables?status=low" cls={s.lowStock ? 'warn' : undefined} />
        <Stat label="Running jobs" value={s.runningJobs} to="/jobs" />
      </div>
      <div className="grid cols-2" style={{ marginTop: 16 }}>
        <div className="panel">
          <h2>Consumables low on stock</h2>
          {data.consumables.lowStock.length === 0 && <div className="muted">Every stock item is above its warning threshold.</div>}
          <table><tbody>
            {data.consumables.lowStock.slice(0, 8).map((c) => (
              <tr key={c.id}><td><Link to={`/consumables/${c.id}`}><ColorSwatch color={c.color} />{c.name}</Link><div className="muted small">{c.partNumber || c.type}{c.location ? ` · ${c.location}` : ''}</div></td>
                <td><StockPill status={c.status} /></td><td className="small">{c.onHand} on hand · warn ≤ {c.warningAt}</td></tr>
            ))}
          </tbody></table>
          {data.consumables.pendingReplacements > 0 && <div className="muted small" style={{ marginTop: 8 }}><Link to="/consumables?tab=unmatched">{data.consumables.pendingReplacements} replacement{data.consumables.pendingReplacements === 1 ? '' : 's'} awaiting confirmation</Link></div>}
        </div>
        <div className="panel">
          <h2>Upcoming replacements &amp; reorder forecast</h2>
          {data.consumables.reorder.length === 0 && <div className="muted">No forecast yet: levels need a few days of polling history on linked devices.</div>}
          <table><tbody>
            {data.consumables.reorder.map((c) => (
              <tr key={c.id}><td><Link to={`/consumables/${c.id}`}><ColorSwatch color={c.color} />{c.name}</Link><div className="muted small">{c.linkedDevices} device{c.linkedDevices === 1 ? '' : 's'} · {c.onHand} on hand{c.forecast.nextNeedDays !== null ? ` · next need ${fmtDays(c.forecast.nextNeedDays)}` : ''}</div></td>
                <td><ForecastCell f={c.forecast} /></td></tr>
            ))}
          </tbody></table>
        </div>
      </div>
      <div className="grid cols-2" style={{ marginTop: 16 }}>
        <div className="panel">
          <h2>Needs attention</h2>
          {data.attention.length === 0 && <div className="muted">All polled devices are healthy.</div>}
          <table><tbody>
            {data.attention.map((d) => (
              <tr key={d.id}><td><Link to={`/devices/${d.id}`}>{deviceName(d)}</Link><div className="muted small">{d.model} · {d.address}</div></td>
                <td><SeverityBadge d={d} /></td><td className="small muted">{d.status?.message || d.status?.errors?.join(', ')}</td></tr>
            ))}
          </tbody></table>
        </div>
        <div className="panel">
          <h2>Latest alerts</h2>
          {data.alerts.length === 0 && <div className="muted">No active alerts.</div>}
          <table><tbody>
            {data.alerts.map((a) => (
              <tr key={a.id}><td><Badge sev={a.severity}>{a.severity}</Badge></td>
                <td><Link to={a.deviceId ? `/devices/${a.deviceId}` : a.source === 'consumable' ? `/consumables/${a.key.replace('consumable:', '')}` : '/alerts'}>{a.description}</Link><div className="muted small">{a.source} · {ago(a.raisedAt)}{a.ackedBy ? ` · acked by ${a.ackedBy}` : ''}</div></td></tr>
            ))}
          </tbody></table>
        </div>
      </div>
      <div className="panel">
        <h2>Recent jobs</h2>
        {data.jobs.length === 0 && <div className="muted">No jobs yet. Start a <Link to="/discovery">discovery run</Link> to find printers.</div>}
        <table><tbody>
          {data.jobs.map((j) => (
            <tr key={j.id}><td><Link to={`/jobs/${j.id}`}>{j.type}</Link></td><td><JobStatus status={j.status} /></td>
              <td style={{ width: 200 }}><Progress done={j.done} total={j.total} /></td>
              <td className="small muted">{j.done}/{j.total}{j.failed ? ` · ${j.failed} failed` : ''} · {ago(j.createdAt)} by {j.createdBy}</td></tr>
          ))}
        </tbody></table>
      </div>
    </>
  )
}

function Stat({ label, value, to, cls }: { label: string; value: number; to: string; cls?: string }) {
  return (
    <Link to={to} className="panel stat" style={{ marginBottom: 0 }}>
      <span className="value" style={cls ? { color: `var(--${cls})` } : undefined}>{value}</span>
      <span className="label">{label}</span>
    </Link>
  )
}
