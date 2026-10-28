import { useCallback, useEffect, useState } from 'react'
import { api, fmtTime } from '../api'
import type { AuditEntry, ConsumableSettings, User } from '../types'

export default function Settings({ user }: { user: User }) {
  const [tab, setTab] = useState<'runtime' | 'consumables' | 'users' | 'audit' | 'account'>('runtime')
  return (
    <>
      <h1>Settings</h1>
      <div className="tabs">
        {(['runtime', 'consumables', 'users', 'audit', 'account'] as const).filter((t) => t !== 'users' || user.role === 'admin').filter((t) => t !== 'audit' || user.role !== 'viewer').map((t) => (
          <button key={t} className={tab === t ? 'active' : ''} onClick={() => setTab(t)}>{t}</button>
        ))}
      </div>
      {tab === 'runtime' && <Runtime />}
      {tab === 'consumables' && <ConsumablesSettings canEdit={user.role === 'admin'} />}
      {tab === 'users' && <Users me={user} />}
      {tab === 'audit' && <Audit />}
      {tab === 'account' && <Account />}
    </>
  )
}

function Runtime() {
  const [s, setS] = useState<Record<string, unknown> | null>(null)
  const [drivers, setDrivers] = useState<{ name: string; capabilities: string[] }[]>([])
  useEffect(() => {
    api.get<Record<string, unknown>>('/api/settings').then(setS).catch(() => {})
    api.get<{ name: string; capabilities: string[] }[]>('/api/drivers').then(setDrivers).catch(() => {})
  }, [])
  if (!s) return <div className="muted">Loading…</div>
  return (
    <div className="grid cols-2">
      <div className="panel"><h2>Runtime configuration</h2><div className="muted small" style={{ marginBottom: 8 }}>Set via config file or PM_* environment variables; restart to apply.</div><pre>{JSON.stringify(s, null, 2)}</pre></div>
      <div className="panel"><h2>Registered drivers</h2>
        <table><thead><tr><th>Driver</th><th>Capabilities</th></tr></thead><tbody>
          {drivers.map((d) => <tr key={d.name}><td>{d.name}</td><td className="small">{d.capabilities.join(', ')}</td></tr>)}
        </tbody></table>
        <div className="muted small" style={{ marginTop: 8 }}>Vendor plugins register against the same driver interface; the core has no vendor-specific code.</div>
      </div>
    </div>
  )
}

function ConsumablesSettings({ canEdit }: { canEdit: boolean }) {
  const [s, setS] = useState<ConsumableSettings | null>(null)
  const [msg, setMsg] = useState('')
  useEffect(() => { api.get<ConsumableSettings>('/api/consumables/settings').then(setS).catch(() => {}) }, [])
  if (!s) return <div className="muted">Loading…</div>
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    try { setS(await api.put<ConsumableSettings>('/api/consumables/settings', s)); setMsg('Saved.') } catch (err) { setMsg((err as Error).message) }
  }
  const num = (k: keyof ConsumableSettings) => (e: React.ChangeEvent<HTMLInputElement>) => setS({ ...s, [k]: Math.max(0, Number(e.target.value) || 0) })
  return (
    <form className="grid cols-2" onSubmit={save}>
      <div className="panel">
        <h2>Stock defaults</h2>
        <div className="muted small" style={{ marginBottom: 8 }}>Applied to new consumables; each item keeps its own thresholds. {!canEdit && 'Only admins can change these.'}</div>
        <div className="grid cols-2">
          <label className="field">Default warning at ≤<input type="number" min={0} value={s.defaultWarningAt} disabled={!canEdit} onChange={num('defaultWarningAt')} /></label>
          <label className="field">Default critical at ≤<input type="number" min={0} value={s.defaultCriticalAt} disabled={!canEdit} onChange={num('defaultCriticalAt')} /></label>
          <label className="field">Forecast needs at least (days of history)<input type="number" min={1} value={s.forecastMinDays} disabled={!canEdit} onChange={num('forecastMinDays')} /></label>
          <label className="field">"Reorder soon" when stock lasts ≤ (days)<input type="number" min={0} value={s.reorderLeadDays} disabled={!canEdit} onChange={num('reorderLeadDays')} /></label>
        </div>
        <label className="field" style={{ marginTop: 8, flexDirection: 'row', alignItems: 'center', gap: 8 }}><input type="checkbox" checked={s.autoSuggestFromScans} disabled={!canEdit} onChange={(e) => setS({ ...s, autoSuggestFromScans: e.target.checked })} /> Suggest new stock items from unmatched installed supplies</label>
      </div>
      <div className="panel">
        <h2>Replacement detection</h2>
        <div className="muted small" style={{ marginBottom: 8 }}>When an installed supply's level jumps back up, PrintMarshal infers a replacement and can take one unit from the linked stock item. Every detection is logged as a stock event.</div>
        <label className="field">Auto-decrement stock on replacement
          <select value={s.autoDecrement} disabled={!canEdit} onChange={(e) => setS({ ...s, autoDecrement: e.target.value as ConsumableSettings['autoDecrement'] })}>
            <option value="off">Off — log only, never change stock</option>
            <option value="prompt">Prompt — ask an operator to confirm each one</option>
            <option value="automatic">Automatic — decrement immediately</option>
          </select>
        </label>
        <label className="field" style={{ marginTop: 8 }}>Level rise that counts as a replacement (%)<input type="number" min={10} max={95} value={s.replacementJumpPct} disabled={!canEdit} onChange={num('replacementJumpPct')} /></label>
        {canEdit && <div className="row" style={{ marginTop: 12 }}><button className="primary">Save</button><span className="small">{msg}</span></div>}
      </div>
    </form>
  )
}

