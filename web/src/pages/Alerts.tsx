import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { Alert, Device, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Btn, Card, ChipSelect, EmptyState, SearchBox, Seg, SevPill, Tag } from '../components/ui'

export default function Alerts({ user }: { user: User }) {
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [devices, setDevices] = useState<Record<string, Device>>({})
  const [history, setHistory] = useState(false)
  const [sev, setSev] = useState<'all' | 'critical' | 'warning' | 'cleared'>('all')
  const [source, setSource] = useState('')
  const [q, setQ] = useState('')
  const [grouped, setGrouped] = useState<'flat' | 'device'>('flat')
  const [sel, setSel] = useState<Record<string, boolean>>({})
  const [error, setError] = useState('')
  const canOperate = user.role !== 'viewer'
  const load = useCallback(() => {
    api.get<Alert[]>(`/api/alerts?all=${history}`).then(setAlerts).catch((e) => setError(e.message))
    api.get<{ devices: Device[] }>('/api/devices?includeDecommissioned=true').then((r) => setDevices(Object.fromEntries(r.devices.map((d) => [d.id, d])))).catch(() => {})
  }, [history])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('alert.')) load() }, [load])

  const act = async (ids: string[], what: 'ack' | 'clear') => {
    try { await Promise.all(ids.map((id) => api.post(`/api/alerts/${id}/${what}`))); setSel({}); load() } catch (e) { setError((e as Error).message) }
  }
  const sources = useMemo(() => Array.from(new Set(alerts.map((a) => a.source))).sort(), [alerts])
  const list = alerts.filter((a) => {
    if (sev === 'critical' && (a.severity !== 'critical' || a.clearedAt)) return false
    if (sev === 'warning' && (a.severity !== 'warning' || a.clearedAt)) return false
    if (sev === 'cleared' && !a.clearedAt) return false
    if (source && a.source !== source) return false
    if (q) { const d = devices[a.deviceId]; const hay = `${a.description} ${d ? deviceName(d) + ' ' + d.model + ' ' + d.address : ''}`.toLowerCase(); if (!hay.includes(q.toLowerCase())) return false }
    return true
  })
  const counts = { all: alerts.length, critical: alerts.filter((a) => a.severity === 'critical' && !a.clearedAt).length, warning: alerts.filter((a) => a.severity === 'warning' && !a.clearedAt).length, cleared: alerts.filter((a) => a.clearedAt).length, acked: alerts.filter((a) => a.ackedAt && !a.clearedAt).length }
  const selected = Object.keys(sel).filter((k) => sel[k])
  const rows: { group?: string; count?: number; a?: Alert }[] = grouped === 'device'
    ? Object.entries(list.reduce<Record<string, Alert[]>>((m, a) => { (m[a.deviceId || 'stock'] ||= []).push(a); return m }, {})).flatMap(([id, as]) => [{ group: id, count: as.length }, ...as.map((a) => ({ a }))])
    : list.map((a) => ({ a }))
  const devName = (id: string) => (devices[id] ? deviceName(devices[id]) : id ? id.slice(0, 8) : 'Stock')
  const devHref = (a: Alert) => (a.deviceId ? `/devices/${a.deviceId}` : a.source === 'consumable' ? `/consumables/${a.key.replace('consumable:', '')}` : '/consumables')

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Alerts</h1><div className="cap num">{counts.critical + counts.warning} active · {counts.acked} acknowledged · trap listener on</div></div>
        <div className="row"><Seg label="Active or history" value={history ? 'history' : 'active'} onChange={(v) => setHistory(v === 'history')} options={[{ v: 'active', label: 'Active' }, { v: 'history', label: 'History' }]} /></div>
      </div>
      <div className="row">
        <Seg label="Severity" value={sev} onChange={setSev} options={[
          { v: 'all', label: <>All <span className="num cap">{counts.all}</span></> },
          { v: 'critical', label: <><span className="dot" style={{ background: 'var(--crit)' }} />Critical <span className="num">{counts.critical}</span></> },
          { v: 'warning', label: <><span className="dot" style={{ background: 'var(--warn)' }} />Warning <span className="num">{counts.warning}</span></> },
          ...(history ? [{ v: 'cleared' as const, label: <><span className="dot" style={{ background: 'var(--ok)' }} />Cleared <span className="num">{counts.cleared}</span></> }] : []),
        ]} />
        <ChipSelect label="Source" value={source} onChange={setSource} options={sources.map((s) => ({ v: s, label: s }))} />
        <SearchBox sm filter value={q} onChange={setQ} placeholder="Device or description…" />
        <div style={{ marginLeft: 'auto' }}><Seg label="Grouping" value={grouped} onChange={setGrouped} options={[{ v: 'flat', label: 'Flat' }, { v: 'device', label: 'By device' }]} /></div>
      </div>
      {error && <div className="banner crit">{error}</div>}
      <Card aria-label="Alert list">
        {selected.length > 0 && canOperate && (
          <div className="bulk" role="toolbar" aria-label="Bulk actions">
            <b className="num" style={{ color: 'var(--accent)' }}>{selected.length} selected</b><span className="vsep" />
            <Btn sm onClick={() => act(selected, 'ack')}><Ic.check />Acknowledge</Btn>
            <Btn sm onClick={() => act(selected, 'clear')}><Ic.x />Clear</Btn>
            <Btn sm variant="ghost" ic style={{ marginLeft: 'auto' }} aria-label="Clear selection" onClick={() => setSel({})}><Ic.x /></Btn>
          </div>
        )}
        {list.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title={alerts.length === 0 ? 'All clear' : 'No alerts match'} text={alerts.length === 0 ? 'No active alerts across the fleet.' : 'Try another severity or source.'} /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 1000 }}>
            <thead><tr>{canOperate && <th style={{ width: 36 }}><span className="sr">Select</span></th>}<th>Severity</th><th>Device</th><th>Description</th><th>Source</th><th>First seen</th><th>Last seen</th><th>State</th><th className="r">Actions</th></tr></thead>
            <tbody>
              {rows.map((r, i) => r.group !== undefined ? (
                <tr key={`g-${r.group}-${i}`} className="grp">{canOperate && <td />}<td colSpan={8}>{r.group === 'stock' ? 'Stock items' : <Link to={`/devices/${r.group}`}>{devName(r.group)}</Link>} <span className="cap" style={{ fontWeight: 400 }}>· {r.count} alert{r.count === 1 ? '' : 's'}</span></td></tr>
              ) : r.a && (
                <tr key={r.a.id} className={sel[r.a.id] ? 'sel' : r.a.clearedAt ? 'dim' : ''}>
                  {canOperate && <td><input className="cbx" type="checkbox" aria-label={`Select alert ${r.a.description}`} checked={!!sel[r.a.id]} onChange={() => setSel((s) => ({ ...s, [r.a!.id]: !s[r.a!.id] }))} disabled={!!r.a.clearedAt} /></td>}
                  <td><SevPill sev={r.a.severity} cleared={!!r.a.clearedAt} /></td>
                  <td><Link to={devHref(r.a)} style={{ fontWeight: 500 }}>{devName(r.a.deviceId)}</Link>{devices[r.a.deviceId] && <div className="cap mono" style={{ fontSize: 11 }}>{devices[r.a.deviceId].model}</div>}</td>
                  <td className="wrap">{r.a.description}</td>
                  <td><Tag>{r.a.source}</Tag></td>
                  <td className="num cap">{ago(r.a.raisedAt)}</td>
                  <td className="num cap">{ago(r.a.updatedAt)}</td>
                  <td>{r.a.clearedAt ? <span className="cap">Cleared {ago(r.a.clearedAt)}</span> : r.a.ackedAt ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, color: 'var(--text-2)' }}><Ic.check size={14} />Ack · {r.a.ackedBy}</span> : <span style={{ fontWeight: 500 }}>Active</span>}</td>
                  <td className="r" style={{ whiteSpace: 'nowrap' }}>{canOperate && !r.a.clearedAt && (<>{!r.a.ackedAt && <Btn sm onClick={() => act([r.a!.id], 'ack')}>Acknowledge</Btn>} <Btn sm variant="ghost" onClick={() => act([r.a!.id], 'clear')}>Clear</Btn></>)}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
        <div className="cf" style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: 8 }}><span className="cap num">Showing {list.length} of {alerts.length} · newest first · updates live</span><span className="cap">Cleared alerts stay in History.</span></div>
      </Card>
    </>
  )
}
