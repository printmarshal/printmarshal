import { useCallback, useEffect, useState } from 'react'
import { api, fmtTime } from '../api'
import type { Credential, Group, User } from '../types'

export default function Credentials({ user }: { user: User }) {
  const [creds, setCreds] = useState<Credential[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [type, setType] = useState('snmp_community')
  const [form, setForm] = useState({ name: '', scope: '', username: '', priority: 0, community: '', authProtocol: 'SHA', authPassword: '', privProtocol: 'AES', privPassword: '', password: '' })
  const [msg, setMsg] = useState('')
  const isAdmin = user.role === 'admin'
  const load = useCallback(() => {
    api.get<Credential[]>('/api/credentials').then(setCreds).catch((e) => setMsg(e.message))
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])

  const create = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    const secret: Record<string, string> = {}
    if (type === 'snmp_community') secret.community = form.community
    if (type === 'snmpv3') Object.assign(secret, { authProtocol: form.authProtocol, authPassword: form.authPassword, privProtocol: form.privProtocol, privPassword: form.privPassword })
    if (type === 'admin_password') secret.password = form.password
    try {
      await api.post('/api/credentials', { name: form.name, type, scope: form.scope, username: form.username, priority: Number(form.priority), secret })
      setForm({ ...form, name: '', community: '', authPassword: '', privPassword: '', password: '' })
      load()
    } catch (err) { setMsg((err as Error).message) }
  }
  const remove = async (c: Credential) => {
    if (!confirm(`Delete credential "${c.name}"?`)) return
    await api.del(`/api/credentials/${c.id}`); load()
  }
  return (
    <>
      <h1>Credentials</h1>
      <div className="muted" style={{ marginBottom: 12 }}>Secrets are encrypted at rest and never returned by the API. Without any credential, SNMP uses the "public" community. Device- and group-scoped credentials take precedence over global ones.</div>
      <div className="grid cols-2">
        <div className="panel">
          <table>
            <thead><tr><th>Name</th><th>Type</th><th>Scope</th><th>User</th><th>Priority</th><th>Created</th><th /></tr></thead>
            <tbody>
              {creds.map((c) => (
                <tr key={c.id}><td>{c.name}</td><td><span className="badge outline">{c.type}</span></td><td className="small">{scopeLabel(c.scope, groups)}</td><td>{c.username}</td><td>{c.priority}</td><td className="small">{fmtTime(c.createdAt)}</td>
                  <td>{isAdmin && <button className="danger" onClick={() => remove(c)}>Delete</button>}</td></tr>
              ))}
              {creds.length === 0 && <tr><td colSpan={7} className="muted">No credentials stored.</td></tr>}
            </tbody>
          </table>
        </div>
        {isAdmin && (
          <form className="panel" onSubmit={create} autoComplete="off">
            <h2>Add credential</h2>
            <div className="grid cols-2">
              <label className="field">Name<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></label>
              <label className="field">Type<select value={type} onChange={(e) => setType(e.target.value)}><option value="snmp_community">SNMP v1/v2c community</option><option value="snmpv3">SNMPv3 user</option><option value="admin_password">Device admin password</option></select></label>
              <label className="field">Scope<select value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value })}><option value="">Global</option>{groups.map((g) => <option key={g.id} value={`group:${g.id}`}>Group: {g.name}</option>)}</select></label>
              <label className="field">Priority<input type="number" value={form.priority} onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })} /></label>
            </div>
            {type === 'snmp_community' && <label className="field" style={{ marginTop: 8 }}>Community<input type="password" value={form.community} onChange={(e) => setForm({ ...form, community: e.target.value })} required /></label>}
            {type === 'snmpv3' && (
              <div className="grid cols-2" style={{ marginTop: 8 }}>
                <label className="field">Username<input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required /></label>
                <span />
                <label className="field">Auth protocol<select value={form.authProtocol} onChange={(e) => setForm({ ...form, authProtocol: e.target.value })}>{['MD5', 'SHA', 'SHA224', 'SHA256', 'SHA384', 'SHA512'].map((p) => <option key={p}>{p}</option>)}</select></label>
                <label className="field">Auth password<input type="password" value={form.authPassword} onChange={(e) => setForm({ ...form, authPassword: e.target.value })} /></label>
                <label className="field">Privacy protocol<select value={form.privProtocol} onChange={(e) => setForm({ ...form, privProtocol: e.target.value })}>{['DES', 'AES', 'AES192', 'AES256'].map((p) => <option key={p}>{p}</option>)}</select></label>
                <label className="field">Privacy password<input type="password" value={form.privPassword} onChange={(e) => setForm({ ...form, privPassword: e.target.value })} /></label>
              </div>
            )}
            {type === 'admin_password' && (
              <div className="grid cols-2" style={{ marginTop: 8 }}>
                <label className="field">Username<input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></label>
                <label className="field">Password<input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required /></label>
              </div>
            )}
            <div className="row" style={{ marginTop: 10 }}><button className="primary">Save</button><span className="error small">{msg}</span></div>
          </form>
        )}
      </div>
    </>
  )
}

function scopeLabel(scope: string, groups: Group[]): string {
  if (!scope) return 'global'
  if (scope.startsWith('group:')) return 'group: ' + (groups.find((g) => g.id === scope.slice(6))?.name || scope.slice(6, 14))
  return scope
}
