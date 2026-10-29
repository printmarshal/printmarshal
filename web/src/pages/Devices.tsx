import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, ago } from '../api'
import type { Device, Group, Job, RenameResponse, RenameRow, User } from '../types'
import { deviceName } from '../types'
import { useEvents } from '../ws'
import { useDensity } from '../theme'
import { Ic } from '../components/icons'
import { Bar, Btn, Card, ChipSelect, DevicePill, Dialog, EmptyState, Msb, Pill, SearchBox, Seg, Tag, fmtInt, remaining, supplyColor } from '../components/ui'

export default function Devices({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [view, setView] = useState<'table' | 'cards'>(() => { try { return (localStorage.getItem('pm.devices.view') as 'table' | 'cards') || 'table' } catch { return 'table' } })
  const [density, setDensity] = useDensity()
  const [sel, setSel] = useState<Record<string, boolean>>({})
  const [msg, setMsg] = useState('')
  const [renaming, setRenaming] = useState(false)
  const navigate = useNavigate()
  const q = params.get('q') || ''
  const qs = params.toString()
  const canOperate = user.role !== 'viewer'

  const load = useCallback(() => {
    api.get<{ devices: Device[] }>(`/api/devices?${qs}`).then((r) => { setDevices(r.devices); setLoading(false) }).catch((e) => { setError(e.message); setLoading(false) })
  }, [qs])
  useEffect(() => { load() }, [load])
  useEffect(() => { api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  useEvents((e) => { if (e.type.startsWith('device.')) load() }, [load])

  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const clearAll = () => setParams(new URLSearchParams(), { replace: true })
  const activeFilters = ['severity', 'state', 'group', 'make', 'model'].filter((k) => params.get(k))
  const makes = useMemo(() => Array.from(new Set(devices.map((d) => d.make).filter(Boolean))).sort(), [devices])
  const models = useMemo(() => Array.from(new Set(devices.map((d) => d.model).filter(Boolean))).sort(), [devices])
  const selected = Object.keys(sel).filter((id) => sel[id])
  const toggle = (id: string) => setSel((s) => ({ ...s, [id]: !s[id] }))
  const toggleAll = () => { if (selected.length === devices.length) setSel({}); else setSel(Object.fromEntries(devices.map((d) => [d.id, true]))) }
  const pollSelected = async () => {
    try { const j = await api.post<Job>('/api/poll', { deviceIds: selected }); setMsg(`Poll job queued for ${selected.length} device${selected.length === 1 ? '' : 's'}.`); setSel({}); navigate(`/jobs/${j.id}`) } catch (e) { setMsg((e as Error).message) }
  }
  const addToGroup = async (gid: string) => {
    if (!gid) return
    const g = groups.find((x) => x.id === gid)
    if (!g) return
    try { await api.put(`/api/groups/${gid}/members`, { deviceIds: Array.from(new Set([...g.memberIds, ...selected])) }); setMsg(`Added ${selected.length} to ${g.name}.`); setSel({}) } catch (e) { setMsg((e as Error).message) }
  }
  const useDNSName = async (deviceId: string) => {
    try {
      await api.post<RenameResponse>('/api/devices/rename', { deviceIds: [deviceId], source: 'dns', dryRun: false })
      load()
    } catch (e) { setMsg((e as Error).message) }
  }
  const counts = useMemo(() => ({ sites: new Set(devices.map((d) => d.location.split('·')[0].trim()).filter(Boolean)).size, models: models.length }), [devices, models])
  const setView2 = (v: 'table' | 'cards') => { setView(v); try { localStorage.setItem('pm.devices.view', v) } catch { /* ignore */ } }

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Devices</h1><div className="cap num">{fmtInt(devices.length)} devices{counts.sites ? ` · ${counts.sites} location${counts.sites === 1 ? '' : 's'}` : ''} · {counts.models} model{counts.models === 1 ? '' : 's'}</div></div>
        <div className="row">
          <a className="btn sm" href={`/api/devices.csv?${qs}`}><Ic.download />Export CSV</a>
          <a className="btn sm" href={`/api/supplies.csv?${qs}`}><Ic.download />Supplies CSV</a>
          {canOperate && <Link className="btn sm pri" to="/discovery"><Ic.radar />Run discovery</Link>}
        </div>
      </div>
      <div className="row">
        <SearchBox sm filter value={q} onChange={(v) => set('q', v)} placeholder="Filter by name, IP, serial, model…" />
        <ChipSelect label="Status" value={params.get('severity') || ''} onChange={(v) => set('severity', v)} options={[{ v: 'ok', label: 'OK' }, { v: 'warning', label: 'Warning' }, { v: 'critical', label: 'Critical' }, { v: 'offline', label: 'Offline' }]} />
        <ChipSelect label="Make" value={params.get('make') || ''} onChange={(v) => set('make', v)} options={makes.map((m) => ({ v: m!, label: m! }))} />
        <ChipSelect label="Model" value={params.get('model') || ''} onChange={(v) => set('model', v)} options={models.map((m) => ({ v: m!, label: m! }))} />
        <ChipSelect label="Group" value={params.get('group') || ''} onChange={(v) => set('group', v)} options={groups.map((g) => ({ v: g.id, label: g.name }))} />
        <ChipSelect label="Lifecycle" value={params.get('state') || ''} onChange={(v) => set('state', v)} options={['discovered', 'onboarded', 'active', 'needs_attention', 'decommissioned'].map((s) => ({ v: s, label: s.replace('_', ' ') }))} />
        {(activeFilters.length > 0 || q) && <Btn sm variant="ghost" onClick={clearAll}>Clear all</Btn>}
      </div>
      <div className="row">
        <span className="cap">Saved filters</span>
        <button className="chip pillish" onClick={() => setParams(new URLSearchParams({ sort: 'severity', severity: 'critical' }), { replace: true })}>Critical now</button>
        <button className="chip pillish" onClick={() => setParams(new URLSearchParams({ severity: 'offline' }), { replace: true })}>Offline</button>
        <button className="chip pillish" onClick={() => setParams(new URLSearchParams({ state: 'discovered' }), { replace: true })}>Discovered, not onboarded</button>
      </div>
      {error && <div className="banner crit">{error}</div>}
      {msg && <div className="banner info" role="status"><Ic.info style={{ color: 'var(--info)' }} /><div style={{ flex: 1 }}>{msg}</div><Btn sm variant="ghost" ic onClick={() => setMsg('')}><Ic.x /></Btn></div>}
      <Card aria-label="Device results">
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8, padding: '10px 12px', borderBottom: '1px solid var(--border)' }}>
          <span className="num" style={{ fontWeight: 500 }}>{devices.length} matching <span className="cap">· sorted by {params.get('sort') || 'name'}</span></span>
          <div className="row">
            <ChipSelect label="Sort" value={params.get('sort') || ''} onChange={(v) => set('sort', v)} any="Name" options={[{ v: 'severity', label: 'Severity' }, { v: 'model', label: 'Model' }, { v: 'address', label: 'Address' }, { v: 'lastSeen', label: 'Last seen' }]} />
            <Seg label="View" value={view} onChange={setView2} options={[{ v: 'table', label: <><Ic.list />Table</> }, { v: 'cards', label: <><Ic.dashboard />Cards</> }]} />
            <Seg label="Density" value={density} onChange={setDensity} options={[{ v: 'comfortable', label: 'Comfortable' }, { v: 'compact', label: 'Compact' }]} />
          </div>
        </div>
        {selected.length > 0 && canOperate && (
          <div className="bulk" role="toolbar" aria-label="Bulk actions">
            <b className="num" style={{ color: 'var(--accent)' }}>{selected.length} selected</b>
            <Btn sm variant="ghost" style={{ color: 'var(--accent)' }} onClick={toggleAll}>{selected.length === devices.length ? 'Clear' : `Select all ${devices.length}`}</Btn>
            <span className="vsep" />
            <Btn sm onClick={pollSelected}><Ic.refresh />Poll now</Btn>
            <Btn sm onClick={() => setRenaming(true)}><Ic.pencil />Rename…</Btn>
            <label className="chip"><Ic.layers />Add to group<select aria-label="Add to group" value="" onChange={(e) => addToGroup(e.target.value)}><option value="">choose…</option>{groups.filter((g) => g.kind === 'static').map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}</select></label>
            <a className="btn sm" href={`/api/devices.csv?${qs}`}><Ic.download />Export</a>
            <Btn sm variant="ghost" ic style={{ marginLeft: 'auto' }} aria-label="Clear selection" onClick={() => setSel({})}><Ic.x /></Btn>
          </div>
        )}
        {loading ? <div className="cb" aria-busy="true">{[0, 1, 2].map((i) => <div key={i} className="row" style={{ padding: '8px 0' }}><span className="skel" style={{ width: 60, height: 18, borderRadius: 999 }} /><span className="skel" style={{ flex: 1, height: 10 }} /></div>)}</div>
          : devices.length === 0 ? <div className="cb"><EmptyState icon={<Ic.radar size={20} />} title={qs ? 'No devices match' : 'No devices yet'} text={qs ? 'Try fewer filters.' : 'Find printers on your network to start managing them.'} action={qs ? <Btn sm onClick={clearAll}>Clear filters</Btn> : canOperate ? <Link className="btn sm pri" to="/discovery">Run discovery</Link> : undefined} /></div>
          : view === 'table' ? (
            <div className="tw" style={{ maxHeight: 760, overflow: 'auto' }}>
              <table className={`tbl${density === 'compact' ? ' compact' : ''}`} style={{ minWidth: 1080 }}>
                <thead><tr>
                  {canOperate && <th style={{ width: 36 }}><input className="cbx" type="checkbox" aria-label="Select all visible" checked={selected.length === devices.length && devices.length > 0} onChange={toggleAll} /></th>}
                  <th>Status</th><th>Display name</th><th>Model</th><th>IP / hostname</th><th>Location</th><th>Supplies</th><th className="r">Pages</th><th>Firmware</th><th>Lifecycle</th><th className="r">Last seen</th>
                </tr></thead>
                <tbody>
                  {devices.map((d) => (
                    <tr key={d.id} className={`clickable${sel[d.id] ? ' sel' : ''}`} onClick={() => navigate(`/devices/${d.id}`)}>
                      {canOperate && <td onClick={(e) => e.stopPropagation()}><input className="cbx" type="checkbox" aria-label={`Select ${deviceName(d)}`} checked={!!sel[d.id]} onChange={() => toggle(d.id)} /></td>}
                      <td><DevicePill d={d} /></td>
                      <td>
                        <Link to={`/devices/${d.id}`} style={{ fontWeight: 500 }} onClick={(e) => e.stopPropagation()}>{deviceName(d)}</Link>
                        {d.serial && <div className="cap mono" style={{ fontSize: 11 }}>{d.serial}</div>}
                        {d.dnsNameChanged && (
                          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginTop: 2 }} onClick={(e) => e.stopPropagation()}>
                            <Pill tone="warn" xs>DNS name changed</Pill>
                            {canOperate && !d.dnsNameSuspect && <Btn sm variant="ghost" style={{ height: 20, padding: '0 6px', fontSize: 11 }} onClick={() => useDNSName(d.id)}>Use DNS name</Btn>}
                          </div>
                        )}
                      </td>
                      <td className="mono">{d.model}</td>
                      <td>
                        <div className="mono">{d.address}</div>
                        {(d.dnsName || d.hostname) && (
                          <div className="cap" style={{ fontSize: 11, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                            {d.dnsName || d.hostname}
                            {d.dnsName && d.dnsNameSuspect && <Pill tone="warn" xs title="Suspect DNS name">suspect</Pill>}
                          </div>
                        )}
                      </td>
                      <td className="cap" style={{ color: 'var(--text-2)' }}>{d.location || '—'}</td>
                      <td><Msb supplies={d.supplies} /></td>
                      <td className="num r">{d.counters ? fmtInt(d.counters.total) : <span className="cap">—</span>}</td>
                      <td><span className="mono">{d.firmware || '—'}</span></td>
                      <td><Tag>{d.state.replace('_', ' ')}</Tag></td>
                      <td className="cap num r">{ago(d.lastPolled || d.lastSeen)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(250px, 1fr))', gap: 12, padding: 12 }}>
              {devices.map((d) => (
                <article key={d.id} className={`dcard${sel[d.id] ? ' sel' : ''}`}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>{canOperate && <input className="cbx" type="checkbox" aria-label={`Select ${deviceName(d)}`} checked={!!sel[d.id]} onChange={() => toggle(d.id)} />}<Link to={`/devices/${d.id}`} style={{ fontWeight: 600, color: 'var(--text)' }}>{deviceName(d)}</Link></label>
                    <DevicePill d={d} />
                  </div>
                  <div>
                    <div className="mono" style={{ color: 'var(--text-2)' }}>{d.model} · {d.address}</div>
                    <div className="cap">{d.location || d.serial}</div>
                    {d.dnsNameChanged && (
                      <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginTop: 4 }}>
                        <Pill tone="warn" xs>DNS name changed</Pill>
                        {canOperate && !d.dnsNameSuspect && <Btn sm variant="ghost" style={{ height: 20, padding: '0 6px', fontSize: 11 }} onClick={() => useDNSName(d.id)}>Use DNS name</Btn>}
                      </div>
                    )}
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                    {d.supplies.filter((s) => s.class !== 'other' && s.percent !== null).slice(0, 5).map((s) => {
                      const r = remaining(s) ?? 0
                      return <div key={s.index} style={{ display: 'grid', gridTemplateColumns: '60px minmax(0,1fr) 34px', alignItems: 'center', gap: 8 }}><span className="cap">{s.colorant || s.type}</span><Bar pct={r} color={supplyColor(s)} /><span className="cap num" style={{ textAlign: 'right', color: r <= 10 ? 'var(--warn)' : undefined }}>{r}%</span></div>
                    })}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
                    <span className="cap num">{d.status?.alerts?.length ? `${d.status.alerts.length} device alert${d.status.alerts.length === 1 ? '' : 's'}` : d.counters ? `${fmtInt(d.counters.total)} pages` : 'No alerts'}</span><span className="cap num">Seen {ago(d.lastSeen)}</span>
                  </div>
                </article>
              ))}
            </div>
          )}
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8, padding: '10px 12px', borderTop: '1px solid var(--border)' }}>
          <span className="cap num">{devices.length} rows</span>
          <span className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Ic.info size={14} style={{ color: 'var(--info)' }} />Decommissioned devices are hidden unless you filter by that lifecycle state.</span>
        </div>
      </Card>
      {renaming && (
        <RenameDialog
          deviceIds={selected}
          onClose={() => setRenaming(false)}
          onApplied={(n) => {
            setRenaming(false)
            setSel({})
            setMsg(`Renamed ${n} device${n === 1 ? '' : 's'}.`)
            load()
          }}
        />
      )}
    </>
  )
}

