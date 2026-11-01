import { useCallback, useEffect, useState } from 'react'
import { api, fmtTime } from '../api'
import type { Credential, Group, User } from '../types'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, EmptyState, Pill, Seg, Tag } from '../components/ui'

/** What a credential may be used for (audit A19). Read credentials never write; write credentials never read. */
type Purpose = 'read' | 'write' | 'both'
type Cred = Credential & { purpose?: Purpose }
const purposeLabel: Record<Purpose, string> = { read: 'Read', write: 'Write', both: 'Read + write' }
const purposeHint = 'Write credentials are used for configuration, remediation, certificates and firmware; read credentials never are.'

export default function Credentials({ user }: { user: User }) {
  const [creds, setCreds] = useState<Cred[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [type, setType] = useState('snmp_community')
  const [purpose, setPurpose] = useState<Purpose>('read')
  const [form, setForm] = useState({ name: '', scope: '', username: '', priority: 0, community: '', authProtocol: 'SHA', authPassword: '', privProtocol: 'AES', privPassword: '', password: '' })
  const [msg, setMsg] = useState('')
  const isAdmin = user.role === 'admin'
  const load = useCallback(() => { api.get<Cred[]>('/api/credentials').then(setCreds).catch((e) => setMsg(e.message)); api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  const create = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    const secret: Record<string, string> = {}
    if (type === 'snmp_community') secret.community = form.community
    if (type === 'snmpv3') Object.assign(secret, { authProtocol: form.authProtocol, authPassword: form.authPassword, privProtocol: form.privProtocol, privPassword: form.privPassword })
    if (type === 'admin_password') secret.password = form.password
    try { await api.post('/api/credentials', { name: form.name, type, scope: form.scope, username: form.username, priority: Number(form.priority), purpose, secret }); setForm({ ...form, name: '', community: '', authPassword: '', privPassword: '', password: '' }); load() } catch (err) { setMsg((err as Error).message) }
  }
  const remove = async (c: Cred) => { if (!confirm(`Delete credential “${c.name}”?`)) return; await api.del(`/api/credentials/${c.id}`); load() }
  const scopeLabel = (s: string) => !s ? 'global' : s.startsWith('group:') ? `group: ${groups.find((g) => g.id === s.slice(6))?.name || s.slice(6, 14)}` : s
  const F = ({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) => <div className="fr"><label>{label}</label><div>{children}{hint && <div className="hint">{hint}</div>}</div></div>
  const writers = creds.filter((c) => c.purpose === 'write' || c.purpose === 'both').length
  return (
    <>
      <div className="ph"><div><h1 className="h1">Credentials</h1><div className="cap">Secrets are encrypted at rest and never returned by the API · device and group scopes take precedence over global</div></div></div>
      <Banner tone="info"><b style={{ color: 'var(--info)' }}>Write-only.</b> A stored secret is used by the engine when it talks to devices; it is never displayed again. Without any credential, SNMP uses the “public” community.</Banner>
      {creds.length > 0 && writers === 0 && <Banner tone="warn"><b>No write credential.</b> Every credential here is read-only, so configuration, remediation, certificate and firmware writes are refused on every device until a credential with purpose <b>Write</b> or <b>Read + write</b> is stored.</Banner>}
      {!isAdmin && <Banner tone="info"><b style={{ color: 'var(--info)' }}>Read-only for your role.</b> You can see which credentials exist. Adding or deleting one needs the admin role.</Banner>}
      {msg && <Banner tone="crit">{msg}</Banner>}
      <div className="cols">
        <Card title="Stored credentials" id="cl" sub={`${creds.length} · tried in priority order, highest first`} style={{ flex: '3 1 520px' }}>
          {creds.length === 0 ? <div className="cb"><EmptyState icon={<Ic.lock size={20} />} title="No credentials stored" text="Add an SNMPv3 user or a community so discovery and polling can read your printers." /></div> : (
            <div className="tw"><table className="tbl" style={{ minWidth: 720 }}>
              <thead><tr><th>Name</th><th>Type</th><th>Purpose</th><th>Scope</th><th>User</th><th className="r">Priority</th><th>Created</th><th /></tr></thead>
              <tbody>{creds.map((c) => (
                <tr key={c.id}><td style={{ fontWeight: 500 }}><Ic.lock size={14} style={{ color: 'var(--text-3)', verticalAlign: -2, marginRight: 6 }} />{c.name}</td>
                  <td>{c.type === 'snmpv3' ? <Tag>SNMPv3</Tag> : c.type === 'snmp_community' ? <Pill tone="warn" xs noIcon>v1/v2c community</Pill> : <Tag>{c.type.replace('_', ' ')}</Tag>}</td>
                  <td><Tag title={purposeHint}>{purposeLabel[c.purpose || 'read']}</Tag></td>
                  <td className="cap">{scopeLabel(c.scope)}</td><td className="mono">{c.username || '—'}</td><td className="num r">{c.priority}</td><td className="cap num">{fmtTime(c.createdAt)}</td>
                  <td className="r">{isAdmin && <Btn sm variant="ghost" onClick={() => remove(c)}>Delete</Btn>}</td></tr>
              ))}</tbody>
            </table></div>
          )}
        </Card>
        {isAdmin && (
          <form className="card" onSubmit={create} autoComplete="off" style={{ flex: '2 1 380px' }} aria-label="Add credential">
            <div className="ch"><h2 className="h2">Add credential</h2></div>
            <F label="Name"><input className="fin" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></F>
            <F label="Type"><select className="fin" value={type} onChange={(e) => setType(e.target.value)}><option value="snmp_community">SNMP v1/v2c community</option><option value="snmpv3">SNMPv3 user (preferred)</option><option value="admin_password">Device admin password</option></select></F>
            <F label="Purpose" hint={purposeHint}><Seg<Purpose> label="Purpose" sm value={purpose} onChange={setPurpose} options={[{ v: 'read', label: 'Read' }, { v: 'write', label: 'Write' }, { v: 'both', label: 'Read + write' }]} /></F>
            <F label="Scope" hint="Global applies to every device; a group scope only to its members."><select className="fin" value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value })}><option value="">Global</option>{groups.map((g) => <option key={g.id} value={`group:${g.id}`}>Group: {g.name}</option>)}</select></F>
            <F label="Priority" hint="Higher is tried first."><input className="fin num" type="number" value={form.priority} onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })} style={{ maxWidth: 120 }} /></F>
            {type === 'snmp_community' && <F label="Community"><input className="fin" type="password" value={form.community} onChange={(e) => setForm({ ...form, community: e.target.value })} required autoComplete="new-password" /></F>}
            {type === 'snmpv3' && (<>
              <F label="Username"><input className="fin" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required /></F>
              <F label="Authentication"><div className="row"><select className="fin" style={{ width: 120 }} value={form.authProtocol} onChange={(e) => setForm({ ...form, authProtocol: e.target.value })}>{['MD5', 'SHA', 'SHA224', 'SHA256', 'SHA384', 'SHA512'].map((p) => <option key={p}>{p}</option>)}</select><input className="fin" style={{ flex: 1 }} type="password" placeholder="auth password" value={form.authPassword} onChange={(e) => setForm({ ...form, authPassword: e.target.value })} autoComplete="new-password" /></div></F>
              <F label="Privacy"><div className="row"><select className="fin" style={{ width: 120 }} value={form.privProtocol} onChange={(e) => setForm({ ...form, privProtocol: e.target.value })}>{['DES', 'AES', 'AES192', 'AES256'].map((p) => <option key={p}>{p}</option>)}</select><input className="fin" style={{ flex: 1 }} type="password" placeholder="privacy password" value={form.privPassword} onChange={(e) => setForm({ ...form, privPassword: e.target.value })} autoComplete="new-password" /></div></F>
            </>)}
            {type === 'admin_password' && (<>
              <F label="Username"><input className="fin" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></F>
              <F label="Password"><input className="fin" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required autoComplete="new-password" /></F>
            </>)}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, padding: '12px 16px', borderTop: '1px solid var(--border)' }}><Btn type="submit" variant="pri"><Ic.lock />Store credential</Btn></div>
          </form>
        )}
      </div>
    </>
  )
}
