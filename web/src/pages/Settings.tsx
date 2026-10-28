import { useCallback, useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { AuditEntry, ConsumableSettings, User } from '../types'
import { useDensity, useTheme } from '../theme'
import { Ic } from '../components/icons'
import { Av, Banner, Btn, Card, EmptyState, Pill, Seg, Tag } from '../components/ui'

type Section = 'users' | 'notifications' | 'system' | 'appearance' | 'audit' | 'account'

export default function Settings({ user }: { user: User }) {
  const location = useLocation()
  const navigate = useNavigate()
  const seg = location.pathname.split('/')[2] as Section | undefined
  const isAdmin = user.role === 'admin'
  const canOperate = user.role !== 'viewer'
  const sections = ([
    { v: 'users', label: 'Users & roles', icon: <Ic.users />, show: isAdmin },
    { v: 'notifications', label: 'Notifications', icon: <Ic.bell />, show: true },
    { v: 'system', label: 'System & consumables', icon: <Ic.server />, show: true },
    { v: 'appearance', label: 'Appearance', icon: <Ic.dashboard />, show: true },
    { v: 'audit', label: 'Audit trail', icon: <Ic.activity />, show: canOperate },
    { v: 'account', label: 'My account', icon: <Ic.user />, show: true },
  ] as { v: Section; label: string; icon: JSX.Element; show: boolean }[]).filter((s) => s.show)
  const current: Section = sections.some((s) => s.v === seg) ? (seg as Section) : sections[0].v
  const go = (s: Section) => navigate(`/settings/${s}`)
  return (
    <>
      <div><h1 className="h1">Settings</h1><div className="cap">PrintMarshal · self-hosted at {window.location.host}</div></div>
      <div className="cols">
        <nav className="snav card" aria-label="Settings sections" style={{ flex: '1 1 200px', maxWidth: 240 }}>
          {sections.map((s) => <button key={s.v} aria-current={current === s.v ? 'page' : undefined} onClick={() => go(s.v)}>{s.icon}{s.label}</button>)}
          {canOperate && <Link to="/credentials"><Ic.lock />Credentials<Ic.external size={13} style={{ marginLeft: 'auto' }} /></Link>}
        </nav>
        <div className="stack" style={{ flex: '4 1 600px' }}>
          {current === 'users' && <Users me={user} />}
          {current === 'notifications' && <Notifications />}
          {current === 'system' && <System isAdmin={isAdmin} />}
          {current === 'appearance' && <Appearance />}
          {current === 'audit' && <Audit />}
          {current === 'account' && <Account />}
        </div>
      </div>
    </>
  )
}

function SRow({ label, hint, children }: { label: React.ReactNode; hint?: string; children: React.ReactNode }) {
  return <div className="srow2"><span>{label}{hint && <div className="cap">{hint}</div>}</span><div>{children}</div></div>
}

function Users({ me }: { me: User }) {
  const [users, setUsers] = useState<User[]>([])
  const [form, setForm] = useState({ username: '', password: '', role: 'viewer' })
  const [msg, setMsg] = useState('')
  const load = useCallback(() => { api.get<User[]>('/api/users').then(setUsers).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  const add = async (e: React.FormEvent) => { e.preventDefault(); setMsg(''); try { await api.post('/api/users', form); setForm({ username: '', password: '', role: 'viewer' }); load() } catch (err) { setMsg((err as Error).message) } }
  const counts = { admin: users.filter((u) => u.role === 'admin').length, operator: users.filter((u) => u.role === 'operator').length, viewer: users.filter((u) => u.role === 'viewer').length }
  return (
    <>
      <form className="card" onSubmit={add} autoComplete="off" aria-label="Add user">
        <div className="ch"><h2 className="h2">Add a user</h2><span className="cap">local accounts · passwords are hashed, never shown</span></div>
        <div className="cb row">
          <input className="fin" style={{ flex: '1 1 200px' }} placeholder="username" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required />
          <input className="fin" style={{ flex: '1 1 200px' }} type="password" placeholder="password (min 8)" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required autoComplete="new-password" />
          <select className="fin" style={{ width: 140 }} value={form.role} aria-label="Role" onChange={(e) => setForm({ ...form, role: e.target.value })}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select>
          <Btn type="submit" variant="pri">Create user</Btn>
        </div>
        {msg && <div className="cf error small">{msg}</div>}
      </form>
      <Card title="Users" id="us" sub={`${users.length} users · ${counts.admin} admins · ${counts.operator} operators · ${counts.viewer} viewers`}>
        <div className="tw"><table className="tbl" style={{ minWidth: 560 }}><thead><tr><th>Person</th><th>Role</th><th>Created</th><th /></tr></thead><tbody>
          {users.map((u) => <tr key={u.id}><td><div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Av name={u.username} /><div style={{ fontWeight: 500 }}>{u.username} {u.id === me.id && <span className="cap">(you)</span>}</div></div></td><td><Tag>{u.role}</Tag></td><td className="cap num">{fmtTime(u.createdAt)}</td><td className="r">{u.id !== me.id && <Btn sm variant="ghost" onClick={async () => { if (confirm(`Remove ${u.username}?`)) { await api.del(`/api/users/${u.id}`); load() } }}>Remove</Btn>}</td></tr>)}
        </tbody></table></div>
      </Card>
      <Card title="What each role can do" id="rl">
        <div className="tw"><table className="tbl mx" style={{ minWidth: 560 }}><thead><tr><th>Capability</th><th style={{ textAlign: 'center' }}>Viewer</th><th style={{ textAlign: 'center' }}>Operator</th><th style={{ textAlign: 'center' }}>Admin</th></tr></thead><tbody>
          <tr><td>See devices, supplies, alerts, jobs, consumables</td><td>✓</td><td>✓</td><td>✓</td></tr>
          <tr><td>Poll, acknowledge alerts, run discovery, edit device fields, manage groups and stock</td><td className="cap">—</td><td>✓</td><td>✓</td></tr>
          <tr><td>Apply profiles, remediate, run firmware campaigns (phases 4–6)</td><td className="cap">—</td><td>✓</td><td>✓</td></tr>
          <tr><td>Manage credentials, users and system defaults</td><td className="cap">—</td><td className="cap">—</td><td>✓</td></tr>
        </tbody></table></div>
      </Card>
    </>
  )
}

interface RuntimeSettings { version: string; poll: { interval: string; concurrency: number }; discovery: { enabled: boolean; interval: string; targets: string[] | null; maxAddresses: number }; trap: { enabled: boolean; listen: string }; alerts: { supplyWarningPercent: number; supplyCriticalPercent: number; offlineAfterFailures: number }; notifiers: { name: string; type: string; events: string[] | null }[] }

function Notifications() {
  const [s, setS] = useState<RuntimeSettings | null>(null)
  const [log, setLog] = useState<{ id: number; notifier: string; event: string; target: string; ok: boolean; error?: string; at: string }[]>([])
  useEffect(() => { api.get<RuntimeSettings>('/api/settings').then(setS).catch(() => {}); api.get<typeof log>('/api/notifications?limit=50').then(setLog).catch(() => {}) }, [])
  const events = ['alert.raised', 'alert.cleared', 'job.finished', 'device.discovered']
  const sends = (n: RuntimeSettings['notifiers'][number], ev: string) => !n.events || n.events.length === 0 ? ev.startsWith('alert.') : n.events.includes(ev) || n.events.includes('*')
  return (
    <>
      <Card title="Channels" id="chn" sub="configured in the server's JSON config (notifiers); secrets stay in environment variables">
        {!s ? <div className="cb cap">Loading…</div> : s.notifiers.length === 0 ? <div className="cb"><EmptyState icon={<Ic.bell size={20} />} title="No channels configured" text="Add webhook (Slack, Teams, Discord, generic) or SMTP notifiers to the config file. See deploy/printmarshal.example.json." /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 600 }}><thead><tr><th>Channel</th><th>Type</th><th>Last delivery</th></tr></thead><tbody>
            {s.notifiers.map((n) => { const last = log.find((l) => l.notifier === n.name); return <tr key={n.name}><td style={{ fontWeight: 500 }}>{n.name}</td><td><Tag>{n.type}</Tag></td><td>{last ? last.ok ? <Pill tone="ok">{fmtTime(last.at)} · delivered</Pill> : <Pill tone="crit" title={last.error}>{fmtTime(last.at)} · failed</Pill> : <span className="cap">never</span>}</td></tr> })}
          </tbody></table></div>
        )}
      </Card>
      {s && s.notifiers.length > 0 && (
        <Card title="Events → channels" id="ev" sub="from config; routing is not editable here yet">
          <div className="tw"><table className="tbl mx" style={{ minWidth: 560 }}><thead><tr><th>Event</th>{s.notifiers.map((n) => <th key={n.name} style={{ textAlign: 'center' }}>{n.name}</th>)}</tr></thead><tbody>
            {events.map((ev) => <tr key={ev}><td>{ev === 'alert.raised' ? <><Pill tone="crit" xs noIcon>Critical / warning</Pill> alert raised</> : ev}</td>{s.notifiers.map((n) => <td key={n.name}>{sends(n, ev) ? <Ic.check size={14} style={{ color: 'var(--ok)' }} /> : <span className="cap">—</span>}</td>)}</tr>)}
          </tbody></table></div>
        </Card>
      )}
      <Card title="Delivery log" id="dl">
        {log.length === 0 ? <div className="cb cap">No deliveries yet.</div> : <div className="tw"><table className="tbl"><thead><tr><th>When</th><th>Channel</th><th>Event</th><th>Subject</th><th>Result</th></tr></thead><tbody>{log.map((l) => <tr key={l.id}><td className="cap num">{fmtTime(l.at)}</td><td>{l.notifier}</td><td><Tag>{l.event}</Tag></td><td className="wrap cap">{l.target}</td><td>{l.ok ? <Pill tone="ok" xs>ok</Pill> : <Pill tone="crit" xs title={l.error}>failed</Pill>}</td></tr>)}</tbody></table></div>}
      </Card>
    </>
  )
}