function RenameDialog({ deviceIds, onClose, onApplied }: { deviceIds: string[]; onClose: () => void; onApplied: (updated: number) => void }) {
  const [source, setSource] = useState<'dns' | 'hostname' | 'sysName' | 'pattern'>('dns')
  const [pattern, setPattern] = useState('{location}-{model}')
  const [rows, setRows] = useState<RenameRow[]>([])
  const [busy, setBusy] = useState(false)
  const [applying, setApplying] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    if (source === 'pattern' && !pattern.trim()) {
      setRows([])
      return
    }
    let cancelled = false
    setBusy(true)
    setErr('')
    api.post<RenameResponse>('/api/devices/rename', {
      deviceIds,
      source,
      pattern: source === 'pattern' ? pattern : undefined,
      dryRun: true,
    }).then((res) => {
      if (!cancelled) setRows(res.results || [])
    }).catch((e) => {
      if (!cancelled) setErr((e as Error).message)
    }).finally(() => {
      if (!cancelled) setBusy(false)
    })
    return () => { cancelled = true }
  }, [deviceIds, source, pattern])

  const willUpdate = rows.filter((r) => !r.skipped).length

  const apply = async () => {
    setApplying(true)
    setErr('')
    try {
      const res = await api.post<RenameResponse>('/api/devices/rename', {
        deviceIds,
        source,
        pattern: source === 'pattern' ? pattern : undefined,
        dryRun: false,
      })
      onApplied(res.updated)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setApplying(false)
    }
  }

  return (
    <Dialog
      title={`Rename ${deviceIds.length} device${deviceIds.length === 1 ? '' : 's'}`}
      sub="Preview the new operator display names before applying. Suspect or empty names are skipped."
      onClose={onClose}
      footer={
        <>
          <Btn variant="ghost" onClick={onClose}>Cancel</Btn>
          <Btn variant="pri" disabled={applying || busy || willUpdate === 0} onClick={apply}>
            {applying ? 'Applying…' : `Apply (${willUpdate})`}
          </Btn>
        </>
      }
    >
      <div className="stack" style={{ gap: 12 }}>
        <div className="row" style={{ gap: 10 }}>
          <label className="cap" htmlFor="rn-source">Source</label>
          <select id="rn-source" className="fin" style={{ width: 220 }} value={source} onChange={(e) => setSource(e.target.value as typeof source)}>
            <option value="dns">DNS name</option>
            <option value="hostname">mDNS / advertised hostname</option>
            <option value="sysName">Advertised sysName</option>
            <option value="pattern">Pattern…</option>
          </select>
        </div>
        {source === 'pattern' && (
          <div>
            <label className="cap" htmlFor="rn-pattern">Pattern</label>
            <input id="rn-pattern" className="fin mono" value={pattern} onChange={(e) => setPattern(e.target.value)} placeholder="{location}-{model}-{serial}" />
            <div className="cap" style={{ marginTop: 4 }}>Tokens: {'{serial}'} {'{model}'} {'{make}'} {'{address}'} {'{hostname}'} {'{dnsName}'} {'{sysName}'} {'{location}'} {'{assetTag}'}</div>
          </div>
        )}
        {err && <div className="banner crit">{err}</div>}
        <div className="tw" style={{ maxHeight: 300, overflow: 'auto', border: '1px solid var(--border)', borderRadius: 6 }}>
          <table className="tbl">
            <thead><tr><th>Current name</th><th>New display name</th><th>Status</th></tr></thead>
            <tbody>
              {busy ? (
                <tr><td colSpan={3} className="cap">Loading preview…</td></tr>
              ) : rows.length === 0 ? (
                <tr><td colSpan={3} className="cap">No preview rows.</td></tr>
              ) : rows.map((r) => (
                <tr key={r.id} style={r.skipped ? { opacity: 0.55 } : undefined}>
                  <td className="mono">{r.from}</td>
                  <td className="mono">{r.skipped ? '—' : r.to}</td>
                  <td>{r.skipped ? <span className="cap">Skipped: {r.reason}</span> : <Pill tone="ok" xs>Will rename</Pill>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </Dialog>
  )
}
