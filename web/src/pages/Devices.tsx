import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, ago } from '../api'
import type { Device, Group } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { SeverityBadge, StateBadge } from '../components/ui'

export default function Devices() {
  const [params, setParams] = useSearchParams()
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const q = params.get('q') || ''
  const qs = params.toString()

  const load = useCallback(() => {
    api.get<{ devices: Device[] }>(`/api/devices?${qs}`).then((r) => setDevices(r.devices)).catch((e) => setError(e.message))
  }, [qs])
  useEffect(() => { load() }, [load])
  useEffect(() => { api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  useEvents((e) => { if (e.type.startsWith('device.')) load() }, [load])

  const set = (k: string, v: string) => {
    const p = new URLSearchParams(params)
    if (v) p.set(k, v); else p.delete(k)
    setParams(p, { replace: true })
  }

  return (
    <>
      <div className="row between">
        <h1>Devices <span className="muted small">({devices.length})</span></h1>
        <div className="row">
          <a href={`/api/devices.csv?${qs}`}>Export CSV</a>
          <a href={`/api/supplies.csv?${qs}`}>Supplies CSV</a>
        </div>
      </div>
      <div className="panel row">
        <input placeholder="Search name, model, serial, address, location…" value={q} onChange={(e) => set('q', e.target.value)} style={{ minWidth: 280 }} />
        <select value={params.get('severity') || ''} onChange={(e) => set('severity', e.target.value)}>
          <option value="">Any status</option><option value="ok">OK</option><option value="warning">Warning</option><option value="critical">Critical</option><option value="offline">Offline</option>
        </select>
        <select value={params.get('state') || ''} onChange={(e) => set('state', e.target.value)}>
          <option value="">Any lifecycle state</option>
          {['discovered', 'onboarded', 'active', 'needs_attention', 'decommissioned'].map((s) => <option key={s} value={s}>{s.replace('_', ' ')}</option>)}
        </select>
        <select value={params.get('group') || ''} onChange={(e) => set('group', e.target.value)}>
          <option value="">Any group</option>
          {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
        </select>
        <select value={params.get('sort') || ''} onChange={(e) => set('sort', e.target.value)}>
          <option value="">Sort: name</option><option value="model">Sort: model</option><option value="address">Sort: address</option><option value="severity">Sort: severity</option><option value="lastSeen">Sort: last seen</option>
        </select>
      </div>
      {error && <div className="error">{error}</div>}
      <div className="panel">
        <table>
          <thead><tr><th>Name</th><th>Model</th><th>Address</th><th>Status</th><th>Supplies</th><th>Pages</th><th>State</th><th>Last polled</th></tr></thead>
          <tbody>
            {devices.map((d) => (
              <tr key={d.id} className="clickable" onClick={() => navigate(`/devices/${d.id}`)}>
                <td><strong>{deviceName(d)}</strong><div className="muted small">{d.location || d.serial}</div></td>
                <td>{d.model}<div className="muted small">{d.make}{d.firmware ? ` · fw ${d.firmware}` : ''}</div></td>
                <td>{d.address}</td>
                <td><SeverityBadge d={d} /></td>
                <td><SupplyDots supplies={d.supplies} /></td>
                <td>{d.counters ? d.counters.total.toLocaleString() : '—'}</td>
                <td><StateBadge state={d.state} /></td>
                <td className="muted small">{ago(d.lastPolled)}</td>
              </tr>
            ))}
            {devices.length === 0 && <tr><td colSpan={8} className="muted">No devices match. Run a discovery to find printers.</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}

function SupplyDots({ supplies }: { supplies: Device['supplies'] }) {
  const consumed = supplies.filter((s) => s.class !== 'other' && s.percent !== null)
  if (consumed.length === 0) return <span className="muted">—</span>
  return (
    <div className="row" style={{ gap: 6 }}>
      {consumed.slice(0, 6).map((s) => {
        const p = s.class === 'filled' ? 100 - (s.percent ?? 0) : (s.percent ?? 0)
        const cls = p <= 5 ? 'critical' : p <= 15 ? 'warning' : ''
        return <span key={s.index} className={`bar ${cls}`} style={{ width: 40, minWidth: 40 }} title={`${s.description}: ${s.percent}%`}><div style={{ width: `${p}%` }} /></span>
      })}
    </div>
  )
}
