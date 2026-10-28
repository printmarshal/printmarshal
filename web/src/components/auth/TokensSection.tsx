import { useCallback, useEffect, useState } from 'react'
import { api, fmtTime } from '../../api'
import type { Group } from '../../types'
import { Ic } from '../icons'
import { Banner, Btn, Card, Dialog, EmptyState, Pill, Tag } from '../ui'

// API tokens (service accounts, docs/auth.md): admin-managed principals for
// integrations. The secret is shown exactly once, right after creation.
// Types are local because tokens live outside the core model.

type Role = 'viewer' | 'operator' | 'admin'
interface APIToken { id: string; name: string; role: Role; scope: string[]; expiresAt?: string; createdBy: string; createdAt: string; lastUsedAt?: string; revokedAt?: string }
interface Created { token: APIToken; secret: string }

const ttlOptions = [{ v: '', label: 'Never expires' }, { v: '720h', label: '30 days' }, { v: '2160h', label: '90 days' }, { v: '8760h', label: '1 year' }]

function TokenState({ t }: { t: APIToken }) {
  if (t.revokedAt) return <Pill tone="off" xs>revoked {fmtTime(t.revokedAt)}</Pill>
  if (t.expiresAt && new Date(t.expiresAt).getTime() < Date.now()) return <Pill tone="warn" xs>expired</Pill>
  return <Pill tone="ok" xs>active</Pill>
}

