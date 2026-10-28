import { useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { Device, Group, User } from '../types'
import { deviceName } from '../types'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, EmptyState, Pill, Tag } from '../components/ui'
import type { DriftReport, DriftResponse, Profile } from '../types-config'
import { areaLabel } from '../types-config'
import { StatTile, fmtValue, settingName, useCatalog } from '../components/config/shared'

/** Drift report for a profile: devices × drifted settings, with a way into the apply flow. */
export default function Drift({ user }: { user: User }) {
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const canOperate = user.role !== 'viewer'
  const { byKey } = useCatalog()
  const [profile, setProfile] = useState<Profile | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [refresh, setRefresh] = useState(false)
  const [running, setRunning] = useState(false)
  const [res, setRes] = useState<DriftResponse | null>(null)
  const [ranAt, setRanAt] = useState('')
  const [error, setError] = useState('')
  const groupId = params.get('group') || ''

  useEffect(() => {
    api.get<{ profile: Profile }>(`/api/profiles/${id}`).then((r) => setProfile(r.profile)).catch((e) => setError(e.message))
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices)).catch(() => {})
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
  }, [id])
  const deviceMap = useMemo(() => new Map(devices.map((d) => [d.id, d])), [devices])
  const group = groups.find((g) => g.id === groupId) || null

  const run = async () => {
    setRunning(true); setError('')
    try {
      const target = groupId ? { groupId } : { deviceIds: devices.map((d) => d.id) }
      if (!groupId && devices.length === 0) throw new Error('No devices to check.')
      setRes(await api.post<DriftResponse>(`/api/profiles/${id}/drift`, { target, refresh }))
      setRanAt(new Date().toISOString())
    } catch (e) { setError((e as Error).message) } finally { setRunning(false) }
  }

  const rows = useMemo(() => (res?.reports || []).map((r) => ({ r, d: deviceMap.get(r.deviceId), failed: r.items.some((i) => i.key === ''), n: r.items.filter((i) => i.key !== '').length })).sort((a, b) => Number(b.failed) - Number(a.failed) || b.n - a.n || (a.d ? deviceName(a.d) : '').localeCompare(b.d ? deviceName(b.d) : '')), [res, deviceMap])
  const columns = useMemo(() => {
    if (!profile) return []
    const counts = new Map<string, number>()
    for (const row of rows) for (const it of row.r.items) if (it.key) counts.set(it.key, (counts.get(it.key) || 0) + 1)
    return profile.settings.filter((s) => counts.has(s.key)).map((s) => ({ key: s.key, desired: s.value, n: counts.get(s.key) || 0 }))
  }, [profile, rows])
  const driftedRows = rows.filter((x) => x.n > 0)
  const failedRows = rows.filter((x) => x.failed)
  const itemFor = (r: DriftReport, key: string) => r.items.find((i) => i.key === key)

  if (error && !profile) return <Banner tone="crit">{error}</Banner>
  if (!profile) return <div className="cap" aria-busy="true">Loading…</div>
  const applyTo = groupId ? `/profiles/${id}/apply?group=${groupId}` : `/profiles/${id}/apply`

  return (
    <>
      <div className="ph">
        <div>
          <nav aria-label="Breadcrumb" className="cap"><Link to="/profiles">Configuration</Link> / <Link to="/profiles">Profiles</Link> / {profile.name} / Drift</nav>
          <h1 className="h1" style={{ marginTop: 6 }}>Drift — {profile.name}</h1>
          <div className="cap num">{profile.settings.length} setting{profile.settings.length === 1 ? '' : 's'} in profile · drift compares what the profile wants with what each device has</div>
        </div>
        {canOperate ? <Link className="btn sm wr" to={applyTo}><Ic.upload />Apply profile…</Link> : <Tag>Read-only · Viewer</Tag>}
      </div>
      <Card>
        <div className="cb row" style={{ gap: 12 }}>
          <label className="chip"><Ic.layers />Target<select aria-label="Target" value={groupId} onChange={(e) => { const p = new URLSearchParams(params); if (e.target.value) p.set('group', e.target.value); else p.delete('group'); setParams(p, { replace: true }) }}><option value="">All devices ({devices.length})</option>{groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}</select></label>
          <label className="chip" style={{ cursor: 'pointer' }}><input className="cbx" type="checkbox" checked={refresh} onChange={(e) => setRefresh(e.target.checked)} />Read devices now <span className="cap">(otherwise last snapshot)</span></label>
          <Btn variant="pri" onClick={run} disabled={running}><Ic.activity />{running ? 'Checking…' : 'Run drift check'}</Btn>
          {ranAt && <span className="cap num" style={{ marginLeft: 'auto' }}>checked {fmtTime(ranAt)}{refresh ? ' · live read' : ' · from snapshots'}</span>}
        </div>
        {error && profile && <div className="cf" style={{ color: 'var(--crit)' }}>{error}</div>}
      </Card>
      {res && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(170px, 1fr))', gap: 12 }}>
            <StatTile label="Devices checked" value={res.devices} detail={group ? `group ${group.name}` : 'all devices'} />
            <StatTile label="Drifted" value={driftedRows.length} color={driftedRows.length ? 'var(--warn)' : 'var(--ok)'} detail={driftedRows.length ? 'at least one setting differs' : 'everything matches'} />
            <StatTile label="Matching" value={rows.filter((x) => x.n === 0 && !x.failed).length} color="var(--ok)" />
            <StatTile label="Could not read" value={failedRows.length} color={failedRows.length ? 'var(--off)' : undefined} detail="no snapshot and unreachable" />
            <StatTile label="Settings drifting" value={columns.length} detail={`of ${profile.settings.length}`} />
          </div>
          <Card title="Devices × drifted settings" id="mx" sub="cells show what the device has; the header shows what the profile wants" right={canOperate && driftedRows.length > 0 ? <Link className="btn sm wr" to={applyTo}><Ic.upload />Apply to {group ? group.name : 'all'} (dry run first)</Link> : undefined}>
            {rows.length === 0 ? <div className="cb"><EmptyState icon={<Ic.printer size={20} />} title="No devices in target" tone="muted" /></div>
              : driftedRows.length === 0 && failedRows.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title="No drift" text={`All ${rows.length} device${rows.length === 1 ? '' : 's'} match “${profile.name}”.`} /></div>
                : (
                  <div className="tw"><table className="tbl" style={{ minWidth: 640 }}>
                    <thead><tr><th>Device</th><th className="r">Drifted</th>{columns.map((c) => <th key={c.key} title={c.key}>{settingName(c.key, byKey)}<div className="cap" style={{ fontWeight: 400 }}>{areaLabel(byKey.get(c.key)?.area || 'other')} · wants <span className="new-v mono">{fmtValue(c.desired, byKey.get(c.key))}</span> · {c.n} off</div></th>)}<th /></tr></thead>
                    <tbody>{rows.filter((x) => x.n > 0 || x.failed).map(({ r, d, failed, n }) => (
                      <tr key={r.deviceId}>
                        <td>{d ? <Link to={`/devices/${d.id}`} style={{ fontWeight: 500 }}>{deviceName(d)}</Link> : r.deviceId}{d && <div className="cap mono" style={{ fontSize: 11 }}>{d.address}{d.location ? ` · ${d.location}` : ''}</div>}</td>
                        <td className="r">{failed ? <Pill tone="off" xs title={r.items[0]?.name}>unreadable</Pill> : <Pill tone="warn" xs noIcon>{n}</Pill>}</td>
                        {columns.map((c) => { const it = itemFor(r, c.key); return <td key={c.key} className="mono">{failed ? <span className="cap">—</span> : !it ? <span style={{ color: 'var(--ok)' }}><Ic.check size={13} /></span> : it.missing ? <span className="cap">not exposed</span> : <span className="old">{fmtValue(it.current, byKey.get(c.key))}</span>}</td> })}
                        <td className="r">{canOperate && d && !failed && <Link className="btn sm" to={`/profiles/${id}/apply?device=${d.id}`}>Apply to this device</Link>}</td>
                      </tr>
                    ))}</tbody>
                  </table></div>
                )}
            {failedRows.length > 0 && <div className="cf cap">{failedRows.map(({ r, d }) => <div key={r.deviceId}>{d ? deviceName(d) : r.deviceId}: {r.items[0]?.name}</div>)}</div>}
          </Card>
        </>
      )}
    </>
  )
}