function System({ isAdmin }: { isAdmin: boolean }) {
  const [s, setS] = useState<RuntimeSettings | null>(null)
  const [drivers, setDrivers] = useState<{ name: string; capabilities: string[] }[]>([])
  const [cs, setCs] = useState<ConsumableSettings | null>(null)
  const [msg, setMsg] = useState('')
  useEffect(() => {
    api.get<RuntimeSettings>('/api/settings').then(setS).catch(() => {})
    api.get<typeof drivers>('/api/drivers').then(setDrivers).catch(() => {})
    api.get<ConsumableSettings>('/api/consumables/settings').then(setCs).catch(() => {})
  }, [])
  const save = async (e: React.FormEvent) => { e.preventDefault(); if (!cs) return; setMsg(''); try { setCs(await api.put<ConsumableSettings>('/api/consumables/settings', cs)); setMsg('Saved.') } catch (err) { setMsg((err as Error).message) } }
  const num = (k: keyof ConsumableSettings) => (e: React.ChangeEvent<HTMLInputElement>) => cs && setCs({ ...cs, [k]: Math.max(0, Number(e.target.value) || 0) })
  return (
    <>
      <Card title="Polling & discovery" id="pol" sub="from the config file or PM_* environment variables · restart to apply">
        {!s ? <div className="cb cap">Loading…</div> : (<>
          <SRow label="Scheduled poll" hint="status, supplies, trays, counters, firmware"><span className="mono">every {s.poll.interval}</span> · concurrency <span className="num">{s.poll.concurrency}</span></SRow>
          <SRow label="Scheduled discovery">{s.discovery.enabled ? <span className="mono">every {s.discovery.interval} · {(s.discovery.targets || []).join(', ') || 'zero-config only'}</span> : <span className="cap">off · run discovery manually</span>}</SRow>
          <SRow label="Active-scan cap"><span className="num">{s.discovery.maxAddresses}</span> addresses unless “allow large” is set</SRow>
          <SRow label="SNMP trap listener">{s.trap.enabled ? <Pill tone="ok">listening on {s.trap.listen}</Pill> : <Pill tone="off">disabled</Pill>}</SRow>
          <SRow label="Supply alert thresholds">warning ≤ <span className="num">{s.alerts.supplyWarningPercent}%</span> · critical ≤ <span className="num">{s.alerts.supplyCriticalPercent}%</span> · offline after <span className="num">{s.alerts.offlineAfterFailures}</span> failed polls</SRow>
          <SRow label="Version"><span className="mono">{s.version}</span></SRow>
        </>)}
      </Card>
      <Card title="Drivers & plugins" id="drv" sub="vendor plugins register against the same driver interface; the core has no vendor-specific code">
        <div className="tw"><table className="tbl"><thead><tr><th>Driver</th><th>Capabilities</th></tr></thead><tbody>{drivers.map((d) => <tr key={d.name}><td style={{ fontWeight: 500 }}>{d.name}</td><td style={{ whiteSpace: 'normal' }}>{d.capabilities.map((c) => <Tag key={c}>{c.replace('_', ' ')}</Tag>)}</td></tr>)}</tbody></table></div>
      </Card>
      {cs && (
        <form className="card" onSubmit={save} aria-labelledby="cons">
          <div className="ch"><h2 className="h2" id="cons">Consumables</h2><Link to="/consumables" className="cap">Manage stock →</Link></div>
          <SRow label="Default thresholds" hint="applied to new items; each item can override">
            <div className="row" style={{ gap: 16 }}><label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8, color: 'var(--warn)', fontWeight: 500 }}>Warning at <input className="fin num" type="number" min={0} style={{ width: 64, textAlign: 'center' }} value={cs.defaultWarningAt} disabled={!isAdmin} onChange={num('defaultWarningAt')} /></label><label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8, color: 'var(--crit)', fontWeight: 500 }}>Critical at <input className="fin num" type="number" min={0} style={{ width: 64, textAlign: 'center' }} value={cs.defaultCriticalAt} disabled={!isAdmin} onChange={num('defaultCriticalAt')} /></label><span className="cap">units on hand</span></div>
          </SRow>
          <SRow label="Reorder lead time" hint="drives “reorder soon” in forecasts"><div className="row"><input className="fin num" type="number" min={0} style={{ width: 80 }} value={cs.reorderLeadDays} disabled={!isAdmin} onChange={num('reorderLeadDays')} /><span className="cap">days</span></div></SRow>
          <SRow label="Forecast needs at least" hint="days of level history before a rate is estimated"><div className="row"><input className="fin num" type="number" min={1} style={{ width: 80 }} value={cs.forecastMinDays} disabled={!isAdmin} onChange={num('forecastMinDays')} /><span className="cap">days</span></div></SRow>
          <SRow label="Auto-decrement on replacement" hint="when a device's level jumps back up or someone clicks Mark replaced">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}><Seg label="Auto-decrement" value={cs.autoDecrement} onChange={(v) => isAdmin && setCs({ ...cs, autoDecrement: v })} options={[{ v: 'off', label: 'Off' }, { v: 'prompt', label: 'Prompt' }, { v: 'automatic', label: 'Automatic' }]} /><span className="cap">Prompt asks before taking 1 from stock. Automatic logs it with reason “Replaced in device”.</span></div>
          </SRow>
          <SRow label="Level rise that counts as a replacement"><div className="row"><input className="fin num" type="number" min={10} max={95} style={{ width: 80 }} value={cs.replacementJumpPct} disabled={!isAdmin} onChange={num('replacementJumpPct')} /><span className="cap">%</span></div></SRow>
          <SRow label="Suggest stock items from scans"><label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><input className="cbx" type="checkbox" checked={cs.autoSuggestFromScans} disabled={!isAdmin} onChange={(e) => setCs({ ...cs, autoSuggestFromScans: e.target.checked })} />Offer unmatched installed supplies as new items</label></SRow>
          <SRow label="Stock alerts"><span className="cap">Low and out-of-stock alerts appear in Alerts with source “consumable” and follow the notifier routing.</span></SRow>
          {isAdmin && <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, alignItems: 'center' }}><span className="cap">{msg}</span><Btn type="submit" variant="pri">Save</Btn></div>}
        </form>
      )}
    </>
  )
}