export default function TokensSection() {
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [form, setForm] = useState<{ name: string; role: Role; scope: string[]; ttl: string }>({ name: '', role: 'viewer', scope: [], ttl: '720h' })
  const [created, setCreated] = useState<Created | null>(null)
  const [copied, setCopied] = useState(false)
  const [revoke, setRevoke] = useState<APIToken | null>(null)
  const [msg, setMsg] = useState('')
  const [showRevoked, setShowRevoked] = useState(false)
  const load = useCallback(() => {
    api.get<APIToken[]>('/api/tokens').then(setTokens).catch((err) => setMsg((err as Error).message))
    api.get<Group[]>('/api/groups').then(setGroups).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])

  const create = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg(''); setCopied(false)
    try {
      const body: Record<string, unknown> = { name: form.name.trim(), role: form.role, scope: form.scope }
      if (form.ttl) body.ttl = form.ttl
      setCreated(await api.post<Created>('/api/tokens', body))
      setForm({ name: '', role: 'viewer', scope: [], ttl: '720h' })
      load()
    } catch (err) { setMsg((err as Error).message) }
  }
  const copy = async () => {
    if (!created) return
    try { await navigator.clipboard.writeText(created.secret); setCopied(true) } catch { setCopied(false) }
  }
  const doRevoke = async () => {
    if (!revoke) return
    try { await api.del(`/api/tokens/${revoke.id}`); setRevoke(null); load() } catch (err) { setMsg((err as Error).message) }
  }
  const toggleScope = (id: string) => setForm((f) => ({ ...f, scope: f.scope.includes(id) ? f.scope.filter((x) => x !== id) : [...f.scope, id] }))
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name || id.slice(0, 8)
  const visible = tokens.filter((t) => showRevoked || !t.revokedAt)
  const active = tokens.filter((t) => !t.revokedAt).length

  return (
    <>
      {created && (
        <Card title="Token created" id="tkn" sub="copy the secret now: it is shown once and never stored in clear text" right={<Btn sm variant="ghost" onClick={() => setCreated(null)}>Done</Btn>}>
          <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <Banner tone="warn">Anyone holding this secret acts as <b>{created.token.name}</b> with the <b>{created.token.role}</b> role{created.token.scope.length > 0 ? ` on ${created.token.scope.length} group(s)` : ''}. Store it in your integration's secret store.</Banner>
            <div className="row" style={{ gap: 8, alignItems: 'center' }}>
              <code className="mono" style={{ flex: '1 1 320px', padding: '8px 10px', border: '1px solid var(--border)', borderRadius: 6, overflowX: 'auto', userSelect: 'all' }} aria-label="Token secret">{created.secret}</code>
              <Btn variant="pri" onClick={copy}>{copied ? 'Copied' : 'Copy'}</Btn>
            </div>
            <div className="cap">Use it as <span className="mono">Authorization: Bearer {created.secret.slice(0, 8)}…</span> — tokens cannot sign in to the UI or manage users and tokens.</div>
          </div>
        </Card>
      )}
      <form className="card" onSubmit={create} autoComplete="off" aria-label="Create API token">
        <div className="ch"><div><h2 className="h2">Create an API token</h2><div className="cap">service accounts for integrations · audited · revocable at any time</div></div></div>
        <div className="fr"><label htmlFor="tk-name">Name</label><input className="fin" id="tk-name" placeholder="e.g. monitoring-exporter" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></div>
        <div className="fr"><label htmlFor="tk-role">Role</label><div><select className="fin" id="tk-role" style={{ width: 160 }} value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select><div className="hint">Same capabilities as a user of that role, minus authentication management.</div></div></div>
        <div className="fr"><span className="lb">Group scope</span><div>
          {groups.length === 0 ? <span className="cap">No groups yet · the token will see every device.</span> : (
            <div className="row" style={{ gap: 12, flexWrap: 'wrap' }}>
              {groups.map((g) => <label key={g.id} className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><input className="cbx" type="checkbox" checked={form.scope.includes(g.id)} onChange={() => toggleScope(g.id)} />{g.name}{g.kind === 'rule' && <Tag>rule</Tag>}</label>)}
            </div>
          )}
          <div className="hint">{form.scope.length === 0 ? 'Unscoped: every device.' : `Limited to devices in ${form.scope.length} group(s); everything else answers 403.`}</div>
        </div></div>
        <div className="fr"><label htmlFor="tk-ttl">Expiry</label><select className="fin" id="tk-ttl" style={{ width: 160 }} value={form.ttl} onChange={(e) => setForm({ ...form, ttl: e.target.value })}>{ttlOptions.map((o) => <option key={o.v} value={o.v}>{o.label}</option>)}</select></div>
        <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, alignItems: 'center' }}>{msg && <span className="error small" style={{ marginRight: 'auto' }}>{msg}</span>}<Btn type="submit" variant="pri">Create token</Btn></div>
      </form>
      <Card title="API tokens" id="tks" sub={`${active} active · ${tokens.length - active} revoked`} right={<label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><input className="cbx" type="checkbox" checked={showRevoked} onChange={(e) => setShowRevoked(e.target.checked)} />Show revoked</label>}>
        {visible.length === 0 ? <div className="cb"><EmptyState icon={<Ic.key size={20} />} title="No API tokens" text="Create one above for an integration; humans should keep using their own accounts." /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 720 }}><thead><tr><th>Name</th><th>Role</th><th>Scope</th><th>Expires</th><th>Last used</th><th>State</th><th /></tr></thead><tbody>
            {visible.map((t) => (
              <tr key={t.id} style={t.revokedAt ? { opacity: 0.6 } : undefined}>
                <td><div style={{ fontWeight: 500 }}>{t.name}</div><div className="cap">by {t.createdBy} · {fmtTime(t.createdAt)}</div></td>
                <td><Tag>{t.role}</Tag></td>
                <td style={{ whiteSpace: 'normal' }}>{t.scope.length === 0 ? <span className="cap">all devices</span> : t.scope.map((g) => <Tag key={g}>{groupName(g)}</Tag>)}</td>
                <td className="cap num">{t.expiresAt ? fmtTime(t.expiresAt) : 'never'}</td>
                <td className="cap num">{t.lastUsedAt ? fmtTime(t.lastUsedAt) : 'never'}</td>
                <td><TokenState t={t} /></td>
                <td className="r">{!t.revokedAt && <Btn sm variant="dng" onClick={() => setRevoke(t)}>Revoke</Btn>}</td>
              </tr>
            ))}
          </tbody></table></div>
        )}
      </Card>
      {revoke && (
        <Dialog title={`Revoke “${revoke.name}”?`} sub="The next request with this token gets 401. This cannot be undone; create a new token to restore access." icon={<Ic.key size={20} />} onClose={() => setRevoke(null)}
          footer={<><Btn variant="ghost" onClick={() => setRevoke(null)}>Keep it</Btn><Btn variant="dngf" onClick={doRevoke}>Revoke token</Btn></>} />
      )}
    </>
  )
}
