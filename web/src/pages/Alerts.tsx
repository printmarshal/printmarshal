import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { Alert, Device, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Badge } from '../components/ui'

export default function Alerts({ user }: { user: User }) {
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [devices, setDevices] = useState<Record<string, Device>>({})
  const [showAll, setShowAll] = useState(false)
  const load = useCallback(() => {
    api.get<Alert[]>(`/api/alerts?all=${showAll}`).then(setAlerts).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices?includeDecommissioned=true').then((r) => setDevices(Object.fromEntries(r.devices.map((d) => [d.id, d])))).catch(() => {})
  }, [showAll])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('alert.')) load() }, [load])
  const act = async (a: Alert, what: 'ack' | 'clear') => { await api.post(`/api/alerts/${a.id}/${what}`); load() }
  return (
    <>
      <div className="row between"><h1>Alerts</h1><label><input type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} /> include cleared</label></div>
      <div className="panel">
        <table>
          <thead><tr><th>Severity</th><th>Device</th><th>Description</th><th>Source</th><th>Raised</th><th>Status</th><th /></tr></thead>
          <tbody>
            {alerts.map((a) => {
              const d = devices[a.deviceId]
              return (
                <tr key={a.id}>
                  <td><Badge sev={a.severity}>{a.severity}</Badge></td>
                  <td>{d ? <Link to={`/devices/${d.id}`}>{deviceName(d)}</Link> : a.deviceId ? a.deviceId.slice(0, 8) : a.source === 'consumable' ? <Link to={`/consumables/${a.key.replace('consumable:', '')}`}>stock item</Link> : <span className="muted">system</span>}{d && <div className="muted small">{d.address}</div>}</td>
                  <td>{a.description}</td>
                  <td className="small">{a.source === 'consumable' ? <span className="badge outline">consumable</span> : a.source}</td>
                  <td className="small">{ago(a.raisedAt)}</td>
                  <td className="small">{a.clearedAt ? `cleared ${ago(a.clearedAt)}` : a.ackedAt ? `acked by ${a.ackedBy}` : 'active'}</td>
                  <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                    {user.role !== 'viewer' && !a.clearedAt && (<>
                      {!a.ackedAt && <button onClick={() => act(a, 'ack')}>Ack</button>}{' '}
                      <button onClick={() => act(a, 'clear')}>Clear</button>
                    </>)}
                  </td>
                </tr>
              )
            })}
            {alerts.length === 0 && <tr><td colSpan={7} className="muted">No alerts.</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