function Appearance() {
  const [theme, setTheme] = useTheme()
  const [density, setDensity] = useDensity()
  return (
    <Card title="Appearance & region" id="ap" sub="stored in this browser">
      <SRow label="Theme"><Seg label="Theme" value={theme} onChange={setTheme} options={[{ v: 'light', label: 'Light' }, { v: 'dark', label: 'Dark' }, { v: 'system', label: 'Match system' }]} /></SRow>
      <SRow label="Table density"><Seg label="Density" value={density} onChange={setDensity} options={[{ v: 'comfortable', label: 'Comfortable' }, { v: 'compact', label: 'Compact' }]} /></SRow>
      <SRow label="Time zone"><span className="mono">{Intl.DateTimeFormat().resolvedOptions().timeZone}</span> <span className="cap">· times are shown in your browser's zone</span></SRow>
    </Card>
  )
}

function Audit() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  useEffect(() => { api.get<AuditEntry[]>('/api/audit?limit=300').then(setEntries).catch(() => {}) }, [])
  return (
    <Card title="Audit trail" id="aud" sub="every change to PrintMarshal records and every device write, with who and what">
      {entries.length === 0 ? <div className="cb cap">Nothing audited yet.</div> : (
        <div className="tw"><table className="tbl" style={{ minWidth: 760 }}><thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th><th>Change</th><th>Result</th></tr></thead><tbody>
          {entries.map((e) => <tr key={e.id}><td className="cap num">{fmtTime(e.at)}</td><td>{e.actor}</td><td><Tag>{e.action}</Tag></td><td className="mono cap">{e.target.slice(0, 12)}</td><td className="cap" style={{ whiteSpace: 'normal', maxWidth: 420 }}>{e.before && <>from <span className="old mono">{JSON.stringify(e.before).slice(0, 80)}</span> </>}{e.after && <>to <span className="new-v mono">{JSON.stringify(e.after).slice(0, 80)}</span></>}</td><td><Pill tone={e.result === 'ok' ? 'ok' : e.result === 'queued' ? 'info' : 'warn'} xs>{e.result}</Pill></td></tr>)}
        </tbody></table></div>
      )}
    </Card>
  )
}