function Users({ me }: { me: User }) {
  const [users, setUsers] = useState<User[]>([])
  const [form, setForm] = useState({ username: '', password: '', role: 'viewer' })
  const [msg, setMsg] = useState('')
  const load = useCallback(() => { api.get<User[]>('/api/users').then(setUsers).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  const add = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    try { await api.post('/api/users', form); setForm({ username: '', password: '', role: 'viewer' }); load() } catch (err) { setMsg((err as Error).message) }
  }
  return (
    <div className="grid cols-2">
      <div className="panel"><h2>Users</h2>
        <table><thead><tr><th>User</th><th>Role</th><th>Created</th><th /></tr></thead><tbody>
          {users.map((u) => <tr key={u.id}><td>{u.username}</td><td><span className="badge outline">{u.role}</span></td><td className="small">{fmtTime(u.createdAt)}</td>
            <td>{u.id !== me.id && <button className="danger" onClick={async () => { if (confirm(`Delete ${u.username}?`)) { await api.del(`/api/users/${u.id}`); load() } }}>Delete</button>}</td></tr>)}
        </tbody></table>
      </div>
      <form className="panel" onSubmit={add} autoComplete="off">
        <h2>Add user</h2>
        <div className="grid cols-2">
          <label className="field">Username<input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required /></label>
          <label className="field">Role<select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}><option value="viewer">viewer (read-only)</option><option value="operator">operator (device writes, polls)</option><option value="admin">admin (credentials, users)</option></select></label>
        </div>
        <label className="field" style={{ marginTop: 8 }}>Password (min 8)<input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required /></label>
        <div className="row" style={{ marginTop: 10 }}><button className="primary">Create</button><span className="error small">{msg}</span></div>
      </form>
    </div>
  )
}

function Audit() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  useEffect(() => { api.get<AuditEntry[]>('/api/audit').then(setEntries).catch(() => {}) }, [])
  return (
    <div className="panel">
      <h2>Audit log</h2>
      <table><thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th><th>Change</th><th>Result</th></tr></thead><tbody>
        {entries.map((e) => <tr key={e.id}><td className="small">{fmtTime(e.at)}</td><td>{e.actor}</td><td>{e.action}</td><td className="small">{e.target.slice(0, 12)}</td>
          <td className="small muted">{e.before && <>from {JSON.stringify(e.before)} </>}{e.after && <>to {JSON.stringify(e.after)}</>}</td><td>{e.result}</td></tr>)}
        {entries.length === 0 && <tr><td colSpan={6} className="muted">Nothing audited yet.</td></tr>}
      </tbody></table>
    </div>
  )
}

function Account() {
  const [form, setForm] = useState({ current: '', new: '' })
  const [msg, setMsg] = useState('')
  const change = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    try { await api.post('/api/auth/password', form); setMsg('Password changed.'); setForm({ current: '', new: '' }) } catch (err) { setMsg((err as Error).message) }
  }
  return (
    <form className="panel" onSubmit={change} style={{ maxWidth: 420 }}>
      <h2>Change password</h2>
      <label className="field">Current password<input type="password" value={form.current} onChange={(e) => setForm({ ...form, current: e.target.value })} required /></label>
      <label className="field" style={{ marginTop: 8 }}>New password (min 8)<input type="password" value={form.new} onChange={(e) => setForm({ ...form, new: e.target.value })} required /></label>
      <div className="row" style={{ marginTop: 10 }}><button className="primary">Change</button><span className="small">{msg}</span></div>
    </form>
  )
}