function Account() {
  const [form, setForm] = useState({ current: '', new: '' })
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const change = async (e: React.FormEvent) => { e.preventDefault(); setMsg(null); try { await api.post('/api/auth/password', form); setMsg({ ok: true, text: 'Password changed.' }); setForm({ current: '', new: '' }) } catch (err) { setMsg({ ok: false, text: (err as Error).message }) } }
  return (
    <form className="card" onSubmit={change} style={{ maxWidth: 520 }} aria-labelledby="acc">
      <div className="ch"><h2 className="h2" id="acc">Change password</h2></div>
      <div className="fr"><label htmlFor="cur">Current password</label><input className="fin" id="cur" type="password" value={form.current} onChange={(e) => setForm({ ...form, current: e.target.value })} required autoComplete="current-password" /></div>
      <div className="fr"><label htmlFor="nw">New password</label><div><input className="fin" id="nw" type="password" value={form.new} onChange={(e) => setForm({ ...form, new: e.target.value })} required autoComplete="new-password" /><div className="hint">At least 8 characters.</div></div></div>
      {msg && <div className="cf">{msg.ok ? <Banner tone="ok">{msg.text}</Banner> : <Banner tone="crit">{msg.text}</Banner>}</div>}
      <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end' }}><Btn type="submit" variant="pri">Change password</Btn></div>
    </form>
  )
}
